package tool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Phase 1 — Data model: TodoList zero value, JSON round-trip, encode/decode
// ---------------------------------------------------------------------------

// TestTodoList_ZeroValue verifies REQ-1: an empty value — which is also how an
// absent metadata key surfaces — decodes to Version=1, Items empty, no error.
func TestTodoList_ZeroValue(t *testing.T) {
	list, err := decodeTodoList("")
	if err != nil {
		t.Fatalf("decodeTodoList(\"\") returned unexpected error: %v", err)
	}
	if list.Version != 1 {
		t.Errorf("zero-value Version = %d, want 1", list.Version)
	}
	if len(list.Items) != 0 {
		t.Errorf("zero-value Items len = %d, want 0", len(list.Items))
	}
}

// TestTodoList_DecodeError_MalformedJSON verifies REQ-1: a non-empty value that is
// not valid JSON surfaces an error rather than being silently swallowed.
func TestTodoList_DecodeError_MalformedJSON(t *testing.T) {
	if _, err := decodeTodoList("{not valid json"); err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// TestTodoList_JSONRoundTrip verifies REQ-1: all fields preserved across encode/decode.
func TestTodoList_JSONRoundTrip(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	original := TodoList{
		Version: 1,
		Items: []TodoItem{
			{
				ID:        "td_aabbccdd",
				Content:   "Write tests",
				Status:    "pending",
				Position:  1,
				CreatedAt: now,
				UpdatedAt: now,
			},
			{
				ID:        "td_11223344",
				Content:   "Review PR",
				Status:    "in_progress",
				Position:  2,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	encoded, err := encodeTodoList(original)
	if err != nil {
		t.Fatalf("encodeTodoList error: %v", err)
	}
	if encoded == "" {
		t.Fatal("encoded result is empty")
	}

	decoded, err := decodeTodoList(encoded)
	if err != nil {
		t.Fatalf("decodeTodoList error: %v", err)
	}

	if decoded.Version != original.Version {
		t.Errorf("Version mismatch: got %d, want %d", decoded.Version, original.Version)
	}
	if len(decoded.Items) != len(original.Items) {
		t.Fatalf("Items count mismatch: got %d, want %d", len(decoded.Items), len(original.Items))
	}

	for i, got := range decoded.Items {
		want := original.Items[i]
		if got.ID != want.ID {
			t.Errorf("[%d] ID: got %q, want %q", i, got.ID, want.ID)
		}
		if got.Content != want.Content {
			t.Errorf("[%d] Content: got %q, want %q", i, got.Content, want.Content)
		}
		if got.Status != want.Status {
			t.Errorf("[%d] Status: got %q, want %q", i, got.Status, want.Status)
		}
		if got.Position != want.Position {
			t.Errorf("[%d] Position: got %d, want %d", i, got.Position, want.Position)
		}
		if !got.CreatedAt.Equal(want.CreatedAt) {
			t.Errorf("[%d] CreatedAt: got %v, want %v", i, got.CreatedAt, want.CreatedAt)
		}
		if !got.UpdatedAt.Equal(want.UpdatedAt) {
			t.Errorf("[%d] UpdatedAt: got %v, want %v", i, got.UpdatedAt, want.UpdatedAt)
		}
	}
}

// TestTodoList_DoubleEncode verifies that encoding an already-encoded result is idempotent
// through a decode→encode cycle (not double-JSON-string nesting issue).
func TestTodoList_DoubleEncode(t *testing.T) {
	list := TodoList{Version: 1, Items: []TodoItem{
		{ID: "td_00000001", Content: "task", Status: "pending", Position: 1},
	}}

	enc1, err := encodeTodoList(list)
	if err != nil {
		t.Fatalf("first encode: %v", err)
	}

	dec, err := decodeTodoList(enc1)
	if err != nil {
		t.Fatalf("decode after first encode: %v", err)
	}

	enc2, err := encodeTodoList(dec)
	if err != nil {
		t.Fatalf("second encode: %v", err)
	}

	// Both encodings should round-trip to the same structure.
	var r1, r2 TodoList
	if err := json.Unmarshal([]byte(enc1), &r1); err != nil {
		t.Fatalf("unmarshal enc1: %v", err)
	}
	if err := json.Unmarshal([]byte(enc2), &r2); err != nil {
		t.Fatalf("unmarshal enc2: %v", err)
	}
	if r1.Version != r2.Version || len(r1.Items) != len(r2.Items) {
		t.Errorf("double-encode produced different structures: %+v vs %+v", r1, r2)
	}
}

// TestTodoList_VersionNonRegression verifies REQ-1: Version does not regress across encode/decode.
func TestTodoList_VersionNonRegression(t *testing.T) {
	list := TodoList{Version: 3, Items: nil}

	enc, err := encodeTodoList(list)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	dec, err := decodeTodoList(enc)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if dec.Version != 3 {
		t.Errorf("Version regressed: got %d, want 3", dec.Version)
	}
}

// TestTodoList_EncodedIsJSONString verifies the encoded output is a plain JSON object
// (not double-encoded — the outer storage layer does the string wrapping if needed).
func TestTodoList_EncodedIsJSONObject(t *testing.T) {
	list := TodoList{Version: 1}
	enc, err := encodeTodoList(list)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	trimmed := strings.TrimSpace(enc)
	if !strings.HasPrefix(trimmed, "{") {
		t.Errorf("encoded output should be a JSON object, got prefix: %q", trimmed[:min(20, len(trimmed))])
	}
}

// TestMaxActiveTodos verifies the constant is defined and equals 200.
func TestMaxActiveTodos(t *testing.T) {
	if maxActiveTodos != 200 {
		t.Errorf("maxActiveTodos = %d, want 200", maxActiveTodos)
	}
}
