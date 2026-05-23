package notify

import (
	"encoding/json"
	"testing"
	"time"
)

// TestEvent_JSONRoundTrip_NewFields_AllPresent (REQ-1.2) — full-field round-trip.
func TestEvent_JSONRoundTrip_NewFields_AllPresent(t *testing.T) {
	original := Event{
		Type:       EventTurnCompleted,
		Origin:     OriginAgent,
		ChannelID:  "ch-1",
		Timestamp:  time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		ToolCallID: "tc-1",
		ToolName:   "shell_exec",
		Iteration:  2,
		TokenCount: 150,
		DurationMs: 423,
		CostUSD:    0.0012,
		IsError:    true,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var got Event
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if got.ToolCallID != original.ToolCallID {
		t.Errorf("ToolCallID: got %q want %q", got.ToolCallID, original.ToolCallID)
	}
	if got.ToolName != original.ToolName {
		t.Errorf("ToolName: got %q want %q", got.ToolName, original.ToolName)
	}
	if got.Iteration != original.Iteration {
		t.Errorf("Iteration: got %d want %d", got.Iteration, original.Iteration)
	}
	if got.TokenCount != original.TokenCount {
		t.Errorf("TokenCount: got %d want %d", got.TokenCount, original.TokenCount)
	}
	if got.DurationMs != original.DurationMs {
		t.Errorf("DurationMs: got %d want %d", got.DurationMs, original.DurationMs)
	}
	if got.CostUSD != original.CostUSD {
		t.Errorf("CostUSD: got %f want %f", got.CostUSD, original.CostUSD)
	}
	if got.IsError != original.IsError {
		t.Errorf("IsError: got %v want %v", got.IsError, original.IsError)
	}
}

// TestEvent_JSONRoundTrip_NewFields_OmittedAtZero (REQ-1.1) — zero-value new fields
// must not appear in JSON output.
func TestEvent_JSONRoundTrip_NewFields_OmittedAtZero(t *testing.T) {
	ev := Event{
		Type:      EventTurnStarted,
		Origin:    OriginAgent,
		ChannelID: "ch-1",
		Timestamp: time.Now(),
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal to map: %v", err)
	}

	zeroFields := []string{"tool_call_id", "tool_name", "iteration", "token_count", "duration_ms", "cost_usd", "is_error"}
	for _, field := range zeroFields {
		if _, present := raw[field]; present {
			t.Errorf("field %q should be omitted at zero value but was present in JSON", field)
		}
	}
}

// TestEvent_IsError_OmittedAtFalse (REQ-1.3) — IsError=false must be omitted.
func TestEvent_IsError_OmittedAtFalse(t *testing.T) {
	ev := Event{
		Type:      EventTurnCompleted,
		Origin:    OriginAgent,
		ChannelID: "ch-1",
		Timestamp: time.Now(),
		IsError:   false,
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, present := raw["is_error"]; present {
		t.Error(`"is_error" key should be absent when IsError=false, but was present`)
	}
}

// TestEvent_LegacyEvent_ByteIdenticalToBeforeChange (REQ-1.4) — a legacy event
// (pre-existing fields only) must produce byte-identical JSON before and after the
// struct extension. We verify by checking no new keys appear.
func TestEvent_LegacyEvent_ByteIdenticalToBeforeChange(t *testing.T) {
	ev := Event{
		Type:      EventCronJobFired,
		Origin:    OriginCron,
		JobID:     "job-42",
		ChannelID: "cron:job-42",
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	// Pre-change expected JSON keys (all legacy fields, omitempty-skipping zero values).
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	allowedKeys := map[string]bool{
		"type": true, "origin": true, "job_id": true,
		"channel_id": true, "timestamp": true,
	}
	for key := range raw {
		if !allowedKeys[key] {
			t.Errorf("unexpected key %q in legacy event JSON — new fields should be omitted at zero value", key)
		}
	}
}
