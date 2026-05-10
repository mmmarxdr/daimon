package store

import (
	"encoding/json"
	"testing"
)

// TestConversation_ParentConvIDAndStatusJSONRoundTrip verifies that the new
// ParentConvID and Status fields marshal / unmarshal correctly and that
// zero values produce omitempty-clean JSON (no spurious empty-string keys).
func TestConversation_ParentConvIDAndStatusJSONRoundTrip(t *testing.T) {
	t.Run("zero values produce omitempty-clean JSON", func(t *testing.T) {
		conv := Conversation{ID: "c1", ChannelID: "cli"}
		b, err := json.Marshal(conv)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("unmarshal to map: %v", err)
		}
		if _, ok := m["parent_conv_id"]; ok {
			t.Error("parent_conv_id should be absent (omitempty) when zero")
		}
		if _, ok := m["status"]; ok {
			t.Error("status should be absent (omitempty) when zero")
		}
	})

	t.Run("non-zero values round-trip correctly", func(t *testing.T) {
		conv := Conversation{
			ID:           "c2",
			ChannelID:    "cli",
			ParentConvID: "parent-123",
			Status:       "running",
		}
		b, err := json.Marshal(conv)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var conv2 Conversation
		if err := json.Unmarshal(b, &conv2); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if conv2.ParentConvID != "parent-123" {
			t.Errorf("ParentConvID: got %q, want %q", conv2.ParentConvID, "parent-123")
		}
		if conv2.Status != "running" {
			t.Errorf("Status: got %q, want %q", conv2.Status, "running")
		}
	})
}

// TestCostRecord_NewFieldsJSONRoundTrip verifies that the new ConvID, ParentConvID,
// and AttributionKind fields on CostRecord marshal / unmarshal correctly.
func TestCostRecord_NewFieldsJSONRoundTrip(t *testing.T) {
	t.Run("zero values omitted when omitempty", func(t *testing.T) {
		rec := CostRecord{ID: "r1", SessionID: "s1"}
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// CostRecord fields do not require omitempty, but the fields must at least
		// survive a round-trip.
		var rec2 CostRecord
		if err := json.Unmarshal(b, &rec2); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if rec2.ConvID != "" {
			t.Errorf("ConvID: got %q, want empty", rec2.ConvID)
		}
	})

	t.Run("populated fields round-trip correctly", func(t *testing.T) {
		rec := CostRecord{
			ID:              "r2",
			SessionID:       "sess-abc",
			ConvID:          "conv-abc",
			ParentConvID:    "parent-abc",
			AttributionKind: "self",
		}
		b, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		var rec2 CostRecord
		if err := json.Unmarshal(b, &rec2); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if rec2.ConvID != "conv-abc" {
			t.Errorf("ConvID: got %q, want %q", rec2.ConvID, "conv-abc")
		}
		if rec2.ParentConvID != "parent-abc" {
			t.Errorf("ParentConvID: got %q, want %q", rec2.ParentConvID, "parent-abc")
		}
		if rec2.AttributionKind != "self" {
			t.Errorf("AttributionKind: got %q, want %q", rec2.AttributionKind, "self")
		}
	})
}
