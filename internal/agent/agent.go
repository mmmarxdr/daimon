package agent

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/notify"
	"daimon/internal/provider"
	"daimon/internal/rag"
	"daimon/internal/rag/metrics"
	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ensureToolMap returns m unchanged if non-nil, else an empty map.
// Prevents nil-map panics when New() is called with tools=nil in tests.
func ensureToolMap(m map[string]tool.Tool) map[string]tool.Tool {
	if m != nil {
		return m
	}
	return make(map[string]tool.Tool)
}

// startPruningLoop launches the memory pruning goroutine. It is a no-op when
// the store is not a *SQLiteStore (pruning is SQLite-only).
//
// One goroutine runs PruneMemories once at startup and a second goroutine
// fires on cfg.PruneInterval. Both exit cleanly when ctx is cancelled.
func (a *Agent) startPruningLoop(ctx context.Context) {
	sqlStore, ok := a.store.(*store.SQLiteStore)
	if !ok {
		return
	}
	cfg := store.PruneConfig{
		Threshold:     a.config.PruneThreshold,
		RetentionDays: a.config.PruneRetentionDays,
		Lambda:        0.03,
		BoostFactor:   0.5,
	}

	// Startup prune — runs once in its own goroutine so it doesn't block Run.
	go func() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		p, d, err := sqlStore.PruneMemories(ctx, cfg)
		if err != nil {
			slog.Warn("startup pruning failed", "error", err)
		} else {
			slog.Info("startup pruning complete", "soft_deleted", p, "hard_deleted", d)
		}
	}()

	// Periodic ticker goroutine.
	interval := a.config.PruneInterval
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p, d, err := sqlStore.PruneMemories(ctx, cfg)
				if err != nil {
					slog.Warn("periodic pruning failed", "error", err)
				} else if p > 0 || d > 0 {
					slog.Info("periodic pruning complete", "soft_deleted", p, "hard_deleted", d)
				}
			}
		}
	}()
}

