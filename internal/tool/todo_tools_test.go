package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// newSeqIDGen returns a deterministic IDGen that produces td_00000001,
// td_00000002, … on successive calls. This makes golden assertions stable.
func newSeqIDGen() IDGen {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("td_%08d", n)
	}
}

// fakeStore is a simple in-memory todo list for use in fake Mutate/Read.
type fakeStore struct {
	list TodoList
}

// newFakeStore returns a fakeStore with an empty todo list.
func newFakeStore() *fakeStore {
	return &fakeStore{list: TodoList{Version: 1}}
}

// deps returns a TodoToolDeps wired to this fakeStore.
func (fs *fakeStore) deps(idg IDGen) TodoToolDeps {
	return TodoToolDeps{
		IDGen: idg,
		Mutate: func(convID string, mutate func(*TodoList) (string, error)) (TodoList, error) {
			if _, err := mutate(&fs.list); err != nil {
				return TodoList{}, err
			}
			return fs.list, nil
		},
		Read: func(convID string) (TodoList, error) {
			return fs.list, nil
		},
	}
}

// convCtx returns a context carrying the given conversation ID.
func convCtx(id string) context.Context {
	return WithConvID(context.Background(), id)
}

// rawParams marshals v to json.RawMessage for use in Execute calls.
func rawParams(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("rawParams: %v", err))
	}
	return b
}

// ---------------------------------------------------------------------------
// Task 2.1 RED tests — BuildTodoTools, todoCreateTool, todoUpdateTool, todoListTool
// ---------------------------------------------------------------------------

func TestBuildTodoTools_ReturnsThreeTools(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	want := []string{"todo_create", "todo_update", "todo_list"}
	for _, name := range want {
		if _, ok := tools[name]; !ok {
			t.Errorf("BuildTodoTools: missing tool %q", name)
		}
	}
	if got := len(tools); got != 3 {
		t.Errorf("BuildTodoTools: got %d tools, want 3", got)
	}
}

// ---------------------------------------------------------------------------
// todo_create tests
// ---------------------------------------------------------------------------

func TestTodoCreate_AppendDefault(t *testing.T) {
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_existing1", Content: "A", Status: "pending", Position: 1},
		{ID: "td_existing2", Content: "B", Status: "pending", Position: 2},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": "Write tests"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected IsError=false, got true: %s", res.Content)
	}

	// Should be appended at position 3.
	if len(fs.list.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(fs.list.Items))
	}
	created := fs.list.Items[2]
	if created.Position != 3 {
		t.Errorf("Position = %d, want 3", created.Position)
	}
	if created.Status != "pending" {
		t.Errorf("Status = %q, want \"pending\"", created.Status)
	}
	if created.Content != "Write tests" {
		t.Errorf("Content = %q, want \"Write tests\"", created.Content)
	}
	if !strings.Contains(res.Content, created.ID) {
		t.Errorf("ToolResult.Content %q does not contain ID %q", res.Content, created.ID)
	}
	if !strings.Contains(res.Content, "3") {
		t.Errorf("ToolResult.Content %q does not mention position 3", res.Content)
	}
}

func TestTodoCreate_InsertAtPosition(t *testing.T) {
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_a", Content: "A", Status: "pending", Position: 1},
		{ID: "td_b", Content: "B", Status: "pending", Position: 2},
		{ID: "td_c", Content: "C", Status: "pending", Position: 3},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": "Urgent task", "position": 2}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Content)
	}

	// New item at position 2; existing positions 2,3 shift to 3,4.
	if len(fs.list.Items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(fs.list.Items))
	}
	// Find the new item by its deterministic ID.
	var newItem *TodoItem
	for i := range fs.list.Items {
		if fs.list.Items[i].Content == "Urgent task" {
			newItem = &fs.list.Items[i]
			break
		}
	}
	if newItem == nil {
		t.Fatal("inserted item not found in list")
	}
	if newItem.Position != 2 {
		t.Errorf("new item Position = %d, want 2", newItem.Position)
	}
	// Every position must be unique.
	positions := map[int]bool{}
	for _, it := range fs.list.Items {
		if positions[it.Position] {
			t.Errorf("duplicate position %d", it.Position)
		}
		positions[it.Position] = true
	}
}

func TestTodoCreate_EmptyContentRejected(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": ""}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for empty content, got false")
	}
	if len(fs.list.Items) != 0 {
		t.Errorf("no item should have been appended, got %d", len(fs.list.Items))
	}
}

