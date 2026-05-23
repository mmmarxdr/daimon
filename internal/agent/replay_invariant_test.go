package agent

import (
	"testing"
	"time"

	"daimon/internal/notify"
)

// TestBus_LateSubscriber_DoesNotReceiveHistoricalEvents (REQ-16.1) verifies
// that a handler registered AFTER events have been emitted does NOT receive
// those events. Replay / catch-up is an explicit non-goal in V1.
func TestBus_LateSubscriber_DoesNotReceiveHistoricalEvents(t *testing.T) {
	bus := notify.NewEventBus(256, 1000, 5*time.Second)
	defer bus.Close()

	// Emit 10 events before registering the late subscriber.
	for i := 0; i < 10; i++ {
		bus.Emit(notify.Event{
			Type:      notify.EventToolStart,
			Origin:    notify.OriginAgent,
			Timestamp: time.Now(),
		})
	}

	// Give the bus worker time to process those events.
	time.Sleep(50 * time.Millisecond)

	// Now register a late subscriber.
	lateReceived := make(chan notify.Event, 32)
	bus.Subscribe(func(ev notify.Event) {
		select {
		case lateReceived <- ev:
		default:
		}
	})

	// Emit one more event that the late subscriber SHOULD receive.
	bus.Emit(notify.Event{
		Type:      notify.EventToolEnd,
		Origin:    notify.OriginAgent,
		Timestamp: time.Now(),
	})

	// Wait for the post-subscription event.
	select {
	case ev := <-lateReceived:
		if ev.Type != notify.EventToolEnd {
			t.Errorf("got unexpected event type %q before EventToolEnd", ev.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("late subscriber did not receive post-subscription event within 2s")
	}

	// Drain for a brief window — there must be no additional events (historical replay).
	drainTimeout := time.After(150 * time.Millisecond)
	historyCount := 0
drainLoop:
	for {
		select {
		case <-lateReceived:
			historyCount++
		case <-drainTimeout:
			break drainLoop
		}
	}
	if historyCount > 0 {
		t.Errorf("late subscriber received %d historical events (replay must not occur — REQ-16)", historyCount)
	}
}
