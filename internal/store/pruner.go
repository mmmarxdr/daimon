package store

import (
	"context"
	"log/slog"
	"time"
)

// ConversationPruner physically removes conversations that have been
// soft-deleted for longer than the configured retention window. It runs a
// ticker-driven goroutine inside the daimon web server; Start launches it,
// Stop cancels it cleanly. The first Tick happens one Interval after Start —
// the pruner does NOT run at startup (avoid surprise deletion on fresh
// boots; a manual one-shot admin command is a follow-up).
type ConversationPruner struct {
	store ConvPruneStore
	clock Clock
	cfg   PrunerConfig

	cancel context.CancelFunc
	done   chan struct{}
}

// ConvPruneStore is the narrow store interface the pruner needs. Implemented
// by SQLiteStore. Keeping the interface tiny makes the pruner trivially
// testable with a fake.
type ConvPruneStore interface {
	DeleteConversationsOlderThan(ctx context.Context, cutoff time.Time) (int, error)
	// DeleteToolOutputsBefore removes orphan tool_output rows older than
	// cutoff (rows whose conversation_id is unknown or whose conversation
	// was already compacted-and-evicted, but the FTS row lingered). Returns
	// the number of rows removed.
	DeleteToolOutputsBefore(ctx context.Context, cutoff time.Time) (int, error)
}

// PrunerConfig is the already-clamped runtime config for the pruner. Callers
// (wiring code in server.go) build this from config.PruneConfig +
// ApplyDefaults.
type PrunerConfig struct {
	Enabled   bool
	Retention time.Duration
	Interval  time.Duration
}

// NewConversationPruner wires a pruner. Does NOT start the goroutine — call
// Start. Expected to be called from server startup with the server's own
// parent context.
func NewConversationPruner(store ConvPruneStore, clock Clock, cfg PrunerConfig) *ConversationPruner {
	return &ConversationPruner{store: store, clock: clock, cfg: cfg}
}

// Start launches the ticker goroutine when Enabled. Safe to call once;
// calling a second time without a Stop in between is undefined behavior.
func (p *ConversationPruner) Start(parent context.Context) {
	if !p.cfg.Enabled {
		slog.Info("conversation_pruner_disabled")
		return
	}
	ctx, cancel := context.WithCancel(parent)
	p.cancel = cancel
	p.done = make(chan struct{})
	go p.loop(ctx)
}

// Stop cancels the pruner's internal context and waits for the goroutine to
// exit. Idempotent. Safe to call even if Start was not called (no-op).
func (p *ConversationPruner) Stop() {
	if p.cancel == nil {
		return
	}
	p.cancel()
	<-p.done
	p.cancel = nil
}

func (p *ConversationPruner) loop(ctx context.Context) {
	defer close(p.done)
	ticker := time.NewTicker(p.cfg.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.Tick(ctx)
		}
	}
}

// Tick runs one prune pass. Exposed so tests can drive with a FakeClock
// without waiting on real time.
//
// Two cleanups happen per tick, both bounded by the same retention window:
//  1. Hard-delete soft-deleted conversations older than the cutoff.
//  2. Drop orphan tool_outputs older than the cutoff. Catches rows from
//     before conversation_id was tracked (legacy, conversation_id='') and
//     rows whose owning conversation was hard-deleted in step 1 but whose
//     FTS rows lingered.
func (p *ConversationPruner) Tick(ctx context.Context) {
	start := p.clock.Now()
	cutoff := start.Add(-p.cfg.Retention)

	convDeleted, err := p.store.DeleteConversationsOlderThan(ctx, cutoff)
	if err != nil {
		slog.Error("pruner_run_error", "stage", "conversations", "error", err,
			"duration_ms", time.Since(start).Milliseconds())
		return
	}

	outputsDeleted, err := p.store.DeleteToolOutputsBefore(ctx, cutoff)
	if err != nil {
		slog.Error("pruner_run_error", "stage", "tool_outputs", "error", err,
			"duration_ms", time.Since(start).Milliseconds())
		return
	}

	slog.Info("pruner_run",
		"deleted_conversations", convDeleted,
		"deleted_tool_outputs", outputsDeleted,
		"duration_ms", time.Since(start).Milliseconds())
}