func TestTodoCreate_WhitespaceContentRejected(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": "   "}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for whitespace-only content, got false")
	}
	if len(fs.list.Items) != 0 {
		t.Errorf("no item should have been appended, got %d", len(fs.list.Items))
	}
}

func TestTodoCreate_OutOfRangePositionAppends(t *testing.T) {
	// A position outside [1, len+1] is ignored and the item is appended (AD-8).
	for _, pos := range []int{0, 9999} {
		fs := newFakeStore()
		fs.list.Items = []TodoItem{
			{ID: "td_existing1", Content: "A", Status: "pending", Position: 1},
			{ID: "td_existing2", Content: "B", Status: "pending", Position: 2},
		}
		tools := BuildTodoTools(fs.deps(newSeqIDGen()))
		tc := tools["todo_create"]

		res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": "appended", "position": pos}))
		if err != nil {
			t.Fatalf("position=%d: unexpected error: %v", pos, err)
		}
		if res.IsError {
			t.Fatalf("position=%d: expected IsError=false, got true: %s", pos, res.Content)
		}
		if len(fs.list.Items) != 3 {
			t.Fatalf("position=%d: expected 3 items, got %d", pos, len(fs.list.Items))
		}
		// Out-of-range position is ignored; the item lands appended at position 3.
		created := fs.list.Items[2]
		if created.Content != "appended" || created.Position != 3 {
			t.Errorf("position=%d: appended item = {content:%q pos:%d}, want {\"appended\" 3}", pos, created.Content, created.Position)
		}
	}
}

func TestTodoCreate_CapOverflow(t *testing.T) {
	fs := newFakeStore()
	// Fill to cap with active items.
	for i := 1; i <= maxActiveTodos; i++ {
		fs.list.Items = append(fs.list.Items, TodoItem{
			ID:       fmt.Sprintf("td_x%06d", i),
			Content:  "task",
			Status:   "pending",
			Position: i,
		})
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": "overflow"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for cap overflow")
	}
	// Count should not have increased.
	if len(fs.list.Items) != maxActiveTodos {
		t.Errorf("item count changed: got %d, want %d", len(fs.list.Items), maxActiveTodos)
	}
}

func TestTodoCreate_DeterministicID(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": "first"}))
	if err != nil || res.IsError {
		t.Fatalf("unexpected failure: err=%v isError=%v %s", err, res.IsError, res.Content)
	}
	if !strings.Contains(fs.list.Items[0].ID, "td_") {
		t.Errorf("ID %q does not start with td_", fs.list.Items[0].ID)
	}
}

func TestTodoCreate_TimestampsSet(t *testing.T) {
	before := time.Now().Add(-time.Second)
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": "ts test"})) //nolint:errcheck
	if len(fs.list.Items) == 0 {
		t.Fatal("no item created")
	}
	it := fs.list.Items[0]
	if it.CreatedAt.Before(before) {
		t.Errorf("CreatedAt %v is before test start %v", it.CreatedAt, before)
	}
	if it.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt %v is before test start %v", it.UpdatedAt, before)
	}
}

// ---------------------------------------------------------------------------
// todo_update tests
// ---------------------------------------------------------------------------

func TestTodoUpdate_Status(t *testing.T) {
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_abc123", Content: "Do thing", Status: "pending", Position: 1},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_abc123", "status": "in_progress"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Content)
	}
	if fs.list.Items[0].Status != "in_progress" {
		t.Errorf("Status = %q, want \"in_progress\"", fs.list.Items[0].Status)
	}
}

func TestTodoUpdate_ContentOnly(t *testing.T) {
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_abc123", Content: "Old content", Status: "pending", Position: 1},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_abc123", "content": "Revised description"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Content)
	}
	if fs.list.Items[0].Content != "Revised description" {
		t.Errorf("Content = %q, want \"Revised description\"", fs.list.Items[0].Content)
	}
	if fs.list.Items[0].Status != "pending" {
		t.Errorf("Status should be unchanged: got %q", fs.list.Items[0].Status)
	}
}

func TestTodoUpdate_NotFound(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_missing", "status": "completed"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Not-found is NOT IsError; just informational (mirrors update_memory.go:343).
	if res.IsError {
		t.Errorf("expected IsError=false for not-found, got true")
	}
	if !strings.Contains(res.Content, "td_missing") {
		t.Errorf("Content %q should mention the missing id", res.Content)
	}
}

func TestTodoUpdate_NoFields(t *testing.T) {
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_abc123", Content: "X", Status: "pending", Position: 1},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_abc123"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true when no update fields provided")
	}
}

