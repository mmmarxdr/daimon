package agent

// modes_todo_test.go — RED tests for Phase 4 of todolist-tool PR4.
//
// Tests cover REQ-8 mode-gating and the plan-prompt verbatim wording:
//   - planAllowlist allowlist-level: contains todo_create, todo_update, todo_list
//   - reviewAllowlist allowlist-level: contains todo_list only; NOT todo_create/todo_update
//   - planPrompt: contains exact verbatim wording from REQ-8 spec (contract-locked)
//   - mode-gate integration: plan permits todo_create/update/list; review blocks
//     todo_create/update but permits todo_list; build permits all three.
//
// REQs covered: REQ-8 (scenarios: plan permits create, review blocks create,
// review permits list, build permits all, plan-prompt wording exact).
//
// TDD discipline: written RED first. GREEN achieved by modifying modes.go
// per AD-3 (baseReadOnly extraction + planPrompt verbatim wording).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// Allowlist-level assertions (REQ-8, AD-3)
// ---------------------------------------------------------------------------

// planAllowlistContains returns true if name is present in planAllowlist.
// Accessing the package-private variable directly (same package, test file).
func planAllowlistContains(name string) bool {
	for _, n := range planAllowlist {
		if n == name {
			return true
		}
	}
	return false
}

// reviewAllowlistContains returns true if name is present in reviewAllowlist.
func reviewAllowlistContains(name string) bool {
	for _, n := range reviewAllowlist {
		if n == name {
			return true
		}
	}
	return false
}

// TestPlanAllowlist_ContainsTodoCreate asserts planAllowlist includes todo_create (REQ-8).
func TestPlanAllowlist_ContainsTodoCreate(t *testing.T) {
	if !planAllowlistContains("todo_create") {
		t.Error("planAllowlist must contain 'todo_create' (REQ-8: plan mode permits todo creation)")
	}
}

// TestPlanAllowlist_ContainsTodoUpdate asserts planAllowlist includes todo_update (REQ-8).
func TestPlanAllowlist_ContainsTodoUpdate(t *testing.T) {
	if !planAllowlistContains("todo_update") {
		t.Error("planAllowlist must contain 'todo_update' (REQ-8: plan mode permits todo update)")
	}
}

// TestPlanAllowlist_ContainsTodoList asserts planAllowlist includes todo_list (REQ-8).
func TestPlanAllowlist_ContainsTodoList(t *testing.T) {
	if !planAllowlistContains("todo_list") {
		t.Error("planAllowlist must contain 'todo_list' (REQ-8: plan mode permits todo list)")
	}
}

// TestReviewAllowlist_ContainsTodoList asserts reviewAllowlist includes todo_list (REQ-8).
func TestReviewAllowlist_ContainsTodoList(t *testing.T) {
	if !reviewAllowlistContains("todo_list") {
		t.Error("reviewAllowlist must contain 'todo_list' (REQ-8: review mode permits reading the list)")
	}
}

// TestReviewAllowlist_DoesNotContainTodoCreate asserts reviewAllowlist does NOT include
// todo_create (REQ-8: review is read-only; AD-3 baseReadOnly extraction prevents leakage).
func TestReviewAllowlist_DoesNotContainTodoCreate(t *testing.T) {
	if reviewAllowlistContains("todo_create") {
		t.Error("reviewAllowlist must NOT contain 'todo_create' (REQ-8: review is read-only; AD-3 prevents leakage from planAllowlist)")
	}
}

// TestReviewAllowlist_DoesNotContainTodoUpdate asserts reviewAllowlist does NOT include
// todo_update (REQ-8: review is read-only).
func TestReviewAllowlist_DoesNotContainTodoUpdate(t *testing.T) {
	if reviewAllowlistContains("todo_update") {
		t.Error("reviewAllowlist must NOT contain 'todo_update' (REQ-8: review is read-only)")
	}
}

