package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/store"
)

// CompactorConfig controls the ConversationCompactor worker.
type CompactorConfig struct {
	// Enabled toggles the worker on/off. When false, NewConversationCompactor
	// returns nil and the agent runs without per-session summarisation.
	Enabled bool

	// Interval is how often the worker scans for compactable conversations.
	// 0 → 1h default. Setting this much shorter than `IdleAfter` is wasteful;
	// hourly is plenty.
	Interval time.Duration

	// IdleAfter is the minimum age (since updated_at) a conversation must
	// have before it becomes eligible for compaction. 0 → 7d default.
	IdleAfter time.Duration

	// Model overrides the chat model used for the summarisation call.
	// Empty → provider's default (cheapest model is fine — this is a
	// 1-shot summary, no tool use).
	Model string

	// CallTimeout caps each LLM summarisation call. 0 → 60s default.
	CallTimeout time.Duration

	// MaxConvsPerRun caps how many conversations the worker compacts in a
	// single tick to avoid bursts. 0 → 5 default.
	MaxConvsPerRun int
}

// applyCompactorDefaults fills in any zero-valued fields with sensible
// production defaults so callers can pass a partial config.
func applyCompactorDefaults(c CompactorConfig) CompactorConfig {
	if c.Interval == 0 {
		c.Interval = 1 * time.Hour
	}
	if c.IdleAfter == 0 {
		c.IdleAfter = 7 * 24 * time.Hour
	}
	if c.CallTimeout == 0 {
		c.CallTimeout = 60 * time.Second
	}
	if c.MaxConvsPerRun == 0 {
		c.MaxConvsPerRun = 5
	}
	return c
}

// compactionPrompt is the exact instruction sent to the LLM. Kept as a
// constant so changes are reviewable; a future change can parametrise it
// once we have data on which phrasings produce the cleanest summaries.
const compactionPrompt = `You are summarising a finished agent conversation so a future session can
re-hydrate the key context without re-reading every tool call.

Produce a tight summary (max 250 words) that captures, in this order:

1. The user's goal — one sentence.
2. Key findings or facts the agent discovered (bullets).
3. Concrete artifacts produced (files written, commands run that mattered, URLs fetched, decisions made).
4. Outstanding questions or unfinished work, if any.

Skip transient errors, retries, and routine acknowledgments. Skip anything
the next session can re-derive from the codebase. Plain prose + bullets,
no headers, no preamble, no markdown fences.

CONVERSATION:

`

// compactorStoreAPI is the slice of store.Store the compactor needs.
type compactorStoreAPI interface {
	LoadConversation(ctx context.Context, id string) (*store.Conversation, error)
	SaveConversation(ctx context.Context, conv store.Conversation) error
	ListCompactableConversations(ctx context.Context, idleBefore time.Time, limit int) ([]string, error)
	DeleteToolOutputsForConversation(ctx context.Context, convID string) (int, error)
}

// ConversationCompactor periodically summarises idle conversations and
// evicts their raw tool_outputs. The summary lives on the conversation row
// (Conversation.CompactedSummary) and is re-injected into the system
// prompt when the conversation is resumed.
//
// Failures are silent (slog.Warn at most) — compaction is a best-effort
// optimisation. A failed run only delays cleanup; nothing breaks.
type ConversationCompactor struct {
	store    compactorStoreAPI
	provider compactorProviderAPI
	cfg      CompactorConfig

	doneCtx context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Optional clock for deterministic tests. Defaults to time.Now.
	now func() time.Time

	stopOnce sync.Once
}

// compactorProviderAPI is the slice of provider.Provider the compactor needs.
type compactorProviderAPI interface {
	Chat(ctx context.Context, req provider.ChatRequest) (*provider.ChatResponse, error)
}