type Agent struct {
	config     config.AgentConfig
	mediaCfg   config.MediaConfig // media cleanup configuration
	limits     config.LimitsConfig
	filterCfg  config.FilterConfig
	ctxModeCfg config.ContextModeConfig // context-mode configuration
	channel    channel.Channel

	// providerMu guards a.provider. Reads (including sub-components via
	// providerSnapshot) take RLock; SetProvider (PR2) takes Lock.
	// Mirror of toolsMu/skillsMu — see agent.go:109-114 for the pattern.
	// Lock ordering: providerMu slots between commandsMu and cancels.mu.
	providerMu    sync.RWMutex
	provider      provider.Provider
	providerCreds config.ProviderCredentials // stored at construction for PR2 thinking-config re-apply

	// modeMu guards a.currentMode. Reads via RLock in modeSnapshot(); writes via
	// Lock in SetMode + loadMode.
	//
	// Lock ordering: modeMu is INDEPENDENT of providerMu — no code path holds
	// both simultaneously. providerSnapshot() and modeSnapshot() are called
	// sequentially at turn-start; each releases its lock before returning.
	// Acquire EITHER but never both nested (to avoid deadlock).
	modeMu      sync.RWMutex
	currentMode string // one of: "plan" | "build" | "review"; zero value defaults to "build"

	// newProviderFn is the factory used by SetProvider to construct a replacement
	// provider from a config. Defaults to provider.NewFromConfig; overridable in
	// tests to avoid real API calls.
	newProviderFn func(config.ProviderConfig) (provider.Provider, error)

	store       store.Store
	outputStore store.OutputStore // for auto-indexing tool outputs
	auditorFn   func() audit.Auditor

	// subMgr manages spawned child agents (nil when no executable skills loaded).
	subMgr *SubagentManager

	// preStartedInbox is set by the production SubagentManager.newChildAgent
	// closure before launching Run in a goroutine. When set, Run reuses this
	// inbox (and the channel.Start call becomes idempotent) instead of creating
	// a fresh one — ensuring the inbox that SubagentChannel.Deliver wrote to is
	// the same one the child's event loop reads from.
	preStartedInbox chan channel.IncomingMessage

	// tools / skills are protected by toolsMu / skillsMu so the dashboard's
	// hot-add flow (RegisterMCPServer / ReplaceSkills) can mutate them while
	// the agent loop is running. Reads are RLock; mutations Lock. The
	// processMessage hot path acquires RLock once at the top and again for
	// each tool invocation — both are <100ns and not measurable next to LLM
	// calls or tool execution.
	toolsMu sync.RWMutex
	tools   map[string]tool.Tool
	// mcpToolNames tracks which tool names came from each MCP server, so
	// UnregisterMCPServer can remove just that server's tools without
	// touching native tools or other MCPs.
	mcpToolNames map[string][]string
	// mcpClients holds the live MCPCaller for each hot-added server so we
	// can Close() it on Unregister or process exit. Boot-time servers are
	// owned by the boot Manager (not tracked here) — only hot-adds.
	mcpClients      map[string]interface{ Close() error }
	skillsMu        sync.RWMutex
	skills          []skill.SkillContent
	skillIndex      skill.SkillIndex
	sem             chan struct{}    // concurrency semaphore
	stream          bool             // true when streaming is enabled and provider supports it
	enricher        *Enricher        // async tag enrichment worker; nil when disabled
	embeddingWorker *EmbeddingWorker // async embedding worker; nil when disabled
	indexWorker     *IndexingWorker  // async output indexing worker; nil when disabled
	curator         *Curator         // smart memory curation; nil when disabled
	consolidator    *Consolidator    // background memory consolidation; nil when disabled
	contextMgr      *ContextManager  // smart context management; nil when disabled
	commands        *CommandRegistry
	cancels         *cancelRegistry // per-(channel,sender) turn cancel funcs (WU4, REQ-6, REQ-7)
	shellCwd        *cwdOverrides   // per-(channel,sender) shell working-dir overrides (WU5, REQ-5)
	activeConv      *convOverrides  // per-(channel,sender) active conv ID overrides (WU7, REQ-1)
	startedAt       time.Time
	inbox           chan channel.IncomingMessage
	channelName     string
	bus             notify.Bus

	// activeTurns is a per-turn registry of live *store.Conversation pointers,
	// keyed by conversation ID. It enables the todo bridge callbacks to locate
	// the in-flight *conv and mutate its Metadata in place so the existing
	// turn-end SaveConversation at loop.go:952 persists the change naturally
	// (D4 resolution, AD-1).
	//
	// Lock ordering: activeTurnsMu is INDEPENDENT of all other agent mutexes
	// (modeMu, providerMu, toolsMu). Never hold activeTurnsMu while calling
	// the store or the bus — copy the pointer out, release the lock, then act.
	activeTurns   map[string]*store.Conversation
	activeTurnsMu sync.Mutex

	// RAG fields — nil when RAG is not wired.
	ragStore         rag.DocumentStore
	ragEmbedFn       func(context.Context, string) ([]float32, error)
	ragMaxChunks     int
	ragMaxTokens     int
	ragRetrievalConf rag.RAGRetrievalConf // neighbor expansion + score thresholds

	// HyDE fields — zero/nil when HyDE is disabled.
	ragHydeConf     config.RAGHydeConf
	ragHypothesisFn func(context.Context, string) (string, error)

	// Metrics recorder — nil means no-op (NoopRecorder equivalent).
	ragMetrics metrics.Recorder

	// Title generator hook — nil means the post-turn title hook is a no-op.
	// The Titler is defined as a minimal interface so the agent package does
	// not depend on TitleGenerator's internals (Group D wires the concrete
	// implementation). Enqueue MUST be non-blocking; drops on queue-full.
	titler Titler
	aiCfg  config.AIConfig
}

// Titler is the minimal interface the agent needs to enqueue async title
// generation jobs after saving a conversation. The concrete implementation
// (TitleGenerator) lives elsewhere; we keep this interface tiny so the
// agent layer does not import provider machinery.
type Titler interface {
	Enqueue(ctx context.Context, convID string)
}

