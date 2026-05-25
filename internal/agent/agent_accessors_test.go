package agent

import (
	"context"
	"encoding/json"
	"testing"

	"daimon/internal/config"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// TestAgent_ToolRegistry_ReturnsCopy verifies that ToolRegistry() returns an
// independent snapshot: mutating the returned map does not affect subsequent calls.
func TestAgent_ToolRegistry_ReturnsCopy(t *testing.T) {
	a := minimalTestAgent(t)
	// Inject a test tool directly into the agent's tools map.
	a.toolsMu.Lock()
	a.tools["test-tool"] = &fakeToolForAccessor{name: "test-tool"}
	a.toolsMu.Unlock()

	// First call — snapshot.
	snap1 := a.ToolRegistry()
	if _, ok := snap1["test-tool"]; !ok {
		t.Fatal("ToolRegistry() missing injected tool")
	}

	// Mutate the returned map.
	snap1["injected-extra"] = &fakeToolForAccessor{name: "injected-extra"}

	// Second call — must not see the mutation from snap1.
	snap2 := a.ToolRegistry()
	if _, ok := snap2["injected-extra"]; ok {
		t.Error("ToolRegistry() returned a live map reference — mutation leaked into subsequent call")
	}
}

// TestAgent_TodoListForConv_ReturnsEmptyOnMissingConv verifies that
// TodoListForConv delegates to todoRead and returns a valid (empty) TodoList
// when the conversation ID is unknown — no panic, no error.
func TestAgent_TodoListForConv_ReturnsEmptyOnMissingConv(t *testing.T) {
	a := minimalTestAgent(t)

	got, err := a.TodoListForConv("nonexistent-conv")
	if err != nil {
		t.Fatalf("TodoListForConv() unexpected error: %v", err)
	}
	// todoRead with no active conv and nil store returns TodoList{Version:1}.
	if got.Version != 1 {
		t.Errorf("TodoListForConv() Version = %d, want 1", got.Version)
	}
}

// TestAgent_ToolRegistry_EmptyAgent verifies ToolRegistry() on a zero-tools
// agent returns an empty non-nil map (not nil, not panic).
func TestAgent_ToolRegistry_EmptyAgent(t *testing.T) {
	a := minimalTestAgent(t)
	reg := a.ToolRegistry()
	if reg == nil {
		t.Error("ToolRegistry() returned nil, want empty map")
	}
	if len(reg) != 0 {
		t.Errorf("ToolRegistry() len=%d, want 0", len(reg))
	}
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// fakeToolForAccessor is a minimal tool.Tool stub implementing all interface methods.
type fakeToolForAccessor struct {
	name string
}

func (f *fakeToolForAccessor) Name() string        { return f.name }
func (f *fakeToolForAccessor) Description() string { return "fake" }
func (f *fakeToolForAccessor) Schema() json.RawMessage {
	return json.RawMessage(`{}`)
}
func (f *fakeToolForAccessor) Execute(_ context.Context, _ json.RawMessage) (tool.ToolResult, error) {
	return tool.ToolResult{Content: "fake"}, nil
}

// minimalTestAgent returns an Agent with only the fields needed for accessor tests.
// Uses a FileStore in a temp directory so todoRead's store fallback doesn't panic.
func minimalTestAgent(t *testing.T) *Agent {
	t.Helper()
	dir := t.TempDir()
	st := store.NewFileStore(config.StoreConfig{Path: dir})
	return &Agent{
		tools: make(map[string]tool.Tool),
		store: st,
	}
}
