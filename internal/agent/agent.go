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
	config      config.AgentConfig
	mediaCfg    config.MediaConfig // media cleanup configuration
	limits      config.LimitsConfig
	filterCfg   config.FilterConfig
	ctxModeCfg  config.ContextModeConfig // context-mode configuration
	channel     channel.Channel
	provider    provider.Provider
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
	startedAt       time.Time
	inbox           chan channel.IncomingMessage
	channelName     string
	bus             notify.Bus

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
	contextMgr := NewContextManager(ctxCfg, prov, nil)

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
		enricher:        NewEnricher(prov, st, cfg),
		embeddingWorker: embWorker,
		indexWorker:     idxWorker,
		contextMgr:      contextMgr,
		commands:        reg,
		channelName:     ch.Name(),
	}
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

	// Register the /compact command now that the agent struct is fully built.
	reg.Register("compact", "Force-compact conversation context", func(cc CommandContext) error {
		return a.cmdCompact(cc)
	}, SourceBuiltin)
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