func New(
	cfg config.AgentConfig,
	limits config.LimitsConfig,
	filterCfg config.FilterConfig,
	ch channel.Channel,
	prov provider.Provider,
	st store.Store,
	auditor audit.Auditor, // wrapped into a static accessor; use WithAuditorAccessor for hot-swap
	tools map[string]tool.Tool,
	skills []skill.SkillContent,
	skillIndex skill.SkillIndex,
	maxConcurrent int,
	stream bool,
	storeCfg ...config.StoreConfig,
) *Agent {
	if maxConcurrent <= 0 {
		maxConcurrent = 4
	}

	// Only enable streaming if the provider actually implements StreamingProvider.
	enableStream := stream
	if enableStream {
		if _, ok := prov.(provider.StreamingProvider); !ok {
			slog.Warn("streaming enabled in config but provider does not implement StreamingProvider, falling back to sync")
			enableStream = false
		}
	}

	// Wire embedding worker if store is SQLite and embeddings are enabled.
	var embWorker *EmbeddingWorker
	var sCfg config.StoreConfig
	if len(storeCfg) > 0 {
		sCfg = storeCfg[0]
	}
	if sqlStore, ok := st.(*store.SQLiteStore); ok && sCfg.Embeddings {
		embWorker = NewEmbeddingWorker(prov, sqlStore.DB(), sCfg)
		if embWorker != nil {
			// Capture the type assertion outside the closure — safe because
			// NewEmbeddingWorker already verified prov implements EmbeddingProvider.
			ep := prov.(provider.EmbeddingProvider)
			// Register the embed function for two-phase search reranking.
			sqlStore.SetEmbedQueryFunc(func(ctx context.Context, text string) ([]float32, error) {
				return ep.Embed(ctx, text)
			})
		}
	}

	// Extract OutputStore if available (for auto-indexing)
	var outputStore store.OutputStore
	if sqlStore, ok := st.(store.OutputStore); ok {
		outputStore = sqlStore
	}

	// Wire indexing worker when an OutputStore is present.
	// Agent owns the full lifecycle: created and started here, stopped in Shutdown.
	var idxWorker *IndexingWorker
	if outputStore != nil {
		idxWorker = NewIndexingWorker(outputStore)
		idxWorker.Start(context.Background())
	}

	reg := NewCommandRegistry()
	registerBuiltinCommands(reg)

	// Synthesize ContextConfig from legacy flat fields when cfg.Context is zero-value.
	// Priority:
	//   1. cfg.Context.Strategy is already set → use cfg.Context as-is
	//   2. MaxContextTokens > 0 → strategy "smart"
	//   3. HistoryLength > 0 only → strategy "legacy"
	//   4. Neither → strategy "none"
	ctxCfg := cfg.Context
	if ctxCfg.Strategy == "" {
		switch {
		case cfg.MaxContextTokens > 0:
			ctxCfg.Strategy = "smart"
			ctxCfg.MaxTokens = cfg.MaxContextTokens
			ctxCfg.SummaryMaxTokens = cfg.SummaryTokens
		case cfg.HistoryLength > 0:
			ctxCfg.Strategy = "legacy"
		default:
			ctxCfg.Strategy = "none"
		}
	}
	// providerSnapshotFn is a temporary closure used during construction to pass
	// the provider accessor to sub-components. After the agent struct is built,
	// sub-components should use a.providerSnapshot() via the same closure shape.
	// We use a pointer indirection: after `a` is allocated below, the closure
	// will read a.provider under a.providerMu on each call.
	// However, at construction time `a` does not yet exist, so we first build a
	// simple static closure pointing at prov, then replace contextMgr/enricher
	// with the live-snapshot closure after `a` is built.
	staticProvFn := func() provider.Provider { return prov }
	contextMgr := NewContextManager(ctxCfg, staticProvFn, nil)

	// Register compact command as a closure so it can access the agent after construction.
	// legacyFn will be wired after the agent struct is built (needs `a` to call legacyTruncate).
	// This is a two-step registration: we register a placeholder here and fix it up
	// after the agent is built — or we use a post-construction step.
	// Simple approach: register it after the struct is built below.

	a := &Agent{
		config:          cfg,
		limits:          limits,
		filterCfg:       filterCfg,
		ctxModeCfg:      cfg.ContextMode,
		channel:         ch,
		provider:        prov,
		store:           st,
		outputStore:     outputStore,
		auditorFn:       func() audit.Auditor { return auditor },
		tools:           ensureToolMap(tools),
		mcpToolNames:    map[string][]string{},
		mcpClients:      map[string]interface{ Close() error }{},
		skills:          skills,
		skillIndex:      skillIndex,
		sem:             make(chan struct{}, maxConcurrent),
		stream:          enableStream,
		enricher:        NewEnricher(staticProvFn, st, cfg),
		embeddingWorker: embWorker,
		indexWorker:     idxWorker,
		contextMgr:      contextMgr,
		commands:        reg,
		cancels:         newCancelRegistry(),
		shellCwd:        newCwdOverrides(),
		activeConv:      newConvOverrides(),
		channelName:     ch.Name(),
		newProviderFn:   provider.NewFromConfig,
		activeTurns:     make(map[string]*store.Conversation),
	}
	// Re-wire sub-components to use the live-snapshot closure now that `a` exists.
	// The staticProvFn used at construction read the original `prov` by value.
	// The live closure calls a.providerSnapshot() under providerMu.RLock, so sub-
	// components see the updated provider after a SetProvider swap (REQ-3, REQ-4).
	liveProvFn := func() provider.Provider { return a.providerSnapshot() }
	if a.enricher != nil {
		a.enricher.providerFn = liveProvFn
	}
	a.contextMgr.providerFn = liveProvFn

	// Wire the legacy truncation function now that the agent struct is fully built.
	// This lets ContextManager.legacyManage delegate to the existing legacyTruncate method.
	// The closure preserves the original guard: only truncate when over HistoryLength.
	if ctxCfg.Strategy == "legacy" {
		histLen := cfg.HistoryLength
		a.contextMgr.legacyFn = func(ctx context.Context, messages []provider.ChatMessage) []provider.ChatMessage {
			if histLen > 0 && len(messages) > histLen {
				return a.legacyTruncate(ctx, messages)
			}
			return messages
		}
	}

	// Register method-bound commands that require agent access.
	reg.Register("compact", "Force-compact conversation context", func(cc CommandContext) error {
		return a.cmdCompact(cc)
	}, SourceBuiltin)
	reg.Register("cancel", "Cancel in-progress LLM turn", func(cc CommandContext) error {
		return a.cmdCancel(cc)
	}, SourceBuiltin)
	reg.Register("cd", "Set shell working directory: /cd <path> (or /cd to reset)", func(cc CommandContext) error {
		return a.cmdCd(cc)
	}, SourceBuiltin)
	// WU7: new built-in commands (REQ-1..4).
	reg.Register("resume", "List or switch active conversation: /resume [convID]", func(cc CommandContext) error {
		return a.cmdResume(cc)
	}, SourceBuiltin)
	reg.Register("save", "Snapshot current conversation: /save [name]", func(cc CommandContext) error {
		return a.cmdSave(cc)
	}, SourceBuiltin)
	reg.Register("fork", "Branch conversation at last user turn: /fork", func(cc CommandContext) error {
		return a.cmdFork(cc)
	}, SourceBuiltin)
	reg.Register("export", "Export conversation: /export [markdown|json]", func(cc CommandContext) error {
		return a.cmdExport(cc)
	}, SourceBuiltin)
	// model-hot-swap PR5: /model command (REQ-2, REQ-6, REQ-7, REQ-9).
	reg.Register("model", "Swap the active LLM model or list available models. Usage: /model [<model_name>]", func(cc CommandContext) error {
		return a.cmdModel(cc)
	}, SourceBuiltin)
	// mode-system PR3: /mode command (REQ-1, REQ-2, REQ-3, REQ-10, REQ-12).
	reg.Register("mode", "Switch or list operational modes. Usage: /mode [<plan|build|review>]", func(cc CommandContext) error {
		return a.cmdMode(cc)
	}, SourceBuiltin)
	return a
}

