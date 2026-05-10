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
// Task 4.6 — provider 429 retry + sub timeout → timeout wins
// ---------------------------------------------------------------------------

// TestSubagent_TimeoutWinsOverProviderError verifies that when a sub's timeout
// fires while the child is stuck in a long retry loop (simulating 429 backoff),
// the timeout WINS and produces EventSubagentFailed{reason:"budget_exceeded"}
// (the spec mandates "budget_exceeded" for all budget-cap types including timeout).
//
// The mock child agent blocks indefinitely and only exits when its subCtx is done.
// The budget is set with a very short Timeout so the context fires quickly.
// Satisfies proposal §Phase 4 and design §3.1.
func TestSubagent_TimeoutWinsOverProviderError(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	// Track how the child exits.
	var childDoneReason string
	var childMu sync.Mutex

	m := &SubagentManager{
		bus:         bus,
		store:       st,
		mu:          sync.RWMutex{},
		subs:        make(map[string]*subRecord),
		callerIsSub: make(map[string]bool),
	}
	m.softWarnFn = m.injectSoftWarning

	m.newChildAgent = func(
		_ skill.ExecutableSkillDef,
		_ string,
		subCtx context.Context,
		subCh *channel.SubagentChannel,
		_ map[string]tool.Tool,
		_ store.Store,
	) (*Agent, error) {
		inbox := make(chan channel.IncomingMessage, 10)
		_ = subCh.Start(context.Background(), inbox)

		// Simulate a child stuck in a 429 retry loop (blocks until ctx done).
		go func() {
			// Consume the prompt from inbox so Deliver doesn't block.
			select {
			case <-inbox:
			case <-subCtx.Done():
				// ignore
			}

			// Simulate long 429 backoff — keeps blocking until timeout fires.
			<-subCtx.Done()

			childMu.Lock()
			if subCtx.Err() == context.DeadlineExceeded {
				childDoneReason = "timeout"
			} else {
				childDoneReason = "cancelled"
			}
			childMu.Unlock()

			_ = subCh.Stop()
		}()
		return nil, nil
	}
	m.installBusSubscription()

	// Very short timeout (100ms) to trigger the timeout cap quickly.
	def := skill.ExecutableSkillDef{
		Name:        "slow_provider",
		Description: "blocks due to 429 retry",
		Budget: skill.BudgetConfig{
			MaxCostUSD: 10.0,
			MaxTurns:   100,
			Timeout:    100 * time.Millisecond, // fires quickly
		},
	}

	_, spawnErr := m.Spawn(context.Background(), def, "do work", SpawnModeAsync, "conv_parent")
	if spawnErr != nil {
		t.Fatalf("Spawn: %v", spawnErr)
	}

	// Wait for EventSubagentFailed within 3s.
	ev, found := bus.waitForEvent(notify.EventSubagentFailed, 3*time.Second)
	if !found {
		t.Fatal("EventSubagentFailed not emitted within 3s")
	}

	// Reason must be "budget_exceeded" (timeout-cap, not "provider_error").
	reason := ev.Meta["reason"]
	if reason != "budget_exceeded" {
		t.Errorf("reason = %q, want 'budget_exceeded' (timeout wins over provider error)", reason)
	}

	// The child exited via DeadlineExceeded (timeout), not cancellation.
	time.Sleep(50 * time.Millisecond) // let goroutine record reason
	childMu.Lock()
	dr := childDoneReason
	childMu.Unlock()
	if dr != "timeout" {
		t.Errorf("childDoneReason = %q, want 'timeout'", dr)
	}
}

// ---------------------------------------------------------------------------
// Task 4.6b — budget_low skill: fires EventSubagentFailed{reason:budget_exceeded}
// ---------------------------------------------------------------------------

// TestSubagent_BudgetLow_OneFailedEvent verifies that a skill with a very low
// cost cap produces exactly one EventSubagentFailed{reason:"budget_exceeded"}.
// This is the standard budget enforcement test (mirrors 2.18). Satisfies
// SUBAGENTS-REQ-5 and REQ-13.
func TestSubagent_BudgetLow_OneFailedEvent(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	def := skill.ExecutableSkillDef{
		Name:        "budget_low",
		Description: "very low budget",
		Budget: skill.BudgetConfig{
			MaxCostUSD: 0.0001, // exceeded on first turn
			MaxTurns:   100,
			Timeout:    10 * time.Second,
		},
	}

	handle, err := m.Spawn(context.Background(), def, "do work", SpawnModeAsync, "conv_parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Emit one turn that exceeds the $0.0001 cap.
	bus.Emit(notify.Event{
		Type:      notify.EventTurnCompleted,
		ChannelID: handle.rec.subChannel.ID(),
		Timestamp: time.Now(),
		Meta:      map[string]string{"cost_usd": "0.001"}, // 10x the cap
	})

	ev, found := bus.waitForEvent(notify.EventSubagentFailed, 3*time.Second)
	if !found {
		t.Fatal("EventSubagentFailed not emitted within 3s")
	}
	if ev.Meta["reason"] != "budget_exceeded" {
		t.Errorf("reason = %q, want 'budget_exceeded'", ev.Meta["reason"])
	}

	// Wait for done channel.
	select {
	case <-handle.rec.done:
		// Expected.
	case <-time.After(2 * time.Second):
		t.Error("done channel not closed after budget exceeded")
	}

	// Conversation status must be "failed".
	st.mu.Lock()
	convStatus := st.status
	st.mu.Unlock()
	if convStatus != "failed" {
		t.Errorf("conversation status = %q, want 'failed'", convStatus)
	}
}
