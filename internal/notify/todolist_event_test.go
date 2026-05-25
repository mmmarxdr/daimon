package notify

import "testing"

// TestEventTodolistChanged_InKnownEventTypes (REQ-7) — the todolist.changed event
// must be present in KnownEventTypes.
func TestEventTodolistChanged_InKnownEventTypes(t *testing.T) {
	if !KnownEventTypes[EventTodolistChanged] {
		t.Errorf("KnownEventTypes missing %q", EventTodolistChanged)
	}
}

// TestEventTodolistChanged_Value verifies the exact constant string value.
func TestEventTodolistChanged_Value(t *testing.T) {
	if EventTodolistChanged != "agent.todolist.changed" {
		t.Errorf("EventTodolistChanged = %q, want %q", EventTodolistChanged, "agent.todolist.changed")
	}
}

// TestEventTodolistChanged_NotInStreamingSkipSet (REQ-7) — the todolist.changed
// event MUST NOT appear in StreamingSkipSet (it is a real domain event, not a
// high-frequency streaming boundary event).
func TestEventTodolistChanged_NotInStreamingSkipSet(t *testing.T) {
	if StreamingSkipSet[EventTodolistChanged] {
		t.Errorf("StreamingSkipSet must NOT contain %q", EventTodolistChanged)
	}
}
