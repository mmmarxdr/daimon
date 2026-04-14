package notify

import "testing"

func TestKnownEventTypes_ContainsExpected(t *testing.T) {
	expected := []string{
		EventCronJobFired,
		EventCronJobCompleted,
		EventCronJobFailed,
		EventTurnStarted,
		EventTurnCompleted,
		EventContextCompacted,
	}
	for _, ev := range expected {
		if !KnownEventTypes[ev] {
			t.Errorf("KnownEventTypes missing %q", ev)
		}
	}
}

func TestEventContextCompacted_Value(t *testing.T) {
	if EventContextCompacted != "agent.context.compacted" {
		t.Errorf("EventContextCompacted = %q, want %q", EventContextCompacted, "agent.context.compacted")
	}
}