// TestReviewAllowlist_PreservesNonTodoTools verifies that the AD-3 restructuring
// did NOT remove any of the pre-existing read-only tools from reviewAllowlist.
// Checks a representative sample: Read, Grep, Glob, Bash, and several mem_*/codegraph_* tools.
func TestReviewAllowlist_PreservesNonTodoTools(t *testing.T) {
	required := []string{
		"Read", "Grep", "Glob", "WebFetch", "WebSearch",
		"Bash", // review gets Bash
		"mem_save", "mem_search", "mem_get_observation",
		"codegraph_search", "codegraph_context",
	}
	for _, name := range required {
		if !reviewAllowlistContains(name) {
			t.Errorf("reviewAllowlist missing %q — AD-3 restructuring must preserve all existing non-todo tools", name)
		}
	}
}

// TestPlanAllowlist_PreservesNonTodoTools verifies that the AD-3 restructuring
// did NOT remove any of the pre-existing read-only tools from planAllowlist.
func TestPlanAllowlist_PreservesNonTodoTools(t *testing.T) {
	required := []string{
		"Read", "Grep", "Glob", "WebFetch", "WebSearch",
		"mem_save", "mem_search", "mem_get_observation",
		"codegraph_search", "codegraph_context",
	}
	for _, name := range required {
		if !planAllowlistContains(name) {
			t.Errorf("planAllowlist missing %q — AD-3 restructuring must preserve all existing tools", name)
		}
	}
}

// TestPlanAllowlist_DoesNotContainBash asserts Bash is NOT in planAllowlist
// (plan is read-only for external tools; Bash is only in review).
func TestPlanAllowlist_DoesNotContainBash(t *testing.T) {
	if planAllowlistContains("Bash") {
		t.Error("planAllowlist must NOT contain 'Bash' — plan mode is read-only for shell commands")
	}
}

// ---------------------------------------------------------------------------
// Plan-prompt verbatim wording (REQ-8, contract-locked)
// ---------------------------------------------------------------------------

// planPromptVerbatim is the EXACT wording required by REQ-8 spec scenario
// "plan-prompt wording is exact". Tests assert this substring is present.
const planPromptVerbatim = "You MUST NOT take any EXTERNAL or IRREVERSIBLE action with side effects" +
	" — no file writes, no shell commands, no API mutations. Maintaining your internal planning" +
	" state (the todolist via todo_create / todo_update) is explicitly permitted:" +
	" the todolist IS your planning artifact, not an external side effect."

// TestPlanPrompt_ContainsVerbatimTodoWording asserts planPrompt contains the
// exact wording from REQ-8. This is a contract-locked assertion analogous to
// TestErrInvalidMode_ErrorString — if the string changes, this test breaks.
func TestPlanPrompt_ContainsVerbatimTodoWording(t *testing.T) {
	if !strings.Contains(planPrompt, planPromptVerbatim) {
		t.Errorf("planPrompt does not contain the required REQ-8 verbatim wording.\n"+
			"Want substring: %q\nGot planPrompt: %q", planPromptVerbatim, planPrompt)
	}
}

// ---------------------------------------------------------------------------
// Mode-gate integration tests (REQ-8: S8-1 to S8-4 for todo tools)
// ---------------------------------------------------------------------------

// todoGateTestProvider drives a single todo tool call and captures the result.
// Call 0: returns the specified tool call. Call 1: captures tool results and returns "done".
type todoGateTestProvider struct {
	toolName    string
	toolInput   json.RawMessage
	callCount   int
	toolResults []string
}

func (p *todoGateTestProvider) Name() string                                  { return "todo-gate" }
func (p *todoGateTestProvider) Model() string                                 { return "todo-gate-model" }
func (p *todoGateTestProvider) SupportsTools() bool                           { return true }
func (p *todoGateTestProvider) SupportsMultimodal() bool                      { return false }
func (p *todoGateTestProvider) SupportsAudio() bool                           { return false }
func (p *todoGateTestProvider) HealthCheck(_ context.Context) (string, error) { return "ok", nil }

