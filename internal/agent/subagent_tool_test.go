package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// Helpers — fake manager for tool tests
// ---------------------------------------------------------------------------

// fakeManager is a test double for SpawnCaller interface used by SubagentSpawnTool.
type fakeManager struct {
	mu          sync.Mutex
	spawnCalled bool
	spawnDef    skill.ExecutableSkillDef
	spawnPrompt string
	spawnMode   SpawnMode
	// If set, Spawn returns an error.
	spawnErr error
	// If set, Wait returns an error.
	waitErr error
	// handle to return.
	handle *SubagentHandle
}

func (f *fakeManager) Spawn(
	_ context.Context,
	def skill.ExecutableSkillDef,
	prompt string,
	mode SpawnMode,
	_ string,
) (*SubagentHandle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.spawnCalled = true
	f.spawnDef = def
	f.spawnPrompt = prompt
	f.spawnMode = mode

	if f.spawnErr != nil {
		return nil, f.spawnErr
	}
	if f.handle != nil {
		return f.handle, nil
	}

	// Build a pre-completed handle.
	rec := &subRecord{
		id:      "test-id",
		batchID: "test-batch",
		status:  "completed",
		done:    make(chan struct{}),
		mu:      sync.Mutex{},
		result: &SubagentResult{
			Status:  "completed",
			Summary: "done",
			Cost:    0.001,
			Turns:   1,
			Metadata: map[string]string{
				"subagent_id": "test-id",
				"batch_id":    "test-batch",
			},
		},
	}
	// Close done immediately so Wait returns right away.
	close(rec.done)
	h := &SubagentHandle{ID: "test-id", BatchID: "test-batch", rec: rec}
	if f.waitErr != nil {
		// Override wait to error: re-open done and don't close (Wait will use ctx).
		rec.done = make(chan struct{}) // won't be closed
	}
	return h, nil
}

// newToolTestDef returns a minimal ExecutableSkillDef for testing.
func newToolTestDef() skill.ExecutableSkillDef {
	return skill.ExecutableSkillDef{
		Name:        "researcher",
		Description: "Research a topic and summarize findings",
		Budget: skill.BudgetConfig{
			MaxCostUSD: 0.5,
			MaxTurns:   20,
			Timeout:    10 * time.Minute,
		},
	}
}

// ---------------------------------------------------------------------------
// 2.12 — SubagentSpawnTool tests
// ---------------------------------------------------------------------------

func TestSubagentSpawnTool_Name(t *testing.T) {
	def := newToolTestDef()
	mgr := &fakeManager{}
	tool := &SubagentSpawnTool{def: def, manager: mgr}

	if got := tool.Name(); got != "researcher" {
		t.Errorf("Name() = %q, want %q", got, "researcher")
	}
}

func TestSubagentSpawnTool_Description(t *testing.T) {
	def := newToolTestDef()
	mgr := &fakeManager{}
	tool := &SubagentSpawnTool{def: def, manager: mgr}

	if got := tool.Description(); got != def.Description {
		t.Errorf("Description() = %q, want %q", got, def.Description)
	}
}

func TestSubagentSpawnTool_Schema_Valid(t *testing.T) {
	def := newToolTestDef()
	mgr := &fakeManager{}
	st := &SubagentSpawnTool{def: def, manager: mgr}

	schema := st.Schema()
	if len(schema) == 0 {
		t.Fatal("Schema() returned empty JSON")
	}

	var parsed map[string]any
	if err := json.Unmarshal(schema, &parsed); err != nil {
		t.Fatalf("Schema() is not valid JSON: %v", err)
	}

	// Must have "properties" with "prompt" (required) and "mode" enum.
	props, ok := parsed["properties"].(map[string]any)
	if !ok {
		t.Fatal("Schema.properties is missing or not an object")
	}
	if _, ok := props["prompt"]; !ok {
		t.Error("Schema.properties.prompt is missing")
	}
	if modeProps, ok := props["mode"].(map[string]any); !ok {
		t.Error("Schema.properties.mode is missing")
	} else {
		enum, _ := modeProps["enum"].([]any)
		if len(enum) != 2 {
			t.Errorf("Schema.properties.mode.enum has %d items, want 2", len(enum))
		}
	}

	// "prompt" must be in required.
	required, _ := parsed["required"].([]any)
	hasPrompt := false
	for _, r := range required {
		if r == "prompt" {
			hasPrompt = true
		}
	}
	if !hasPrompt {
		t.Error("Schema.required does not contain 'prompt'")
	}
}

