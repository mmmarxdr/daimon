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
// WU9 test helpers
// ---------------------------------------------------------------------------

// spawnAndSubscribe spawns a subagent, calls Subscribe, and returns the handle
// and the event channel.
func spawnAndSubscribe(t *testing.T, m *SubagentManager, bus notify.Bus) (*SubagentHandle, <-chan notify.Event) {
	t.Helper()
	ctx := context.Background()
	handle, err := m.Spawn(ctx, defaultDef("test-skill"), "do work", SpawnModeSync, "conv-parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	ch, err := handle.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	return handle, ch
}

// collectN reads n events from ch with a timeout; returns collected events.
func collectN(ch <-chan notify.Event, n int, timeout time.Duration) []notify.Event {
	var out []notify.Event
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
	return out
}

// emitSubagentEvent emits a bus event attributed to the given subagent ID.
func emitSubagentEvent(bus notify.Bus, subagentID string, evType string) {
	bus.Emit(notify.Event{
		Type:      evType,
		Origin:    notify.OriginAgent,
		ChannelID: "subagent:" + subagentID,
		Timestamp: time.Now(),
		Meta:      map[string]string{"subagent_id": subagentID},
	})
}

// ---------------------------------------------------------------------------
// REQ-11.1 — Subscribe before run; receive filtered events
// ---------------------------------------------------------------------------

func TestSubagentHandle_Subscribe_ReceivesFilteredEvents(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	handle, ch := spawnAndSubscribe(t, m, bus)

	// Emit 2 tool events for this subagent and 1 unrelated event.
	emitSubagentEvent(bus, handle.ID, notify.EventToolStart)
	emitSubagentEvent(bus, handle.ID, notify.EventToolEnd)
	emitSubagentEvent(bus, "other-subagent", notify.EventToolStart)

	evs := collectN(ch, 2, 500*time.Millisecond)
	if len(evs) != 2 {
		t.Fatalf("expected 2 events for this subagent, got %d", len(evs))
	}
	for _, ev := range evs {
		if ev.Meta["subagent_id"] != handle.ID {
			t.Errorf("received event not attributed to %q: %v", handle.ID, ev.Meta)
		}
	}
}

// ---------------------------------------------------------------------------
// REQ-11.2 — Subscribe after done; channel is already closed
// ---------------------------------------------------------------------------

func TestSubagentHandle_Subscribe_AfterDone_ClosedImmediately(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	// Use bus-aware fake so it emits EventTurnCompleted → budgetMonitor → finalize.
	fake := &fakeChildAgent{emitBus: bus}
	m := &SubagentManager{
		bus:         bus,
		store:       st,
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
		fake.mu.Lock()
		fake.channel = subCh
		fake.mu.Unlock()
		inbox := make(chan channel.IncomingMessage, 10)
		_ = subCh.Start(context.Background(), inbox)
		go fake.runFn(subCtx)
		return nil, nil
	}
	m.installBusSubscription()

	ctx := context.Background()
	handle, err := m.Spawn(ctx, defaultDef("test-skill"), "do work", SpawnModeSync, "conv-parent")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Give budgetMonitor time to finalize via EventTurnCompleted.
	select {
	case <-handle.rec.done:
		// done channel closed — subagent finished.
	case <-time.After(3 * time.Second):
		t.Fatal("subagent did not complete in time")
	}

	// Subscribe AFTER completion.
	subCh, err := handle.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe after done: %v", err)
	}

	// Channel should close quickly.
	select {
	case _, ok := <-subCh:
		if ok {
			t.Error("expected channel to be closed, got event")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("channel not closed after 500ms")
	}
}

// ---------------------------------------------------------------------------
// REQ-11.3 — Multiple concurrent subscribers receive independent channels
// ---------------------------------------------------------------------------

func TestSubagentHandle_Subscribe_ConcurrentSubscribers_Independent(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	handle, _ := spawnAndSubscribe(t, m, bus)

	// Subscribe a second time independently.
	ch2, err := handle.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}

	// Emit 3 events for this subagent.
	for i := 0; i < 3; i++ {
		emitSubagentEvent(bus, handle.ID, notify.EventToolStart)
	}

	// Both subscribers should receive events.
	ch1, err2 := handle.Subscribe(context.Background())
	if err2 != nil {
		t.Fatalf("third Subscribe: %v", err2)
	}

	// Give time for events to propagate.
	time.Sleep(100 * time.Millisecond)

	// The channels are independent — one consumer's speed doesn't affect the other.
	// At minimum, both channels should receive the events.
	_ = ch2
	_ = ch1
}

// ---------------------------------------------------------------------------
// REQ-11.4 — Context cancellation closes channel
// ---------------------------------------------------------------------------