// providerSnapshot returns the current provider under a short RLock.
// The returned interface value reflects the live a.provider pointer: callers
// MUST capture it into a local variable and use that local for the duration
// of one logical operation (e.g. one turn in processMessage) to guarantee
// consistency. Do NOT cache the result across goroutine-yielding operations.
//
// Sub-components (Enricher, ContextManager, etc.) receive this method as a
// closure: func() provider.Provider { return a.providerSnapshot() }. This
// keeps them agent-agnostic while still reading the live provider after a
// SetProvider swap (PR2).
func (a *Agent) providerSnapshot() provider.Provider {
	a.providerMu.RLock()
	defer a.providerMu.RUnlock()
	return a.provider
}

// modeSnapshot returns the ModeDefinition for the active mode under RLock.
// Mirror of providerSnapshot — acquire RLock briefly, copy out name, release,
// then resolve the tuple via LookupMode.
//
// If currentMode is empty (zero value, i.e. a fresh agent before any turn) or
// contains an invalid value (corruption / future downgrade), falls back to the
// "build" mode definition and logs a warning. This branch is unreachable in
// normal operation because SetMode validates before mutating.
//
// The returned ModeDefinition is a value copy — safe to use after the lock is
// released. Callers should capture the result once and use it for the entire turn.
func (a *Agent) modeSnapshot() ModeDefinition {
	a.modeMu.RLock()
	name := a.currentMode
	a.modeMu.RUnlock()

	def, err := LookupMode(name)
	if err != nil {
		// Unknown or empty currentMode: fall back to build (defensive).
		slog.Warn("mode_snapshot: unknown mode, falling back to build", "mode", name, "error", err)
		def, _ = LookupMode("build")
	}
	return def
}

// WithProviderCredentials stores the ProviderCredentials for the active
// provider so that SetProvider (PR2) can re-apply thinking configuration
// after rebuilding the provider via NewFromConfig.
//
// Rationale (AD-5): thinking config is wired POST-construction via
// SetThinkingConfig — it is NOT part of ProviderConfig. The agent must
// carry the original credentials to replicate what provider/registry.go
// does at startup.
//
// Call this after New() and before Run(), typically in main.go / web_cmd.go
// alongside the other WithX options.
func (a *Agent) WithProviderCredentials(creds config.ProviderCredentials) *Agent {
	a.providerCreds = creds
	return a
}

