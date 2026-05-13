package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"daimon/internal/channel"
	"daimon/internal/notify"
	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// Task 4.5 — parent ctx cancelled during spawn (race condition)
// ---------------------------------------------------------------------------

// TestSubagent_ParentCtxCancelDuringSpawn verifies the race described in
// design §3.1: if the parent context is cancelled AFTER the conversation row
// is persisted but BEFORE the child agent starts, SubagentManager must:
//   - emit EventSubagentFailed{reason:"cancelled_during_spawn"}
//   - not start any goroutine (child should never run)
//   - leave the subRecord out of subs (or set to cancelled status)
//
// Satisfies design §3.1 race note.
func TestSubagent_ParentCtxCancelDuringSpawn(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	// Create a parent context that we cancel after the store write.
	parentCtx, parentCancel := context.WithCancel(context.Background())

	m := &SubagentManager{
		bus:         bus,
		store:       st,
		mu:          sync.RWMutex{},
		subs:        make(map[string]*subRecord),
		callerIsSub: make(map[string]bool),
	}
	m.softWarnFn = m.injectSoftWarning

	// childStarted tracks whether the child agent goroutine was launched.
	var childStarted bool
	var childMu sync.Mutex

	m.newChildAgent = func(
		_ skill.ExecutableSkillDef,
		_ string,
		_ context.Context,
		subCh *channel.SubagentChannel,
		_ map[string]tool.Tool,
		_ store.Store,
	) (*Agent, error) {
		childMu.Lock()
		childStarted = true
		childMu.Unlock()
		inbox := make(chan channel.IncomingMessage, 10)
		_ = subCh.Start(context.Background(), inbox)
		return nil, nil
	}
	m.installBusSubscription()

	// Cancel the parent context BEFORE calling Spawn to simulate the race.
	// The manager checks ctx.Err() after the store write (step 5 in Spawn).
	parentCancel()

	def := skill.ExecutableSkillDef{
		Name:        "researcher",
		Description: "test",
		Budget: skill.BudgetConfig{
			MaxCostUSD: 1.0,
			MaxTurns:   10,
			Timeout:    5 * time.Second,
		},
	}

	_, err := m.Spawn(parentCtx, def, "do work", SpawnModeAsync, "conv_parent")
	// Spawn should return an error (context already cancelled).
	if err == nil {
		t.Error("expected error from Spawn when parent ctx already cancelled, got nil")
	}

	// Wait briefly for the event to propagate.
	time.Sleep(100 * time.Millisecond)

	// EventSubagentFailed with reason="cancelled_during_spawn" must be emitted.
	failEvents := bus.eventsOfType(notify.EventSubagentFailed)
	if len(failEvents) == 0 {
		t.Fatal("expected EventSubagentFailed to be emitted, got none")
	}
	reason := failEvents[0].Meta["reason"]
	if reason != "cancelled_during_spawn" {
		t.Errorf("reason = %q, want 'cancelled_during_spawn'", reason)
	}

	// The child agent must NOT have been started (no goroutine leak).
	childMu.Lock()
	started := childStarted
	childMu.Unlock()
	if started {
		t.Error("child agent goroutine was started despite parent ctx cancellation")
	}
}

// ---------------------------------------------------------------------------
// Task 4.7 — tool_result metadata contains subagent_id + batch_id
// ---------------------------------------------------------------------------

// TestSubagentResult_MetadataContainsIDs verifies that SubagentResult.Metadata
// carries subagent_id and batch_id so the parent's tool_result contains them.
// Satisfies SUBAGENTS-REQ-8.
func TestSubagentResult_MetadataContainsIDs(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, fake := newTestManager(t, bus, st)
	// Wire the fake to emit EventTurnCompleted after Send (what a real child's
	// loop.go does). Without this, budgetMonitor never gets the event that
	// triggers the FinalAssistant completion check and Wait hangs until the
	// per-spawn ctx times out — a 5-minute test stall in CI (see CI failure
	// on PR #9: TestSubagentResult_MetadataContainsIDs (4m46s)).
	fake.emitBus = bus
	m.installBusSubscription()

	// Tight Wait deadline so any future regression here fails in seconds
	// rather than burning the package-level 5-minute timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	handle, err := m.Spawn(ctx, defaultDef("researcher"), "do research", SpawnModeSync, "conv_parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Wait for completion.
	result, waitErr := handle.Wait(ctx)
	if waitErr != nil {
		t.Fatalf("Wait: %v", waitErr)
	}
	if result == nil {
		t.Fatal("result is nil")
	}

	if result.Metadata == nil {
		t.Fatal("result.Metadata is nil — subagent_id and batch_id not present")
	}
	if result.Metadata["subagent_id"] == "" {
		t.Error("result.Metadata[subagent_id] is empty")
	}
	if result.Metadata["batch_id"] == "" {
		t.Error("result.Metadata[batch_id] is empty")
	}
	// Values must match handle IDs.
	if result.Metadata["subagent_id"] != handle.ID {
		t.Errorf("result.Metadata[subagent_id] = %q, want %q", result.Metadata["subagent_id"], handle.ID)
	}
	if result.Metadata["batch_id"] != handle.BatchID {
		t.Errorf("result.Metadata[batch_id] = %q, want %q", result.Metadata["batch_id"], handle.BatchID)
	}
}
