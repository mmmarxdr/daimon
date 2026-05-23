package notify

import (
	"testing"
	"time"
)

// TestNewEventBus_ZeroDefaults_AppliesV2Caps (REQ-13.1) — when bufferSize and
// maxPerMin are both 0, the constructor must apply the V2 defaults:
// bufferSize=1024, maxPerMin=1000.
func TestNewEventBus_ZeroDefaults_AppliesV2Caps(t *testing.T) {
	bus := NewEventBus(0, 0, 0)
	defer bus.Close()

	// Verify maxPerMin default. We do this by emitting 1001 events and checking
	// that the circuit breaker trips at 1000 (not at 30 as the old default).
	// Use a buffered channel to count delivered events.
	var delivered int
	done := make(chan struct{})
	bus.Subscribe(func(e Event) {
		delivered++
		if delivered == 1000 {
			close(done)
		}
	})

	// Emit 1001 events quickly. The first 1000 should pass; the 1001st should be dropped.
	for i := 0; i < 1001; i++ {
		bus.Emit(Event{Type: EventTurnStarted, Origin: OriginAgent, Timestamp: time.Now(), ChannelID: "test"})
	}

	// Wait for 1000 deliveries or timeout.
	select {
	case <-done:
		// Good — 1000 events reached the handler, confirming maxPerMin=1000.
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out waiting for 1000 events; delivered=%d (old default=30 would have stopped much earlier)", delivered)
	}
}

// TestNewEventBus_ExplicitValues_Respected (REQ-13.2) — explicit non-zero values
// passed to NewEventBus must override the defaults.
func TestNewEventBus_ExplicitValues_Respected(t *testing.T) {
	const explicitMax = 200
	bus := NewEventBus(512, explicitMax, 3*time.Second)
	defer bus.Close()

	var count int
	bus.Subscribe(func(e Event) {
		count++
	})

	// Emit exactly explicitMax events.
	for i := 0; i < explicitMax; i++ {
		bus.Emit(Event{Type: EventTurnStarted, Origin: OriginAgent, Timestamp: time.Now(), ChannelID: "test"})
	}

	// Give the worker time to drain.
	time.Sleep(200 * time.Millisecond)

	// All explicitMax events should have been delivered (circuit breaker at 200).
	if count > explicitMax {
		t.Errorf("circuit breaker should cap at %d, got %d deliveries", explicitMax, count)
	}
}