func TestTodoUpdate_CancelledIsTerminal(t *testing.T) {
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_abc123", Content: "X", Status: "cancelled", Position: 1},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_abc123", "status": "pending"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for cancelled→pending transition")
	}
	if fs.list.Items[0].Status != "cancelled" {
		t.Errorf("status should remain \"cancelled\", got %q", fs.list.Items[0].Status)
	}
	// Pin the user-facing message so a future refactor of errCancelledTerminal
	// surfaces here rather than silently changing the tool output.
	if !strings.Contains(res.Content, "terminal") {
		t.Errorf("expected message to mention \"terminal\", got %q", res.Content)
	}
}

func TestTodoUpdate_InvalidStatus(t *testing.T) {
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_abc123", Content: "X", Status: "pending", Position: 1},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_abc123", "status": "done"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for invalid status \"done\"")
	}
}

func TestTodoUpdate_UpdatedAtBumped(t *testing.T) {
	before := time.Now().Add(-time.Second)
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_abc123", Content: "X", Status: "pending", Position: 1, UpdatedAt: before.Add(-time.Hour)},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_abc123", "status": "in_progress"})) //nolint:errcheck
	if fs.list.Items[0].UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt was not bumped: got %v", fs.list.Items[0].UpdatedAt)
	}
}

func TestTodoUpdate_PositionPreserved(t *testing.T) {
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_abc123", Content: "X", Status: "pending", Position: 3},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_abc123", "status": "completed"})) //nolint:errcheck
	if fs.list.Items[0].Position != 3 {
		t.Errorf("Position changed: got %d, want 3", fs.list.Items[0].Position)
	}
}

func TestTodoUpdate_CompletedToInProgress_Allowed(t *testing.T) {
	// REQ-9: completed → in_progress is permitted (only cancelled is terminal).
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_abc123", Content: "X", Status: "completed", Position: 1},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_abc123", "status": "in_progress"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("expected IsError=false for completed→in_progress, got true: %s", res.Content)
	}
	if fs.list.Items[0].Status != "in_progress" {
		t.Errorf("Status = %q, want \"in_progress\"", fs.list.Items[0].Status)
	}
}

func TestTodoUpdate_CancelledToCompleted_Rejected(t *testing.T) {
	// REQ-9: cancelled → completed must also be rejected.
	fs := newFakeStore()
	fs.list.Items = []TodoItem{
		{ID: "td_abc123", Content: "X", Status: "cancelled", Position: 1},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_abc123", "status": "completed"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for cancelled→completed")
	}
}

// ---------------------------------------------------------------------------
// todo_list tests
// ---------------------------------------------------------------------------

