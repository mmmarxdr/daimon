package notify

import "testing"

// TestKnownEventTypes_NewBusRoutedConstants_Present (REQ-12.1) — the 5 new
// bus-routed event type constants must be present in KnownEventTypes.
func TestKnownEventTypes_NewBusRoutedConstants_Present(t *testing.T) {
	busRouted := []string{
		EventToolStart,
		EventToolEnd,
		EventReasoningStart,
		EventReasoningEnd,
		EventTokensUsage,
	}
	for _, ev := range busRouted {
		if !KnownEventTypes[ev] {
			t.Errorf("KnownEventTypes missing bus-routed constant %q", ev)
		}
	}
}

// TestKnownEventTypes_InterfaceOnlyConstants_Absent (REQ-12.2) — the 3
// interface-only constants must NOT be in KnownEventTypes.
func TestKnownEventTypes_InterfaceOnlyConstants_Absent(t *testing.T) {
	interfaceOnly := []string{
		EventMessageChunk,
		EventReasoningDelta,
		EventToolDelta,
	}
	for _, ev := range interfaceOnly {
		if KnownEventTypes[ev] {
			t.Errorf("KnownEventTypes should NOT contain interface-only constant %q", ev)
		}
	}
}

// TestStreamingSkipSet_ContainsAllFiveBusRoutedNewTypes (REQ-12.3) — the
// exported StreamingSkipSet must contain all 5 new bus-routed types so the
// rules engine can skip them.
func TestStreamingSkipSet_ContainsAllFiveBusRoutedNewTypes(t *testing.T) {
	required := []string{
		EventToolStart,
		EventToolEnd,
		EventReasoningStart,
		EventReasoningEnd,
		EventTokensUsage,
	}
	for _, ev := range required {
		if !StreamingSkipSet[ev] {
			t.Errorf("StreamingSkipSet missing %q", ev)
		}
	}
}