// WithAuditorAccessor replaces the static auditor wrapper set by New() with a
// dynamic accessor. The accessor is called on every Emit — it MUST be
// goroutine-safe (e.g. reading under an RLock). Use this when the auditor can
// be hot-swapped at runtime so the agent always emits to the current backend.
func (a *Agent) WithAuditorAccessor(fn func() audit.Auditor) *Agent {
	a.auditorFn = fn
	return a
}

// WithMediaConfig sets the media configuration on the agent, enabling the
// periodic media cleanup loop in Run(). Call before Run().
func (a *Agent) WithMediaConfig(cfg config.MediaConfig) *Agent {
	a.mediaCfg = cfg
	return a
}

// WithBus sets the event bus on the agent, enabling agent.turn.started/completed events.
// Also propagates the bus to contextMgr and subMgr (if already constructed) so
// that callers who invoke WithBus after WithExecutableSkills still get a wired bus.
// Returns a for fluent chaining.
func (a *Agent) WithBus(bus notify.Bus) *Agent {
	a.bus = bus
	if a.contextMgr != nil {
		a.contextMgr.bus = bus
	}
	if a.subMgr != nil {
		a.subMgr.bus = bus
	}
	return a
}

// WithCurator sets the Curator on the agent. Call after New(), before Run().
func (a *Agent) WithCurator(c *Curator) { a.curator = c }

// WithConsolidator sets the Consolidator on the agent. Call after New(), before Run().
func (a *Agent) WithConsolidator(c *Consolidator) { a.consolidator = c }

// WithRAGStore wires a DocumentStore into the agent for automatic retrieval-augmented
// generation. On every turn the agent will search for relevant chunks from st and
// inject them into the system prompt.
//
//   - embedFn may be nil (FTS-only search without vector reranking).
//   - maxChunks is the number of top chunks to retrieve per turn (default 5).
//   - maxTokens is the token budget for the RAG section in the prompt (default 10000).
func (a *Agent) WithRAGStore(st rag.DocumentStore, embedFn func(context.Context, string) ([]float32, error), maxChunks, maxTokens int) *Agent {
	a.ragStore = st
	a.ragEmbedFn = embedFn
	if maxChunks <= 0 {
		maxChunks = 5
	}
	if maxTokens <= 0 {
		maxTokens = 10000
	}
	a.ragMaxChunks = maxChunks
	a.ragMaxTokens = maxTokens
	return a
}

// WithRAGRetrievalConf stores retrieval-precision options (neighbor radius,
// score thresholds) that are applied on every SearchChunks call. Call this
// after WithRAGStore when the user has opted into non-default retrieval behavior.
func (a *Agent) WithRAGRetrievalConf(conf rag.RAGRetrievalConf) *Agent {
	a.ragRetrievalConf = conf
	return a
}

// RAGRetrievalConfig returns the retrieval-precision options currently in
// effect on the agent. Exposed so wiring regression tests can verify startup
// paths populated the config (the bug this guards against shipped in PR #2
// and went undetected for ~24h because existing tests only exercised the
// setter directly, never the wiring).
func (a *Agent) RAGRetrievalConfig() rag.RAGRetrievalConf {
	return a.ragRetrievalConf
}

// WithRAGHydeConf stores the HyDE configuration and hypothesis function.
// hypothesisFn may be nil — when nil, HyDE is effectively disabled regardless
// of conf.Enabled, and the baseline retrieval path is always used.
func (a *Agent) WithRAGHydeConf(conf config.RAGHydeConf, hypothesisFn func(context.Context, string) (string, error)) *Agent {
	a.ragHydeConf = conf
	a.ragHypothesisFn = hypothesisFn
	return a
}

// WithRAGMetrics sets the metrics recorder for the RAG retrieval path.
// When r is nil the agent behaves as if a NoopRecorder is set — no panic, no log.
func (a *Agent) WithRAGMetrics(r metrics.Recorder) *Agent {
	a.ragMetrics = r
	return a
}

// WithTitler wires the async title-generation worker. Pass nil to disable
// (the post-save hook is a no-op when titler is nil OR aiCfg.Enabled is false).
func (a *Agent) WithTitler(t Titler) *Agent {
	a.titler = t
	return a
}

// WithAIConfig stores the AI config (used for the title-generation enabled
// flag). Without this, the post-save hook stays a no-op even if WithTitler
// is called — wiring from main must call both.
func (a *Agent) WithAIConfig(cfg config.AIConfig) *Agent {
	a.aiCfg = cfg
	return a
}

