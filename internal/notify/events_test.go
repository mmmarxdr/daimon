package notify

import (
	"encoding/json"
	"testing"
)

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

// ---------------------------------------------------------------------------
// Seam 2 — ADR-2: SysToks/MsgToks/ToolToks fields on Event (RED → GREEN)
// ---------------------------------------------------------------------------

// TestEvent_CategoryFields_OmitEmpty: zero-value category fields are absent from
// JSON; non-zero values produce keys in JSON.
func TestEvent_CategoryFields_OmitEmpty(t *testing.T) {
	// Zero values — keys must be absent.
	ev := Event{Type: EventTokensUsage, ChannelID: "ch-1"}
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	for _, key := range []string{"sys_toks", "msg_toks", "tool_toks"} {
		if _, ok := m[key]; ok {
			t.Errorf("JSON contains key %q with zero value, want absent (omitempty)", key)
		}
	}

	// Non-zero values — keys must be present.
	ev2 := Event{
		Type:      EventTokensUsage,
		ChannelID: "ch-1",
		SysToks:   1500,
		MsgToks:   4200,
		ToolToks:  800,
	}
	data2, err := json.Marshal(ev2)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m2 map[string]interface{}
	if err := json.Unmarshal(data2, &m2); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	for key, wantVal := range map[string]float64{"sys_toks": 1500, "msg_toks": 4200, "tool_toks": 800} {
		v, ok := m2[key]
		if !ok {
			t.Errorf("JSON missing key %q, want present", key)
			continue
		}
		if v.(float64) != wantVal {
			t.Errorf("JSON[%q] = %v, want %v", key, v, wantVal)
		}
	}
}

// ---------------------------------------------------------------------------
// Seam 4 — ADR-5: EventMemoryChanged registration (RED — combined in notify)
// ---------------------------------------------------------------------------

// TestEventMemoryChanged_Registered: constant must be in KnownEventTypes but NOT
// in StreamingSkipSet.
func TestEventMemoryChanged_Registered(t *testing.T) {
	if !KnownEventTypes[EventMemoryChanged] {
		t.Errorf("KnownEventTypes missing EventMemoryChanged (%q)", EventMemoryChanged)
	}
	if StreamingSkipSet[EventMemoryChanged] {
		t.Errorf("EventMemoryChanged must NOT be in StreamingSkipSet")
	}
}

// ---------------------------------------------------------------------------
// Seam 6 — ADR-7: new fields are zero on unrelated event types
// ---------------------------------------------------------------------------

// TestEvent_NewFieldsZeroOnUnrelatedTypes: SysToks/MsgToks/ToolToks are Go zero
// value on any event that is not EventTokensUsage.
func TestEvent_NewFieldsZeroOnUnrelatedTypes(t *testing.T) {
	evs := []Event{
		{Type: EventTurnStarted, ChannelID: "ch-1"},
		{Type: EventToolEnd, ChannelID: "ch-1"},
		{Type: EventTurnCompleted, ChannelID: "ch-1"},
	}
	for _, ev := range evs {
		if ev.SysToks != 0 {
			t.Errorf("event %q: SysToks = %d, want 0", ev.Type, ev.SysToks)
		}
		if ev.MsgToks != 0 {
			t.Errorf("event %q: MsgToks = %d, want 0", ev.Type, ev.MsgToks)
		}
		if ev.ToolToks != 0 {
			t.Errorf("event %q: ToolToks = %d, want 0", ev.Type, ev.ToolToks)
		}
	}
}
