package notify

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSubagentEventConstants_Distinct verifies the 3 new event constants are
// distinct strings (no copy-paste collision).
func TestSubagentEventConstants_Distinct(t *testing.T) {
	constants := []string{
		EventSubagentSpawned,
		EventSubagentCompleted,
		EventSubagentFailed,
	}
	seen := make(map[string]bool)
	for _, c := range constants {
		if seen[c] {
			t.Errorf("duplicate event constant value: %q", c)
		}
		seen[c] = true
		if c == "" {
			t.Errorf("event constant must not be empty")
		}
	}
}

// TestSubagentEventConstants_InKnownEventTypes verifies all 3 new constants
// are registered in KnownEventTypes.
func TestSubagentEventConstants_InKnownEventTypes(t *testing.T) {
	required := []string{
		EventSubagentSpawned,
		EventSubagentCompleted,
		EventSubagentFailed,
	}
	for _, ev := range required {
		if !KnownEventTypes[ev] {
			t.Errorf("KnownEventTypes missing %q", ev)
		}
	}
}

// TestSubagentEventConstants_ValuesMatchSpec verifies the exact string values
// specified in design §2.6.
func TestSubagentEventConstants_ValuesMatchSpec(t *testing.T) {
	if EventSubagentSpawned != "agent.subagent.spawned" {
		t.Errorf("EventSubagentSpawned = %q, want %q", EventSubagentSpawned, "agent.subagent.spawned")
	}
	if EventSubagentCompleted != "agent.subagent.completed" {
		t.Errorf("EventSubagentCompleted = %q, want %q", EventSubagentCompleted, "agent.subagent.completed")
	}
	if EventSubagentFailed != "agent.subagent.failed" {
		t.Errorf("EventSubagentFailed = %q, want %q", EventSubagentFailed, "agent.subagent.failed")
	}
}

// TestSubagentEvent_MetaFieldsSerialization verifies the Event struct can carry
// all required subagent metadata fields and round-trip through JSON.
func TestSubagentEvent_MetaFieldsSerialization(t *testing.T) {
	meta := map[string]string{
		"subagent_id":    "abc-123",
		"batch_id":       "abc-123",
		"skill":          "researcher",
		"parent_conv_id": "conv_parent",
		"reason":         "budget_exceeded",
		"cost_usd":       "0.4200",
		"turns":          "8",
		"model":          "claude-haiku-4-5",
		"max_cost_usd":   "0.5000",
		"max_turns":      "20",
		"timeout_sec":    "600",
	}

	ev := Event{
		Type:      EventSubagentFailed,
		Origin:    OriginAgent,
		ChannelID: "sub:abc-123",
		Text:      "",
		Error:     "budget_exceeded",
		Timestamp: time.Now(),
		Meta:      meta,
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("json.Marshal Event: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal Event: %v", err)
	}

	// Verify required fields survived the round-trip.
	requiredKeys := []string{
		"subagent_id", "batch_id", "skill", "parent_conv_id",
		"reason", "cost_usd", "turns",
	}
	for _, key := range requiredKeys {
		if decoded.Meta[key] != meta[key] {
			t.Errorf("Meta[%q] = %q, want %q", key, decoded.Meta[key], meta[key])
		}
	}

	if decoded.Type != EventSubagentFailed {
		t.Errorf("Type = %q, want %q", decoded.Type, EventSubagentFailed)
	}
	if decoded.Error != "budget_exceeded" {
		t.Errorf("Error = %q, want %q", decoded.Error, "budget_exceeded")
	}
}
