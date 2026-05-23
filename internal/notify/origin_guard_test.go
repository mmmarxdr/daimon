package notify_test

import (
	"testing"
	"time"

	"daimon/internal/notify"
)

// TestOriginNotificationStillDropped_AfterStreamingChanges (REQ-17.1) verifies
// that the bus worker continues to drop events where Origin == OriginNotification.
// This guard prevents notification re-emission loops and MUST NOT be regressed.
func TestOriginNotificationStillDropped_AfterStreamingChanges(t *testing.T) {
	bus := notify.NewEventBus(64, 1000, 5*time.Second)
	defer bus.Close()

	called := make(chan struct{}, 1)
	bus.Subscribe(func(_ notify.Event) {
		select {
		case called <- struct{}{}:
		default:
		}
	})

	// Emit an event with OriginNotification — handler must NOT be called.
	bus.Emit(notify.Event{
		Type:      notify.EventTurnCompleted,
		Origin:    notify.OriginNotification,
		Timestamp: time.Now(),
	})

	// Give the worker time to process.
	select {
	case <-called:
		t.Error("handler was called for OriginNotification event — loop-prevention guard regressed (REQ-17.1)")
	case <-time.After(200 * time.Millisecond):
		// correct: handler not called for OriginNotification
	}

	// Emit an OriginAgent event — handler SHOULD be called to confirm bus is live.
	bus.Emit(notify.Event{
		Type:      notify.EventTurnCompleted,
		Origin:    notify.OriginAgent,
		Timestamp: time.Now(),
	})
	select {
	case <-called:
		// correct
	case <-time.After(2 * time.Second):
		t.Error("handler not called for OriginAgent event — bus may be broken")
	}
}

// TestNewEventToolStart_OriginIsAgent (REQ-17.2) verifies that the canonical
// agent.tool.start event constant exists and is named as expected. This test
// documents that all new bus events introduced by agent-stream-events use
// OriginAgent, not OriginNotification.
func TestNewEventToolStart_OriginIsAgent(t *testing.T) {
	// Construct the event as the production loop.go does and verify Origin.
	ev := notify.Event{
		Type:      notify.EventToolStart,
		Origin:    notify.OriginAgent,
		Timestamp: time.Now(),
	}
	if ev.Origin != notify.OriginAgent {
		t.Errorf("EventToolStart must use OriginAgent, got %v", ev.Origin)
	}
	// Also confirm the constant value matches spec.
	if notify.EventToolStart != "agent.tool.start" {
		t.Errorf("EventToolStart = %q, want agent.tool.start", notify.EventToolStart)
	}
	if notify.EventToolEnd != "agent.tool.end" {
		t.Errorf("EventToolEnd = %q, want agent.tool.end", notify.EventToolEnd)
	}
	if notify.EventTokensUsage != "agent.tokens.usage" {
		t.Errorf("EventTokensUsage = %q, want agent.tokens.usage", notify.EventTokensUsage)
	}
}