// WithExecutableSkills wires spawnable subagent definitions into the agent.
// For each def a SubagentSpawnTool is registered in a.tools under def.Name.
// The agent's SubagentManager is created on the first call and its production
// newChildAgent closure is wired so that Spawn actually starts a child Agent.
// Call before Run().
//
// Two-phase tool validation (design §2.5.4): unknown names in def.ToolsAllowlist
// are dropped with a slog.Warn — the loader cannot cross-check at parse time
// because MCP tools are not yet materialized then.
func (a *Agent) WithExecutableSkills(defs []skill.ExecutableSkillDef) *Agent {
	if len(defs) == 0 {
		return a
	}
	if a.subMgr == nil {
		a.subMgr = NewSubagentManager(a.bus, a.store)
		a.subMgr.installBusSubscription()
	}

	// Wire the production newChildAgent closure. It captures a snapshot of the
	// parent's tools under toolsMu so filterParentTools receives the live map.
	// The closure is set once; subsequent WithExecutableSkills calls (if any)
	// reuse the same manager and the closure remains valid.
	if a.subMgr.newChildAgent == nil {
		a.subMgr.newChildAgent = a.makeChildAgentFn()
	}

	a.toolsMu.Lock()
	defer a.toolsMu.Unlock()
	for _, def := range defs {
		// Two-phase allowlist validation: drop unknown tool names with a warning.
		def.ToolsAllowlist = filterKnownTools(def.ToolsAllowlist, a.tools)
		spawn := &SubagentSpawnTool{def: def, manager: a.subMgr}
		if _, exists := a.tools[def.Name]; exists {
			slog.Warn("subagent tool name collides with existing tool; subagent wins", "name", def.Name)
		}
		a.tools[def.Name] = spawn
	}

	// WU8 (REQ-12): auto-mount each skill as a /<normalized_name> slash command.
	// registerSkillCommands acquires commands.mu internally — no deadlock risk
	// because toolsMu is held OUTSIDE and commands.mu is acquired INSIDE (correct
	// nesting order per design D5).
	registerSkillCommands(a, defs)

	return a
}

// makeChildAgentFn returns the production newChildAgent closure for the
// SubagentManager. The closure captures the parent agent (a) and:
//  1. Snapshots the parent's tools under toolsMu.
//  2. Builds a child provider (from def.ProviderName or falls back to the
//     parent's provider when the skill does not specify one).
//  3. Constructs a child Agent via agent.New with sem=1 and the sub-channel.
//  4. Pre-starts the sub-channel inbox so Spawn.Deliver writes to the correct
//     channel before the child goroutine has had a chance to call Start itself.
//  5. Launches go childAgent.Run(subCtx) in a goroutine.
//
// Tests override newChildAgent directly on the SubagentManager and never call
// makeChildAgentFn — this function is production-only.
func (a *Agent) makeChildAgentFn() func(
	def skill.ExecutableSkillDef,
	prompt string,
	subCtx context.Context,
	subCh *channel.SubagentChannel,
	parentTools map[string]tool.Tool,
	st store.Store,
) (*Agent, error) {
	return func(
		def skill.ExecutableSkillDef,
		_ string,
		subCtx context.Context,
		subCh *channel.SubagentChannel,
		_ map[string]tool.Tool, // ignored: closure re-snapshots under toolsMu
		st store.Store,
	) (*Agent, error) {
		// Snapshot parent tools at spawn time (may include MCP tools registered
		// after WithExecutableSkills was called).
		a.toolsMu.RLock()
		parentSnapshot := make(map[string]tool.Tool, len(a.tools))
		for k, v := range a.tools {
			parentSnapshot[k] = v
		}
		a.toolsMu.RUnlock()

		childTools := filterParentTools(parentSnapshot, def.ToolsAllowlist)

		// Build child provider: use the skill's declared provider when present;
		// otherwise inherit the parent's provider (same credentials, same model
		// unless overridden by def.Model).
		childProv := a.provider
		if def.ProviderName != "" {
			cfg := providerConfigForSkill(def, a.provider)
			p, err := provider.NewFromConfig(cfg)
			if err != nil {
				return nil, fmt.Errorf("subagent: create provider for skill %q: %w", def.Name, err)
			}
			childProv = p
		}

		childCfg := config.AgentConfig{
			MaxIterations: def.Budget.MaxTurns,
			Personality:   def.SystemAddendum,
		}
		childAgent := New(
			childCfg,
			a.limits,
			a.filterCfg,
			subCh,
			childProv,
			st,
			a.auditorFn(),
			childTools,
			nil,
			skill.SkillIndex{},
			1,     // sem=1: child agents are single-turn in V1
			false, // no streaming for subagents
		)
		// Wire the parent's bus so the child emits EventTurnCompleted;
		// the budgetMonitor fan-out in the parent's SubagentManager listens
		// for this event to track cost and detect natural completion.
		if a.bus != nil {
			childAgent.WithBus(a.bus)
		}

		// Pre-start the channel: create the inbox, hand it to subCh so that
		// Spawn.Deliver (called right after this closure returns) writes to the
		// same channel the Run loop will read from.
		inbox := make(chan channel.IncomingMessage, 100)
		childAgent.preStartedInbox = inbox
		if err := subCh.Start(subCtx, inbox); err != nil {
			return nil, fmt.Errorf("subagent: start channel: %w", err)
		}

		go func() {
			_ = childAgent.Run(subCtx)
		}()

		return childAgent, nil
	}
}