func TestSubagentSpawnTool_Execute_EmptyPrompt(t *testing.T) {
	def := newToolTestDef()
	mgr := &fakeManager{}
	st := &SubagentSpawnTool{def: def, manager: mgr}

	params := json.RawMessage(`{"prompt":""}`)
	result, err := st.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Execute with empty prompt: expected IsError=true")
	}
}

func TestSubagentSpawnTool_Execute_SyncMode_BlocksAndReturnsResult(t *testing.T) {
	def := newToolTestDef()
	mgr := &fakeManager{}
	st := &SubagentSpawnTool{def: def, manager: mgr}

	params := json.RawMessage(`{"prompt":"research golang","mode":"sync"}`)
	result, err := st.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("Execute sync: IsError=true, content=%q", result.Content)
	}

	// Content should be SubagentResult JSON.
	var res SubagentResult
	if err := json.Unmarshal([]byte(result.Content), &res); err != nil {
		t.Fatalf("Execute sync: content is not SubagentResult JSON: %v — content=%q", err, result.Content)
	}
	if res.Status != "completed" {
		t.Errorf("SubagentResult.Status = %q, want %q", res.Status, "completed")
	}

	// Meta must carry subagent_id and batch_id.
	if result.Meta["subagent_id"] == "" {
		t.Error("Meta.subagent_id must not be empty")
	}
	if result.Meta["batch_id"] == "" {
		t.Error("Meta.batch_id must not be empty")
	}
	if result.Meta["mode"] != "sync" {
		t.Errorf("Meta.mode = %q, want %q", result.Meta["mode"], "sync")
	}
}

func TestSubagentSpawnTool_Execute_AsyncMode_ReturnsHandleImmediately(t *testing.T) {
	def := newToolTestDef()
	// Build a handle whose done channel never closes (simulates running sub).
	rec := &subRecord{
		id:      "async-id",
		batchID: "async-batch",
		status:  "running",
		done:    make(chan struct{}), // never closed
		mu:      sync.Mutex{},
	}
	mgr := &fakeManager{
		handle: &SubagentHandle{ID: "async-id", BatchID: "async-batch", rec: rec},
	}
	st := &SubagentSpawnTool{def: def, manager: mgr}

	params := json.RawMessage(`{"prompt":"research golang","mode":"async"}`)

	// async should return before the subagent completes.
	done := make(chan struct{})
	var result interface{}
	go func() {
		r, _ := st.Execute(context.Background(), params)
		result = r
		close(done)
	}()

	select {
	case <-done:
		// Good — returned immediately.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Execute async: did not return within 500ms (should return immediately)")
	}
	_ = result
}

func TestSubagentSpawnTool_Execute_SpawnError_ReturnsToolError(t *testing.T) {
	def := newToolTestDef()
	mgr := &fakeManager{spawnErr: ErrSubagentDepthExceeded}
	st := &SubagentSpawnTool{def: def, manager: mgr}

	params := json.RawMessage(`{"prompt":"research golang"}`)
	result, err := st.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("Execute with spawn error: expected IsError=true")
	}
}

func TestSubagentSpawnTool_Execute_DefaultMode_IsSync(t *testing.T) {
	def := newToolTestDef()
	mgr := &fakeManager{}
	st := &SubagentSpawnTool{def: def, manager: mgr}

	// No "mode" field — should default to sync.
	params := json.RawMessage(`{"prompt":"research golang"}`)
	_, err := st.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	mgr.mu.Lock()
	mode := mgr.spawnMode
	mgr.mu.Unlock()

	if mode != SpawnModeSync {
		t.Errorf("default mode = %q, want %q", mode, SpawnModeSync)
	}
}