func (p *todoGateTestProvider) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	defer func() { p.callCount++ }()
	if p.callCount == 0 {
		input := p.toolInput
		if input == nil {
			input = json.RawMessage(`{}`)
		}
		return &provider.ChatResponse{
			ToolCalls: []provider.ToolCall{
				{ID: "todo-gate-tc-1", Name: p.toolName, Input: input},
			},
		}, nil
	}
	// Second call: scan messages for tool results.
	for _, m := range req.Messages {
		if m.Role == "tool" {
			p.toolResults = append(p.toolResults, m.Content.TextOnly())
		}
	}
	return &provider.ChatResponse{Content: "done"}, nil
}

// newAgentInMode creates a minimal Agent with the given mode preset in conv.Metadata
// and the provided tools wired in. The convID must be unique per test.
func newAgentInMode(t *testing.T, mode string, convID string, prov provider.Provider, tools map[string]tool.Tool) *Agent {
	t.Helper()
	st := &mockStore{
		conv: &store.Conversation{
			ID:        convID,
			ChannelID: convID + "-ch",
			Metadata:  map[string]string{"daimon/mode": mode},
		},
	}
	ch := &mockChannel{}
	ag := New(
		config.AgentConfig{MaxIterations: 5, MaxTokensPerTurn: 100},
		defaultLimits(),
		config.FilterConfig{},
		ch,
		prov,
		st,
		audit.NoopAuditor{},
		tools,
		nil,
		skill.SkillIndex{},
		4,
		false,
	)
	return ag
}

// todoMockTool is a minimal tool.Tool that records invocations.
type todoMockTool struct {
	name   string
	calls  int
	result tool.ToolResult
}

func (m *todoMockTool) Name() string            { return m.name }
func (m *todoMockTool) Description() string     { return "mock " + m.name }
func (m *todoMockTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (m *todoMockTool) Execute(_ context.Context, _ json.RawMessage) (tool.ToolResult, error) {
	m.calls++
	if m.result.Content == "" {
		return tool.ToolResult{Content: m.name + " called"}, nil
	}
	return m.result, nil
}

// S8-R8-1: plan mode permits todo_create (call is not blocked by gate).
func TestModeGate_PlanMode_PermitsTodoCreate(t *testing.T) {
	t.Parallel()

	createTool := &todoMockTool{name: "todo_create"}
	prov := &todoGateTestProvider{
		toolName:  "todo_create",
		toolInput: json.RawMessage(`{"content":"test item"}`),
	}
	ag := newAgentInMode(t, "plan", "conv-gate-plan-create-1", prov,
		map[string]tool.Tool{"todo_create": createTool})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-gate-plan-create-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("create a todo"),
	})

	if createTool.calls == 0 {
		t.Error("REQ-8: plan mode must permit todo_create — tool was not called (mode gate blocked it)")
	}
}

// S8-R8-2: plan mode permits todo_update.
func TestModeGate_PlanMode_PermitsTodoUpdate(t *testing.T) {
	t.Parallel()

	updateTool := &todoMockTool{name: "todo_update"}
	prov := &todoGateTestProvider{
		toolName:  "todo_update",
		toolInput: json.RawMessage(`{"id":"td_00000001","status":"in_progress"}`),
	}
	ag := newAgentInMode(t, "plan", "conv-gate-plan-update-1", prov,
		map[string]tool.Tool{"todo_update": updateTool})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-gate-plan-update-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("update a todo"),
	})

	if updateTool.calls == 0 {
		t.Error("REQ-8: plan mode must permit todo_update — tool was not called (mode gate blocked it)")
	}
}

// S8-R8-3: plan mode permits todo_list.
func TestModeGate_PlanMode_PermitsTodoList(t *testing.T) {
	t.Parallel()

	listTool := &todoMockTool{name: "todo_list"}
	prov := &todoGateTestProvider{toolName: "todo_list"}
	ag := newAgentInMode(t, "plan", "conv-gate-plan-list-1", prov,
		map[string]tool.Tool{"todo_list": listTool})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-gate-plan-list-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("list todos"),
	})

	if listTool.calls == 0 {
		t.Error("REQ-8: plan mode must permit todo_list — tool was not called (mode gate blocked it)")
	}
}