// providerConfigForSkill builds a config.ProviderConfig for a child agent
// from an ExecutableSkillDef. It uses the skill's declared provider type and,
// when the skill does not specify its own credentials, inherits them from the
// parent provider (when it exposes a Config() accessor).
func providerConfigForSkill(def skill.ExecutableSkillDef, parent provider.Provider) config.ProviderConfig {
	cfg := config.ProviderConfig{
		Type:  def.ProviderName,
		Model: def.Model,
	}

	// Inherit from parent when the parent exposes its config.
	// Uses the canonical provider.ConfigurableProvider interface (REQ-20).
	if pc, ok := parent.(provider.ConfigurableProvider); ok {
		parentCfg := pc.Config()
		if cfg.Model == "" {
			cfg.Model = parentCfg.Model
		}
		if cfg.Type == parentCfg.Type {
			// Same provider type: reuse credentials so the skill author doesn't
			// have to repeat API keys in every skill file.
			if cfg.APIKey == "" {
				cfg.APIKey = parentCfg.APIKey
			}
			if cfg.BaseURL == "" {
				cfg.BaseURL = parentCfg.BaseURL
			}
			if cfg.Timeout == 0 {
				cfg.Timeout = parentCfg.Timeout
			}
			if cfg.MaxRetries == 0 {
				cfg.MaxRetries = parentCfg.MaxRetries
			}
		}
	}

	// Explicit skill-level overrides win over parent values.
	if apiKey, ok := def.ProviderConfig["api_key"].(string); ok && apiKey != "" {
		cfg.APIKey = apiKey
	}
	if baseURL, ok := def.ProviderConfig["base_url"].(string); ok && baseURL != "" {
		cfg.BaseURL = baseURL
	}

	return cfg
}

// filterKnownTools returns the allowlist with unknown tool names removed.
// Unknown names are logged as warnings (non-fatal per design §2.5.4).
func filterKnownTools(allowlist []string, tools map[string]tool.Tool) []string {
	if len(allowlist) == 0 {
		return allowlist
	}
	filtered := make([]string, 0, len(allowlist))
	for _, name := range allowlist {
		if _, ok := tools[name]; ok {
			filtered = append(filtered, name)
		} else {
			slog.Warn("subagent tools_allowlist: unknown tool name dropped", "name", name)
		}
	}
	return filtered
}

// SubagentManager returns the agent's SubagentManager. May be nil when no
// executable skills are loaded. Used by the web handler for the active-subs endpoint.
func (a *Agent) SubagentManager() *SubagentManager { return a.subMgr }

// ActiveSubagents returns a snapshot of currently running subagents.
// Returns nil when no SubagentManager is configured.
// Satisfies web.SubagentProvider.
func (a *Agent) ActiveSubagents() []SubagentStatus {
	if a.subMgr == nil {
		return nil
	}
	return a.subMgr.Active()
}

// SubagentBus returns the notify.Bus wired into this agent.
// May be nil when no bus has been configured.
// Satisfies web.SubagentProvider.
func (a *Agent) SubagentBus() notify.Bus { return a.bus }

// CancelSubagent cancels a single running subagent by ID. Idempotent — calling
// twice on the same ID is safe (the second call is a no-op once the first
// fired). Returns nil when the agent has no SubagentManager (no executable
// skills loaded). Returns an error from SubagentManager.Cancel when the ID is
// not registered.
//
// Satisfies web.SubagentProvider.CancelSubagent. (REQ-18)
func (a *Agent) CancelSubagent(id string) error {
	if a.subMgr == nil {
		// No executable skills loaded → nothing to cancel.
		return nil
	}
	return a.subMgr.Cancel(id)
}