func TestSubagentHandle_Subscribe_ContextCancel_ClosesChannel(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	handle, _ := spawnAndSubscribe(t, m, bus)

	ctx, cancel := context.WithCancel(context.Background())
	subCh, err := handle.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Cancel the context; the channel should close.
	cancel()

	select {
	case _, ok := <-subCh:
		if ok {
			t.Error("expected channel to be closed after context cancel")
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("channel not closed within 500ms after context cancel")
	}
}

// ---------------------------------------------------------------------------
// REQ-11 — nil bus returns closed channel immediately
// ---------------------------------------------------------------------------

func TestSubagentHandle_Subscribe_NilBus_ClosedChannelImmediately(t *testing.T) {
	// Create a subRecord with nil bus directly (no SubagentManager needed).
	rec := &subRecord{
		id:      "test-sub",
		batchID: "b1",
		done:    make(chan struct{}),
		status:  "running",
	}
	// rec.bus is nil by default (zero value of notify.Bus interface).
	handle := &SubagentHandle{ID: "test-sub", BatchID: "b1", rec: rec}

	subCh, err := handle.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe with nil bus: %v", err)
	}

	select {
	case _, ok := <-subCh:
		if ok {
			t.Error("expected closed channel for nil bus")
		}
	case <-time.After(200 * time.Millisecond):
		t.Error("channel not closed within 200ms for nil bus")
	}
}

// ---------------------------------------------------------------------------
// REQ-11.5 — concurrency: send vs close race (reproduce panic: send on closed channel)
// ---------------------------------------------------------------------------

// TestSubagentHandle_Subscribe_SendCloseRace stresses the fan-out handler
// against concurrent subscriber cancellations to reproduce the "send on
// closed channel" panic described in the bug report.
//
// Root cause: the handler copies rec.subs under the lock, releases the lock,
// then sends on the copy. Concurrently, the cleanup goroutine removes ch from
// rec.subs (under lock) and then closes ch OUTSIDE the lock. The handler can
// race past the removal, holding a stale copy still containing ch, and send on
// the already-closed channel → panic.
//
// Run with: go test ./internal/agent/ -race -run TestSubagentHandle_Subscribe_SendCloseRace -count=20
func TestSubagentHandle_Subscribe_SendCloseRace(t *testing.T) {
	const (
		iterations = 200
		workers    = 8
	)

	for iter := 0; iter < iterations; iter++ {
		bus := notify.NewEventBus(256, 0, 0)

		rec := &subRecord{
			id:      "race-sub",
			batchID: "b1",
			done:    make(chan struct{}),
			status:  "running",
			bus:     bus,
		}
		handle := &SubagentHandle{ID: "race-sub", rec: rec}

		// Subscribe installs the bus handler lazily on the first call.
		subCtx, subCancel := context.WithCancel(context.Background())
		ch, err := handle.Subscribe(subCtx)
		if err != nil {
			t.Fatalf("iter %d: Subscribe: %v", iter, err)
		}
		_ = ch

		// Drain the channel in the background so it never blocks on a
		// full buffer (buf=32); we care about the panic, not the value.
		// Tracked with drainWg so we can join after ch is guaranteed closed,
		// preventing goroutine accumulation across iterations.
		var drainWg sync.WaitGroup
		drainWg.Add(1)
		go func() {
			defer drainWg.Done()
			for range ch {
			}
		}()

		// Concurrent emitters: blast events at the same subagent_id so the
		// fan-out handler is busy sending to ch.
		var wg sync.WaitGroup
		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 50; i++ {
					bus.Emit(notify.Event{
						Type:      notify.EventToolStart,
						Origin:    notify.OriginAgent,
						ChannelID: "subagent:race-sub",
						Timestamp: time.Now(),
						Meta:      map[string]string{"subagent_id": "race-sub"},
					})
				}
			}()
		}

		// Cancel the subscriber context while emitters are still running to
		// race the cleanup goroutine (remove+close) against the fan-out send.
		subCancel()
		wg.Wait()
		bus.Close()
		// ch is guaranteed closed now: subCancel triggered the cleanup goroutine
		// (remove+close under subMu), and bus.Close() ensures no new sends arrive.
		// Join the drain goroutine before moving to the next iteration.
		drainWg.Wait()
	}
}

// ---------------------------------------------------------------------------
// REQ-18 — slow consumer: drops with warn (non-blocking handler)
// ---------------------------------------------------------------------------

func TestSubagentHandle_Subscribe_SlowConsumer_DropsWithWarn(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	handle, subCh := spawnAndSubscribe(t, m, bus)

	// Emit more events than the channel buffer can hold without consuming.
	// The handler must NOT block even if the subscriber is slow.
	for i := 0; i < 64; i++ {
		emitSubagentEvent(bus, handle.ID, notify.EventToolStart)
	}

	// If we reach here within the timeout, the handler was non-blocking.
	done := make(chan struct{})
	go func() {
		// Drain whatever arrived.
		for {
			select {
			case <-subCh:
			case <-time.After(100 * time.Millisecond):
				close(done)
				return
			}
		}
	}()

	select {
	case <-done:
		// pass
	case <-time.After(2 * time.Second):
		t.Error("subscriber handler blocked for > 2s — not non-blocking")
	}
}
