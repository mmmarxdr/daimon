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

// TestSubagentCancelCascade_ParentCtxCancelled spawns 3 subagents, cancels
// the parent context, and asserts all 3 done channels close within 1 second
// and EventSubagentFailed{reason:"cancelled"} is emitted for each.
// Satisfies SUBAGENTS-REQ-6 and AGENT-LOOP-REQ-6.
func TestSubagentCancelCascade_ParentCtxCancelled(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	parentCtx, parentCancel := context.WithCancel(context.Background())

	// Build a manager with a test seam that blocks until the sub context is done.
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
		// Start the channel.
		inbox := make(chan channel.IncomingMessage, 10)
		_ = subCh.Start(context.Background(), inbox)
		// The "child" just blocks until its context is cancelled.
		go func() {
			<-subCtx.Done()
			// On cancellation, Stop the channel (simulates child loop exit).
			_ = subCh.Stop()
		}()
		return nil, nil
	}
	m.installBusSubscription()

	// Spawn 3 subagents.
	const numSubs = 3
	handles := make([]*SubagentHandle, numSubs)
	def := skill.ExecutableSkillDef{
		Name:        "blocker",
		Description: "blocks until cancelled",
		Budget: skill.BudgetConfig{
			MaxCostUSD: 1.0,
			MaxTurns:   100,
			Timeout:    10 * time.Second,
		},
	}

	for i := 0; i < numSubs; i++ {
		h, err := m.Spawn(parentCtx, def, "block", SpawnModeAsync, "conv_parent")
		if err != nil {
			t.Fatalf("Spawn %d: %v", i, err)
		}
		handles[i] = h
	}

	// Cancel parent context.
	parentCancel()

	// All 3 done channels must close within 1 second.
	deadline := time.Now().Add(1 * time.Second)
	for i, h := range handles {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Errorf("sub %d: deadline passed before done channel closed", i)
			continue
		}
		select {
		case <-h.rec.done:
			// Good.
		case <-time.After(remaining):
			t.Errorf("sub %d: done channel not closed within 1s after parent cancel", i)
		}
	}

	// Wait a moment for events to propagate.
	time.Sleep(100 * time.Millisecond)

	failEvents := bus.eventsOfType(notify.EventSubagentFailed)
	if len(failEvents) < numSubs {
		t.Errorf("expected %d EventSubagentFailed events, got %d", numSubs, len(failEvents))
	}

	// Each event must carry reason "cancelled" (parent ctx cancel) or
	// "budget_exceeded" (timeout_min cap per spec REQ-5). The old "timeout"
	// value is retired — all budget-cap breaches now use "budget_exceeded".
	for _, ev := range failEvents {
		reason := ev.Meta["reason"]
		if reason != "cancelled" && reason != "budget_exceeded" {
			t.Errorf("EventSubagentFailed reason = %q, want 'cancelled' or 'budget_exceeded'", reason)
		}
	}
}

// TestSubagentCancelCascade_TableDriven runs the cancel cascade test with
// 1, 5, and 10 children as specified by the tasks.
func TestSubagentCancelCascade_TableDriven(t *testing.T) {
	cases := []struct {
		name    string
		numSubs int
	}{
		{"1_child", 1},
		{"5_children", 5},
		{"10_children", 10},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus := newBusRecorder()
			t.Cleanup(func() { bus.Close() })
			st := &mockStore{}

			parentCtx, parentCancel := context.WithCancel(context.Background())
			t.Cleanup(parentCancel)

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
				go func() {
					<-subCtx.Done()
					_ = subCh.Stop()
				}()
				return nil, nil
			}
			m.installBusSubscription()

			def := skill.ExecutableSkillDef{
				Name: "blocker",
				Budget: skill.BudgetConfig{
					MaxCostUSD: 1.0,
					MaxTurns:   100,
					Timeout:    10 * time.Second,
				},
			}

			handles := make([]*SubagentHandle, tc.numSubs)
			for i := 0; i < tc.numSubs; i++ {
				h, err := m.Spawn(parentCtx, def, "block", SpawnModeAsync, "conv_parent")
				if err != nil {
					t.Fatalf("Spawn %d: %v", i, err)
				}
				handles[i] = h
			}

			parentCancel()

			deadline := time.Now().Add(1 * time.Second)
			for i, h := range handles {
				remaining := time.Until(deadline)
				if remaining <= 0 {
					t.Errorf("sub %d: deadline passed", i)
					continue
				}
				select {
				case <-h.rec.done:
				case <-time.After(remaining):
					t.Errorf("sub %d: not done within 1s", i)
				}
			}
		})
	}
}