// Enricher returns the agent's async enrichment worker. May be nil.
func (a *Agent) Enricher() *Enricher { return a.enricher }

// EmbeddingWorker returns the agent's async embedding worker. May be nil.
func (a *Agent) EmbeddingWorker() *EmbeddingWorker { return a.embeddingWorker }

func (a *Agent) Run(ctx context.Context) error {
	a.startedAt = time.Now()

	// Use a pre-created inbox when running as a child agent. The
	// SubagentManager.newChildAgent closure creates the inbox, calls
	// subCh.Start, and sets preStartedInbox before launching this goroutine.
	// That guarantees SubagentChannel.Deliver writes to the same channel this
	// loop reads from. For normal (principal) agents, preStartedInbox is nil
	// and we create a fresh buffered channel as before.
	inbox := a.preStartedInbox
	if inbox == nil {
		inbox = make(chan channel.IncomingMessage, 100)
	}
	a.inbox = inbox

	if err := a.channel.Start(ctx, inbox); err != nil {
		return err
	}

	// Start background workers.
	if a.enricher != nil {
		a.enricher.Start(ctx)
	}
	if a.embeddingWorker != nil {
		a.embeddingWorker.Start(ctx)
	}
	if a.consolidator != nil {
		a.consolidator.Start(ctx)
	}
	// indexWorker is started in New() — no need to start here.
	a.startPruningLoop(ctx)

	// Start media cleanup loop when media is enabled and the store supports it.
	if config.BoolVal(a.mediaCfg.Enabled) {
		if _, ok := a.store.(store.MediaStore); ok {
			go a.mediaCleanupLoop(ctx)
		}
	}

	slog.Info("agent loop started")

	for {
		select {
		case <-ctx.Done():
			// Drain semaphore — wait for in-flight messages to complete.
			for i := 0; i < cap(a.sem); i++ {
				a.sem <- struct{}{}
			}
			return ctx.Err()
		case msg := <-inbox:
			go func(m channel.IncomingMessage) {
				select {
				case a.sem <- struct{}{}:
					defer func() { <-a.sem }()
					a.processMessage(ctx, m)
				case <-time.After(30 * time.Second):
					slog.Warn("agent: message dropped, semaphore timeout",
						"channel_id", m.ChannelID,
						"text_preview", truncate(m.Content.TextOnly(), 80))
				}
			}(msg)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (a *Agent) Shutdown() error {
	// Close the bus first — drains pending events while the mux can still deliver.
	if a.bus != nil {
		a.bus.Close()
	}

	// Stop the channel first — no more messages will be enqueued after this.
	err := a.channel.Stop()

	// Drain the index worker after the channel stops feeding new outputs.
	if a.indexWorker != nil {
		a.indexWorker.Stop()
	}

	// Stop remaining background workers.
	if a.consolidator != nil {
		a.consolidator.Stop()
	}
	if a.enricher != nil {
		a.enricher.Stop()
	}
	if a.embeddingWorker != nil {
		a.embeddingWorker.Stop()
	}
	return err
}

// Commands returns a snapshot of all registered commands for GET /api/commands.
// Each entry includes the source tag and the destructive flag from IsDestructiveCommand.
func (a *Agent) Commands() []CommandInfo {
	if a.commands == nil {
		return nil
	}
	entries := a.commands.EntriesWithSource()
	out := make([]CommandInfo, 0, len(entries))
	for _, e := range entries {
		out = append(out, CommandInfo{
			Name:        e.Name,
			Description: e.Desc,
			Source:      e.Source,
			Destructive: IsDestructiveCommand(e.Name),
		})
	}
	return out
}

// RunCommand dispatches a registered command by name for POST /api/commands/run.
// It builds a synthetic CommandContext with a buffer-backed Reply and returns
// the collected reply text. Returns an error when the command is not found or
// the handler returns an error.
func (a *Agent) RunCommand(ctx context.Context, req RunCommandRequest) (CommandResult, error) {
	h, ok := a.commands.Lookup(req.Name)
	if !ok {
		return CommandResult{}, fmt.Errorf("command not found: %s", req.Name)
	}

	var reply string
	cc := CommandContext{
		Ctx:          ctx,
		ChannelID:    req.ChannelID,
		SenderID:     req.SenderID,
		Args:         req.Args,
		Store:        a.store,
		Config:       &a.config,
		ProviderName: a.provider.Name(),
		ChannelName:  a.channelName,
		StartedAt:    a.startedAt,
		Registry:     a.commands,
		Reply: func(text string) {
			reply = text
		},
	}
	if err := h(cc); err != nil {
		return CommandResult{}, err
	}
	return CommandResult{Reply: reply}, nil
}
