package store

import (
	"encoding/json"
	"testing"
)

// TestBudgetJSON_NilRoundTrip verifies nil Budget encodes to SQL NULL and back.
func TestBudgetJSON_NilRoundTrip(t *testing.T) {
	var b *BudgetJSON
	ns := encodeBudget(b)
	if ns.Valid {
		t.Errorf("expected sql.NullString.Valid=false for nil budget, got Valid=true value=%q", ns.String)
	}

	decoded := decodeBudget(ns)
	if decoded != nil {
		t.Errorf("expected nil budget after round-trip, got %+v", decoded)
	}
}

// TestBudgetJSON_NonNilRoundTrip verifies non-nil Budget round-trips via JSON.
func TestBudgetJSON_NonNilRoundTrip(t *testing.T) {
	b := &BudgetJSON{MaxCostUSD: 0.50, MaxTurns: 20, TimeoutMin: 10}
	ns := encodeBudget(b)
	if !ns.Valid {
		t.Fatal("expected Valid=true for non-nil budget")
	}

	decoded := decodeBudget(ns)
	if decoded == nil {
		t.Fatal("expected non-nil budget after round-trip")
	}
	if decoded.MaxCostUSD != 0.50 {
		t.Errorf("MaxCostUSD: got %v, want 0.50", decoded.MaxCostUSD)
	}
	if decoded.MaxTurns != 20 {
		t.Errorf("MaxTurns: got %v, want 20", decoded.MaxTurns)
	}
	if decoded.TimeoutMin != 10 {
		t.Errorf("TimeoutMin: got %v, want 10", decoded.TimeoutMin)
	}
}

// TestBudgetJSON_JSONSerialization verifies BudgetJSON marshals / unmarshals correctly.
func TestBudgetJSON_JSONSerialization(t *testing.T) {
	b := &BudgetJSON{MaxCostUSD: 1.23, MaxTurns: 5, TimeoutMin: 3}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var b2 BudgetJSON
	if err := json.Unmarshal(data, &b2); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if b2.MaxCostUSD != b.MaxCostUSD || b2.MaxTurns != b.MaxTurns || b2.TimeoutMin != b.TimeoutMin {
		t.Errorf("round-trip mismatch: got %+v, want %+v", b2, *b)
	}
}

// TestToolsAllowlist_NilRoundTrip verifies nil allowlist → sql.NullString.Valid=false → nil.
func TestToolsAllowlist_NilRoundTrip(t *testing.T) {
	ns := encodeAllowlist(nil)
	if ns.Valid {
		t.Errorf("expected Valid=false for nil allowlist, got Valid=true value=%q", ns.String)
	}

	decoded := decodeAllowlist(ns)
	if decoded != nil {
		t.Errorf("expected nil after round-trip, got %v", decoded)
	}
}

// TestToolsAllowlist_EmptySliceRoundTrip verifies []string{} → JSON "[]" → non-nil empty slice.
func TestToolsAllowlist_EmptySliceRoundTrip(t *testing.T) {
	ns := encodeAllowlist([]string{})
	if !ns.Valid {
		t.Fatal("expected Valid=true for empty allowlist")
	}
	if ns.String != "[]" {
		t.Errorf("expected encoded value=[],  got %q", ns.String)
	}

	decoded := decodeAllowlist(ns)
	if decoded == nil {
		t.Fatal("expected non-nil empty slice after round-trip, got nil")
	}
	if len(decoded) != 0 {
		t.Errorf("expected empty slice length=0, got %d", len(decoded))
	}
}

// TestToolsAllowlist_ValuesRoundTrip verifies a non-empty allowlist round-trips correctly.
func TestToolsAllowlist_ValuesRoundTrip(t *testing.T) {
	input := []string{"bash", "read_file", "write_file"}
	ns := encodeAllowlist(input)
	if !ns.Valid {
		t.Fatal("expected Valid=true for non-empty allowlist")
	}

	decoded := decodeAllowlist(ns)
	if len(decoded) != len(input) {
		t.Fatalf("length mismatch: got %d, want %d", len(decoded), len(input))
	}
	for i, v := range input {
		if decoded[i] != v {
			t.Errorf("index %d: got %q, want %q", i, decoded[i], v)
		}
	}
}