// S8-R8-4: review mode BLOCKS todo_create (uses existing mode-gate error format).
func TestModeGate_ReviewMode_BlocksTodoCreate(t *testing.T) {
	t.Parallel()

	createTool := &todoMockTool{name: "todo_create"}
	prov := &todoGateTestProvider{
		toolName:  "todo_create",
		toolInput: json.RawMessage(`{"content":"test item"}`),
	}
	ag := newAgentInMode(t, "review", "conv-gate-review-create-1", prov,
		map[string]tool.Tool{"todo_create": createTool})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-gate-review-create-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("create a todo"),
	})

	// The tool must NOT have been called.
	if createTool.calls != 0 {
		t.Errorf("REQ-8: review mode must BLOCK todo_create — tool was called (calls=%d)", createTool.calls)
	}

	// The returned tool result must use the existing mode-gate error format.
	wantErr := "tool 'todo_create' not allowed in mode 'review'"
	found := false
	for _, r := range prov.toolResults {
		if strings.Contains(r, wantErr) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("REQ-8: expected mode-gate error %q in tool results; got: %v", wantErr, prov.toolResults)
	}
}

// S8-R8-5: review mode BLOCKS todo_update.
func TestModeGate_ReviewMode_BlocksTodoUpdate(t *testing.T) {
	t.Parallel()

	updateTool := &todoMockTool{name: "todo_update"}
	prov := &todoGateTestProvider{
		toolName:  "todo_update",
		toolInput: json.RawMessage(`{"id":"td_00000001","status":"completed"}`),
	}
	ag := newAgentInMode(t, "review", "conv-gate-review-update-1", prov,
		map[string]tool.Tool{"todo_update": updateTool})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-gate-review-update-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("update a todo"),
	})

	if updateTool.calls != 0 {
		t.Errorf("REQ-8: review mode must BLOCK todo_update — tool was called (calls=%d)", updateTool.calls)
	}

	wantErr := "tool 'todo_update' not allowed in mode 'review'"
	found := false
	for _, r := range prov.toolResults {
		if strings.Contains(r, wantErr) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("REQ-8: expected mode-gate error %q in tool results; got: %v", wantErr, prov.toolResults)
	}
}

// S8-R8-6: review mode permits todo_list (read-only, shared via baseReadOnly).
func TestModeGate_ReviewMode_PermitsTodoList(t *testing.T) {
	t.Parallel()

	listTool := &todoMockTool{name: "todo_list"}
	prov := &todoGateTestProvider{toolName: "todo_list"}
	ag := newAgentInMode(t, "review", "conv-gate-review-list-1", prov,
		map[string]tool.Tool{"todo_list": listTool})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-gate-review-list-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("list todos"),
	})

	if listTool.calls == 0 {
		t.Error("REQ-8: review mode must permit todo_list — tool was not called (mode gate blocked it)")
	}
}

// S8-R8-7: build mode permits all three todo tools (nil allowlist, no restriction).
func TestModeGate_BuildMode_PermitsAllTodoTools(t *testing.T) {
	t.Parallel()

	tools := []struct {
		name  string
		input json.RawMessage
	}{
		{"todo_create", json.RawMessage(`{"content":"test"}`)},
		{"todo_update", json.RawMessage(`{"id":"td_00000001","status":"completed"}`)},
		{"todo_list", nil},
	}

	for _, tc := range tools {
		tc := tc // capture
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			mt := &todoMockTool{name: tc.name}
			prov := &todoGateTestProvider{
				toolName:  tc.name,
				toolInput: tc.input,
			}
			convID := "conv-gate-build-" + tc.name + "-1"
			ag := newAgentInMode(t, "build", convID, prov,
				map[string]tool.Tool{tc.name: mt})

			ag.processMessage(context.Background(), channel.IncomingMessage{
				ChannelID: convID + "-ch",
				SenderID:  "u1",
				Content:   content.TextBlock("call " + tc.name),
			})

			if mt.calls == 0 {
				t.Errorf("REQ-8: build mode must permit %s — tool was not called (mode gate blocked it)", tc.name)
			}
		})
	}
}