// NewConversationCompactor constructs a compactor. Returns nil when
// cfg.Enabled is false so the caller can write `if c := NewConversationCompactor(...); c != nil { ... }`
// without an extra check.
func NewConversationCompactor(st compactorStoreAPI, prov compactorProviderAPI, cfg CompactorConfig) *ConversationCompactor {
	if !cfg.Enabled {
		return nil
	}
	cfg = applyCompactorDefaults(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	return &ConversationCompactor{
		store:    st,
		provider: prov,
		cfg:      cfg,
		doneCtx:  ctx,
		cancel:   cancel,
		now:      time.Now,
	}
}

// Start launches the periodic ticker. Safe to call once.
func (c *ConversationCompactor) Start() {
	c.wg.Add(1)
	go c.runLoop()
}

// Stop signals the worker to exit and waits up to ctx's deadline for the
// in-flight tick to finish.
func (c *ConversationCompactor) Stop(ctx context.Context) error {
	c.stopOnce.Do(func() { c.cancel() })
	done := make(chan struct{})
	go func() { c.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *ConversationCompactor) runLoop() {
	defer c.wg.Done()
	// Tick once at startup so a long-idle daimon catches up immediately on
	// boot instead of waiting one full Interval.
	c.tick()
	t := time.NewTicker(c.cfg.Interval)
	defer t.Stop()
	for {
		select {
		case <-c.doneCtx.Done():
			return
		case <-t.C:
			c.tick()
		}
	}
}

// tick selects up to MaxConvsPerRun compactable conversations and processes
// each in series. We intentionally avoid parallelism: the LLM summary call
// is the bottleneck and goes through the same provider as the agent loop.
func (c *ConversationCompactor) tick() {
	idleBefore := c.now().Add(-c.cfg.IdleAfter)
	ids, err := c.store.ListCompactableConversations(c.doneCtx, idleBefore, c.cfg.MaxConvsPerRun)
	if err != nil {
		slog.Warn("compactor: list_failed", "error", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	slog.Info("compactor: tick", "candidates", len(ids))
	for _, id := range ids {
		if c.doneCtx.Err() != nil {
			return
		}
		c.compactOne(id)
	}
}

// compactOne summarises a single conversation end-to-end. Each branch maps
// to a failure mode that should not poison the rest of the tick.
func (c *ConversationCompactor) compactOne(convID string) {
	ctx, cancel := context.WithTimeout(c.doneCtx, c.cfg.CallTimeout)
	defer cancel()

	conv, err := c.store.LoadConversation(ctx, convID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			slog.Warn("compactor: load_failed", "conv_id", convID, "error", err)
		}
		return
	}
	if conv.CompactedAt != nil {
		// Race: another tick already handled this one, or the user re-
		// activated it between query and load. Skip.
		return
	}

	prompt := compactionPrompt + serializeConvForCompaction(conv.Messages)
	resp, err := c.provider.Chat(ctx, provider.ChatRequest{
		Model: c.cfg.Model,
		Messages: []provider.ChatMessage{{
			Role:    "user",
			Content: content.Blocks{{Type: content.BlockText, Text: prompt}},
		}},
	})
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			slog.Warn("compactor: provider_timeout", "conv_id", convID)
			return
		}
		slog.Warn("compactor: chat_failed", "conv_id", convID, "error", err)
		return
	}
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		slog.Warn("compactor: empty_response", "conv_id", convID)
		return
	}

	// Re-load + re-check before save: the user may have appended new
	// messages while the LLM call was in flight, in which case the
	// conversation is no longer idle and shouldn't be compacted.
	conv, err = c.store.LoadConversation(ctx, convID)
	if err != nil || conv.CompactedAt != nil {
		return
	}
	if conv.UpdatedAt.After(c.now().Add(-c.cfg.IdleAfter)) {
		slog.Debug("compactor: conv_reactivated_during_summary", "conv_id", convID)
		return
	}

	now := c.now().UTC()
	conv.CompactedAt = &now
	conv.CompactedSummary = summary
	if err := c.store.SaveConversation(ctx, *conv); err != nil {
		slog.Warn("compactor: save_failed", "conv_id", convID, "error", err)
		return
	}

	// Evict raw outputs. A failure here is a soft warning — the summary is
	// already saved and the next pruner sweep can clean orphaned rows.
	deleted, err := c.store.DeleteToolOutputsForConversation(ctx, convID)
	if err != nil {
		slog.Warn("compactor: delete_outputs_failed", "conv_id", convID, "error", err)
		return
	}
	slog.Info("compactor: compacted", "conv_id", convID, "summary_chars", len(summary), "outputs_evicted", deleted)
}

// serializeConvForCompaction renders the conversation messages as plain
// text for the LLM summary prompt. Tool calls and tool results are inlined
// in compact form — they're the bulk of the work we want to summarise.
// Cap at ~30k chars to fit the cheap-model context comfortably.
func serializeConvForCompaction(msgs []provider.ChatMessage) string {
	var b strings.Builder
	const maxBytes = 30_000
	for _, m := range msgs {
		if b.Len() > maxBytes {
			b.WriteString("...(truncated for prompt budget)\n")
			break
		}
		text := strings.TrimSpace(m.Content.TextOnly())
		switch m.Role {
		case "user":
			if text == "" {
				continue
			}
			fmt.Fprintf(&b, "USER: %s\n\n", text)
		case "assistant":
			if text != "" {
				fmt.Fprintf(&b, "ASSISTANT: %s\n", text)
			}
			for _, tc := range m.ToolCalls {
				fmt.Fprintf(&b, "  → tool_call %s(%s)\n", tc.Name, truncateRunes(string(tc.Input), 200))
			}
			if text != "" || len(m.ToolCalls) > 0 {
				b.WriteString("\n")
			}
		case "tool":
			if text == "" {
				continue
			}
			fmt.Fprintf(&b, "  ← tool_result: %s\n\n", truncateRunes(text, 400))
		}
	}
	return b.String()
}

// truncate cuts a string at maxRunes (rune-safe) and appends an ellipsis
// marker so the LLM knows the output was abridged.
func truncateRunes(s string, maxRunes int) string {
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}