func TestTodoList_All(t *testing.T) {
	fs := newFakeStore()
	now := time.Now()
	fs.list.Items = []TodoItem{
		{ID: "td_1", Content: "alpha", Status: "pending", Position: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "td_2", Content: "beta", Status: "completed", Position: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "td_3", Content: "gamma", Status: "in_progress", Position: 3, CreatedAt: now, UpdatedAt: now},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tl := tools["todo_list"]

	res, err := tl.Execute(convCtx("c1"), rawParams(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Content)
	}
	// All three items should appear.
	for _, id := range []string{"td_1", "td_2", "td_3"} {
		if !strings.Contains(res.Content, id) {
			t.Errorf("Content should contain %q, got:\n%s", id, res.Content)
		}
	}
	// Output should be a numbered list (1., 2., 3.).
	for _, prefix := range []string{"1.", "2.", "3."} {
		if !strings.Contains(res.Content, prefix) {
			t.Errorf("Content should contain %q, got:\n%s", prefix, res.Content)
		}
	}
}

func TestTodoList_FilterByStatus(t *testing.T) {
	fs := newFakeStore()
	now := time.Now()
	fs.list.Items = []TodoItem{
		{ID: "td_p1", Content: "pend-A", Status: "pending", Position: 1, CreatedAt: now, UpdatedAt: now},
		{ID: "td_p2", Content: "pend-B", Status: "pending", Position: 2, CreatedAt: now, UpdatedAt: now},
		{ID: "td_c1", Content: "done-C", Status: "completed", Position: 3, CreatedAt: now, UpdatedAt: now},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tl := tools["todo_list"]

	res, err := tl.Execute(convCtx("c1"), rawParams(map[string]any{"status": "pending"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Fatalf("IsError=true: %s", res.Content)
	}
	if !strings.Contains(res.Content, "td_p1") {
		t.Errorf("expected td_p1 in output")
	}
	if !strings.Contains(res.Content, "td_p2") {
		t.Errorf("expected td_p2 in output")
	}
	if strings.Contains(res.Content, "td_c1") {
		t.Errorf("completed item td_c1 should be filtered out")
	}
}

func TestTodoList_Empty(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tl := tools["todo_list"]

	res, err := tl.Execute(convCtx("c1"), rawParams(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsError {
		t.Errorf("IsError=true for empty list: %s", res.Content)
	}
	if !strings.Contains(res.Content, "No todos") {
		t.Errorf("expected \"No todos.\" for empty list, got: %q", res.Content)
	}
}

func TestTodoList_NeverMutates(t *testing.T) {
	// todo_list must not call Mutate.
	mutated := false
	fs := newFakeStore()
	now := time.Now()
	fs.list.Items = []TodoItem{
		{ID: "td_x", Content: "task", Status: "pending", Position: 1, CreatedAt: now, UpdatedAt: now},
	}
	deps := TodoToolDeps{
		IDGen: newSeqIDGen(),
		Mutate: func(convID string, fn func(*TodoList) (string, error)) (TodoList, error) {
			mutated = true
			return TodoList{}, nil
		},
		Read: func(convID string) (TodoList, error) {
			return fs.list, nil
		},
	}
	tools := BuildTodoTools(deps)
	tl := tools["todo_list"]
	tl.Execute(convCtx("c1"), rawParams(map[string]any{})) //nolint:errcheck
	if mutated {
		t.Error("todo_list must not call Mutate")
	}
}

func TestTodoList_InvalidStatusFilter(t *testing.T) {
	fs := newFakeStore()
	now := time.Now()
	fs.list.Items = []TodoItem{
		{ID: "td_x", Content: "task", Status: "pending", Position: 1, CreatedAt: now, UpdatedAt: now},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tl := tools["todo_list"]

	res, err := tl.Execute(convCtx("c1"), rawParams(map[string]any{"status": "done"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for invalid status filter \"done\"")
	}
}

// ---------------------------------------------------------------------------
// Task 2.3 RED tests — schema validation (malformed params, missing required,
// no conv ctx)
// ---------------------------------------------------------------------------

func TestTodoCreate_MalformedParams(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	res, err := tc.Execute(convCtx("c1"), json.RawMessage(`{not valid json`))
	if err != nil {
		t.Fatalf("unexpected error return: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for malformed params")
	}
}

func TestTodoCreate_NoConvCtx(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	res, err := tc.Execute(context.Background(), rawParams(map[string]any{"content": "hello"}))
	if err != nil {
		t.Fatalf("unexpected error return: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true when no conv ctx")
	}
	if !strings.Contains(res.Content, "no conversation context") {
		t.Errorf("Content %q should mention 'no conversation context'", res.Content)
	}
}

func TestTodoUpdate_MalformedParams(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(convCtx("c1"), json.RawMessage(`{not valid`))
	if err != nil {
		t.Fatalf("unexpected error return: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for malformed params")
	}
}

func TestTodoUpdate_NoConvCtx(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tu := tools["todo_update"]

	res, err := tu.Execute(context.Background(), rawParams(map[string]any{"id": "td_x", "status": "completed"}))
	if err != nil {
		t.Fatalf("unexpected error return: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true when no conv ctx")
	}
	if !strings.Contains(res.Content, "no conversation context") {
		t.Errorf("Content %q should mention 'no conversation context'", res.Content)
	}
}

func TestTodoList_MalformedParams(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tl := tools["todo_list"]

	res, err := tl.Execute(convCtx("c1"), json.RawMessage(`{not valid`))
	if err != nil {
		t.Fatalf("unexpected error return: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true for malformed params")
	}
}

func TestTodoList_NoConvCtx(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tl := tools["todo_list"]

	res, err := tl.Execute(context.Background(), rawParams(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error return: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true when no conv ctx")
	}
	if !strings.Contains(res.Content, "no conversation context") {
		t.Errorf("Content %q should mention 'no conversation context'", res.Content)
	}
}

// ---------------------------------------------------------------------------
// REQ-4 output format: "N. [status] content (id: td_xxxx)"
// ---------------------------------------------------------------------------

func TestTodoList_OutputFormat(t *testing.T) {
	fs := newFakeStore()
	now := time.Now()
	fs.list.Items = []TodoItem{
		{ID: "td_00000001", Content: "Do something", Status: "pending", Position: 1, CreatedAt: now, UpdatedAt: now},
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tl := tools["todo_list"]

	res, err := tl.Execute(convCtx("c1"), rawParams(map[string]any{}))
	if err != nil || res.IsError {
		t.Fatalf("unexpected failure: err=%v isError=%v %s", err, res.IsError, res.Content)
	}
	// Expect: "1. [pending] Do something (id: td_00000001)"
	want := "1. [pending] Do something (id: td_00000001)"
	if !strings.Contains(res.Content, want) {
		t.Errorf("output format mismatch.\nwant substring: %q\ngot: %q", want, res.Content)
	}
}

// ---------------------------------------------------------------------------
// REQ-9 — Large content stored faithfully
// ---------------------------------------------------------------------------

func TestTodoCreate_LargeContent(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]
	tl := tools["todo_list"]

	largeContent := strings.Repeat("x", 10_000)
	res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": largeContent}))
	if err != nil || res.IsError {
		t.Fatalf("create failed: err=%v isError=%v %s", err, res.IsError, res.Content)
	}

	listRes, err := tl.Execute(convCtx("c1"), rawParams(map[string]any{}))
	if err != nil || listRes.IsError {
		t.Fatalf("list failed: err=%v isError=%v %s", err, listRes.IsError, listRes.Content)
	}
	if !strings.Contains(listRes.Content, largeContent) {
		t.Errorf("large content not preserved in list output (len want=%d)", len(largeContent))
	}
}

// ---------------------------------------------------------------------------
// AD-5 cap: terminal items do NOT count against the cap
// ---------------------------------------------------------------------------

func TestTodoCreate_CapCountsOnlyActiveItems(t *testing.T) {
	fs := newFakeStore()
	// Fill with terminal items (completed + cancelled) up to 2×cap.
	for i := 1; i <= maxActiveTodos*2; i++ {
		status := "completed"
		if i%2 == 0 {
			status = "cancelled"
		}
		fs.list.Items = append(fs.list.Items, TodoItem{
			ID: fmt.Sprintf("td_t%06d", i), Content: "done", Status: status, Position: i,
		})
	}
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))
	tc := tools["todo_create"]

	// Should succeed because active count = 0.
	res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": "new active"}))
	if err != nil || res.IsError {
		t.Fatalf("should not be capped by terminal items: err=%v isError=%v %s", err, res.IsError, res.Content)
	}
}

// ---------------------------------------------------------------------------
// Nil-guard: tools with nil deps must not panic
// ---------------------------------------------------------------------------

func TestTodoCreate_NilDepsNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("todo_create panicked with nil deps: %v", r)
		}
	}()
	tools := BuildTodoTools(TodoToolDeps{})
	tc := tools["todo_create"]
	res, err := tc.Execute(convCtx("c1"), rawParams(map[string]any{"content": "hello"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true when deps are nil")
	}
}

func TestTodoUpdate_NilDepsNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("todo_update panicked with nil deps: %v", r)
		}
	}()
	tools := BuildTodoTools(TodoToolDeps{})
	tu := tools["todo_update"]
	res, err := tu.Execute(convCtx("c1"), rawParams(map[string]any{"id": "td_x", "status": "completed"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true when deps are nil")
	}
}

func TestTodoList_NilDepsNoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("todo_list panicked with nil deps: %v", r)
		}
	}()
	tools := BuildTodoTools(TodoToolDeps{})
	tl := tools["todo_list"]
	res, err := tl.Execute(convCtx("c1"), rawParams(map[string]any{}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsError {
		t.Errorf("expected IsError=true when deps are nil")
	}
}

// ---------------------------------------------------------------------------
// Name / Description / Schema sanity checks
// ---------------------------------------------------------------------------

func TestTodoTools_Metadata(t *testing.T) {
	fs := newFakeStore()
	tools := BuildTodoTools(fs.deps(newSeqIDGen()))

	cases := []struct {
		name string
		tool Tool
	}{
		{"todo_create", tools["todo_create"]},
		{"todo_update", tools["todo_update"]},
		{"todo_list", tools["todo_list"]},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.tool.Name() != tc.name {
				t.Errorf("Name() = %q, want %q", tc.tool.Name(), tc.name)
			}
			if tc.tool.Description() == "" {
				t.Errorf("Description() is empty")
			}
			schema := tc.tool.Schema()
			if !json.Valid(schema) {
				t.Errorf("Schema() is not valid JSON: %s", schema)
			}
			var obj map[string]any
			if err := json.Unmarshal(schema, &obj); err != nil {
				t.Errorf("Schema() could not unmarshal: %v", err)
			}
			if obj["type"] != "object" {
				t.Errorf("Schema()[\"type\"] = %v, want \"object\"", obj["type"])
			}
		})
	}
}
