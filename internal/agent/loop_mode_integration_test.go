package agent

// loop_mode_integration_test.go — End-to-end execution-gate tests for the
// mode-system (Phase 7, REQ-8, AD-6, AD-11).
//
// These tests configure the agent in a specific mode, wire a mock provider that
// returns a tool call for a tool NOT in the mode's allowlist, and assert that:
//   - the execution gate fires BEFORE the tools-map lookup
//   - the EXACT error string from AD-11 is returned to the provider
//   - the actual tool implementation is NOT invoked
//
// All tests run under -race (the package-level go test -race flag covers them).
// Design references: AD-6 (gate before not-found), AD-11 (exact error wording),
// REQ-8 (S8-1, S8-2, S8-3, S8-4).

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/notify"
	"daimon/internal/provider"
	"daimon/internal/skill"
	"daimon/internal/store"
	"daimon/internal/tool"
)

// executionGateTestProvider is a provider that:
//   - On call 0: returns a tool call for toolName.
//   - On call 1: returns a final text response ("done"), after seeing the
//     tool result from the gate. Captures the tool result content from the
//     conversation messages so the test can assert it.
type executionGateTestProvider struct {
	toolName    string
	callCount   int
	toolResults []string // accumulated tool result contents from conv messages
}

func (p *executionGateTestProvider) Name() string                                  { return "gate-test" }
func (p *executionGateTestProvider) Model() string                                 { return "gate-model" }
func (p *executionGateTestProvider) SupportsTools() bool                           { return true }
func (p *executionGateTestProvider) SupportsMultimodal() bool                      { return false }
func (p *executionGateTestProvider) SupportsAudio() bool                           { return false }
func (p *executionGateTestProvider) HealthCheck(_ context.Context) (string, error) { return "ok", nil }

func (p *executionGateTestProvider) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	defer func() { p.callCount++ }()
	if p.callCount == 0 {
		// First call: return a tool call for the target tool.
		return &provider.ChatResponse{
			ToolCalls: []provider.ToolCall{
				{ID: "gate-tc-1", Name: p.toolName, Input: json.RawMessage(`{}`)},
			},
		}, nil
	}
	// Second call: scan the messages for tool results and capture them.
	for _, m := range req.Messages {
		if m.Role == "tool" {
			p.toolResults = append(p.toolResults, m.Content.TextOnly())
		}
	}
	return &provider.ChatResponse{Content: "done"}, nil
}

// S8-1: plan mode execution gate rejects tool not in allowlist.
// The EXACT error string from AD-11 must be returned to the model.
func TestExecutionGate_PlanMode_RejectsBash(t *testing.T) {
	t.Parallel()
	prov := &executionGateTestProvider{toolName: "Bash"}
	bash := &mockTool{name: "Bash"}
	st := &mockStore{
		conv: &store.Conversation{
			ID:        "conv_gate_ch1_u1",
			ChannelID: "gate-ch1",
			Metadata:  map[string]string{"daimon/mode": "plan"},
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
		map[string]tool.Tool{"Bash": bash},
		nil,
		skill.SkillIndex{},
		4,
		false,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "gate-ch1",
		SenderID:  "u1",
		Content:   content.TextBlock("run bash"),
	})

	// Bash must NOT have been called.
	if bash.calls != 0 {
		t.Errorf("S8-1: Bash tool was dispatched despite being blocked by execution gate (calls=%d)", bash.calls)
	}

	// The tool result seen by the model must be the EXACT error string from AD-11.
	wantErr := "tool 'Bash' not allowed in mode 'plan'"
	found := false
	for _, r := range prov.toolResults {
		if strings.Contains(r, wantErr) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("S8-1: expected exact error string %q in tool results; got: %v", wantErr, prov.toolResults)
	}
}

// S8-2: plan mode execution gate passes Read (in allowlist).
func TestExecutionGate_PlanMode_AllowsRead(t *testing.T) {
	t.Parallel()
	prov := &executionGateTestProvider{toolName: "Read"}
	readTool := &mockTool{name: "Read", result: tool.ToolResult{Content: "file contents"}}
	st := &mockStore{
		conv: &store.Conversation{
			ID:        "conv_gate_ch2_u2",
			ChannelID: "gate-ch2",
			Metadata:  map[string]string{"daimon/mode": "plan"},
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
		map[string]tool.Tool{"Read": readTool},
		nil,
		skill.SkillIndex{},
		4,
		false,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "gate-ch2",
		SenderID:  "u2",
		Content:   content.TextBlock("read a file"),
	})

	// Read MUST have been called (gate allowed it through).
	if readTool.calls == 0 {
		t.Error("S8-2: Read tool was NOT dispatched — gate should have allowed it")
	}
}

// S8-3: build mode (nil allowlist) execution gate passes all tools.
func TestExecutionGate_BuildMode_PassesAllTools(t *testing.T) {
	t.Parallel()
	prov := &executionGateTestProvider{toolName: "Bash"}
	bash := &mockTool{name: "Bash", result: tool.ToolResult{Content: "ok"}}
	st := &mockStore{
		conv: &store.Conversation{
			ID:        "conv_gate_ch3_u3",
			ChannelID: "gate-ch3",
			Metadata:  map[string]string{"daimon/mode": "build"},
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
		map[string]tool.Tool{"Bash": bash},
		nil,
		skill.SkillIndex{},
		4,
		false,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "gate-ch3",
		SenderID:  "u3",
		Content:   content.TextBlock("run bash"),
	})

	// Bash MUST have been called (build mode allows everything).
	if bash.calls == 0 {
		t.Error("S8-3: Bash tool was NOT dispatched — build mode should allow all tools")
	}
}

// S8-4: exact error string from AD-11 — NOT "not found".
// Uses Write (not in plan allowlist). Asserts:
//   - error contains "tool 'Write' not allowed in mode 'plan'"
//   - error does NOT contain "not found"
func TestExecutionGate_PlanMode_ExactErrorString_NotFound(t *testing.T) {
	t.Parallel()
	prov := &executionGateTestProvider{toolName: "Write"}
	writeTool := &mockTool{name: "Write"}
	st := &mockStore{
		conv: &store.Conversation{
			ID:        "conv_gate_ch4_u4",
			ChannelID: "gate-ch4",
			Metadata:  map[string]string{"daimon/mode": "plan"},
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
		map[string]tool.Tool{"Write": writeTool},
		nil,
		skill.SkillIndex{},
		4,
		false,
	)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "gate-ch4",
		SenderID:  "u4",
		Content:   content.TextBlock("write something"),
	})

	// Write must NOT have been called.
	if writeTool.calls != 0 {
		t.Errorf("S8-4: Write tool was dispatched despite plan mode block (calls=%d)", writeTool.calls)
	}

	wantErr := "tool 'Write' not allowed in mode 'plan'"
	found := false
	for _, r := range prov.toolResults {
		if strings.Contains(r, wantErr) {
			found = true
		}
		if strings.Contains(r, "not found") {
			t.Errorf("S8-4: error must not say 'not found'; got: %q", r)
		}
	}
	if !found {
		t.Errorf("S8-4: expected exact error %q; got: %v", wantErr, prov.toolResults)
	}
}

// ---------------------------------------------------------------------------
// Arg-gate integration tests (C5: AD-5, REQ-6, REQ-9)
// ---------------------------------------------------------------------------

// shellArgGateProvider drives a single shell_exec tool call with the given
// command JSON, then returns "done" and captures tool results.
type shellArgGateProvider struct {
	command     string // shell command to inject
	callCount   int
	toolResults []string
}

func (p *shellArgGateProvider) Name() string                                  { return "shell-arg-gate-test" }
func (p *shellArgGateProvider) Model() string                                 { return "shell-arg-gate-model" }
func (p *shellArgGateProvider) SupportsTools() bool                           { return true }
func (p *shellArgGateProvider) SupportsMultimodal() bool                      { return false }
func (p *shellArgGateProvider) SupportsAudio() bool                           { return false }
func (p *shellArgGateProvider) HealthCheck(_ context.Context) (string, error) { return "ok", nil }

func (p *shellArgGateProvider) Chat(_ context.Context, req provider.ChatRequest) (*provider.ChatResponse, error) {
	defer func() { p.callCount++ }()
	if p.callCount == 0 {
		input, _ := json.Marshal(map[string]string{"command": p.command})
		return &provider.ChatResponse{
			ToolCalls: []provider.ToolCall{
				{ID: "shell-arg-gate-tc-1", Name: "shell_exec", Input: json.RawMessage(input)},
			},
		}, nil
	}
	for _, m := range req.Messages {
		if m.Role == "tool" {
			p.toolResults = append(p.toolResults, m.Content.TextOnly())
		}
	}
	return &provider.ChatResponse{Content: "done"}, nil
}

// shellExecStub is a minimal tool.Tool stub with Name() == "shell_exec" (REQ-9).
// It records whether Execute was called. When Execute is called, it returns
// the configured result (default: non-error with "shell_exec called" content).
type shellExecStub struct {
	calls  int
	result tool.ToolResult
}

func (s *shellExecStub) Name() string        { return "shell_exec" }
func (s *shellExecStub) Description() string { return "shell_exec stub for testing" }
func (s *shellExecStub) Schema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"command":{"type":"string"}},"required":["command"]}`)
}
func (s *shellExecStub) Execute(_ context.Context, _ json.RawMessage) (tool.ToolResult, error) {
	s.calls++
	if s.result.Content == "" {
		return tool.ToolResult{Content: "shell_exec stub called"}, nil
	}
	return s.result, nil
}

// newAgentInModeWithTools creates an Agent in the given mode with the specified
// tool map. Reuses newAgentInMode infrastructure from modes_todo_test.go.
func newShellArgGateAgent(t *testing.T, mode, convID string, prov provider.Provider, shellStub *shellExecStub) *Agent {
	t.Helper()
	st := &mockStore{
		conv: &store.Conversation{
			ID:        convID,
			ChannelID: convID + "-ch",
			Metadata:  map[string]string{"daimon/mode": mode},
		},
	}
	ch := &mockChannel{}
	tools := map[string]tool.Tool{"shell_exec": shellStub}
	ag := New(
		config.AgentConfig{
			MaxIterations:    5,
			MaxTokensPerTurn: 100,
			// Explicitly set ContextMode to "off" so PreApply does NOT intercept
			// shell_exec calls — we want the tool stub's Execute to be called, not
			// the BoundedExec sandbox path.
			ContextMode: config.ContextModeConfig{Mode: config.ContextModeOff},
		},
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

// TestReviewModeShellArgGate_AllowedExecutes verifies that an allowed git diff
// command passes the arg gate in review mode and reaches Execute (REQ-2, AD-5).
func TestReviewModeShellArgGate_AllowedExecutes(t *testing.T) {
	t.Parallel()
	stub := &shellExecStub{}
	prov := &shellArgGateProvider{command: "git diff HEAD"}
	ag := newShellArgGateAgent(t, "review", "conv-shell-arg-allow-1", prov, stub)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-shell-arg-allow-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("run git diff"),
	})

	if stub.calls == 0 {
		t.Error("REQ-2: git diff HEAD should pass arg gate and reach Execute in review mode")
	}
	// No IsError result should come back from the gate.
	for _, r := range prov.toolResults {
		if strings.Contains(r, reviewShellRejectMsg) {
			t.Errorf("unexpected rejection for allowed command; result: %q", r)
		}
	}
}

// TestReviewModeShellArgGate_BlockedNotExecuted verifies that a mutating command
// (git commit) is rejected by the arg gate and Execute is NOT called (REQ-3, AD-5).
func TestReviewModeShellArgGate_BlockedNotExecuted(t *testing.T) {
	t.Parallel()
	stub := &shellExecStub{}
	prov := &shellArgGateProvider{command: "git commit -m 'x'"}
	ag := newShellArgGateAgent(t, "review", "conv-shell-arg-block-1", prov, stub)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-shell-arg-block-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("commit changes"),
	})

	if stub.calls != 0 {
		t.Errorf("REQ-3: git commit should be blocked; Execute was called (calls=%d)", stub.calls)
	}
	found := false
	for _, r := range prov.toolResults {
		if strings.Contains(r, reviewShellRejectMsg) {
			found = true
		}
	}
	if !found {
		t.Errorf("REQ-5: expected rejection message %q in tool results; got: %v", reviewShellRejectMsg, prov.toolResults)
	}
}

// TestReviewModeShellArgGate_MetacharBlocked verifies that a command with a shell
// metachar is rejected by the arg gate (REQ-4, AD-5). Execute must NOT be called.
func TestReviewModeShellArgGate_MetacharBlocked(t *testing.T) {
	t.Parallel()
	stub := &shellExecStub{}
	prov := &shellArgGateProvider{command: "git log; rm -rf /"}
	ag := newShellArgGateAgent(t, "review", "conv-shell-arg-meta-1", prov, stub)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-shell-arg-meta-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("show log"),
	})

	if stub.calls != 0 {
		t.Errorf("REQ-4: metachar command should be blocked; Execute was called (calls=%d)", stub.calls)
	}
	found := false
	for _, r := range prov.toolResults {
		if strings.Contains(r, reviewShellRejectMsg) {
			found = true
		}
	}
	if !found {
		t.Errorf("REQ-4: expected rejection message %q; got: %v", reviewShellRejectMsg, prov.toolResults)
	}
}

// TestPlanModeShellExec_NameGateBlocks verifies that in plan mode, shell_exec is
// rejected at the NAME-level gate (not in planAllowlist), not by the arg gate.
// The error must be the name-gate message (not reviewShellRejectMsg), proving
// isArgAllowed did NOT fire (REQ-7, AD-5).
func TestPlanModeShellExec_NameGateBlocks(t *testing.T) {
	t.Parallel()
	stub := &shellExecStub{}
	prov := &shellArgGateProvider{command: "git commit -m x"}
	ag := newShellArgGateAgent(t, "plan", "conv-shell-plan-1", prov, stub)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-shell-plan-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("commit"),
	})

	if stub.calls != 0 {
		t.Errorf("plan mode: shell_exec Execute should not be called (name gate blocks); got calls=%d", stub.calls)
	}
	wantNameErr := "tool 'shell_exec' not allowed in mode 'plan'"
	nameErrFound := false
	argErrFound := false
	for _, r := range prov.toolResults {
		if strings.Contains(r, wantNameErr) {
			nameErrFound = true
		}
		if strings.Contains(r, reviewShellRejectMsg) {
			argErrFound = true
		}
	}
	if !nameErrFound {
		t.Errorf("plan mode: expected name-gate error %q; got: %v", wantNameErr, prov.toolResults)
	}
	if argErrFound {
		t.Errorf("plan mode: arg gate must NOT fire (shell_exec not in planAllowlist); found arg rejection in: %v", prov.toolResults)
	}
}

// TestBuildModeShellExec_Unaffected verifies that in build mode, shell_exec with
// an otherwise-blocked command runs freely (nil ArgAllowlists, REQ-7).
func TestBuildModeShellExec_Unaffected(t *testing.T) {
	t.Parallel()
	stub := &shellExecStub{result: tool.ToolResult{Content: "build executed"}}
	prov := &shellArgGateProvider{command: "rm -rf dist && make all"}
	ag := newShellArgGateAgent(t, "build", "conv-shell-build-1", prov, stub)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-shell-build-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("build"),
	})

	if stub.calls == 0 {
		t.Error("REQ-7: build mode shell_exec should be unaffected by arg gate; Execute not called")
	}
	for _, r := range prov.toolResults {
		if strings.Contains(r, reviewShellRejectMsg) {
			t.Errorf("REQ-7: build mode must not produce arg rejection; got: %q", r)
		}
	}
}

// TestReviewModeShellArgGate_RejectionInConvMessages verifies that a rejected
// shell_exec call appears in conv.Messages as a tool-result (REQ-6).
// We drive the turn and then check that the provider SAW the tool result
// (meaning it was appended to conv.Messages and sent in the next Chat call).
func TestReviewModeShellArgGate_RejectionInConvMessages(t *testing.T) {
	t.Parallel()
	stub := &shellExecStub{}
	prov := &shellArgGateProvider{command: "git commit -m 'test'"}
	ag := newShellArgGateAgent(t, "review", "conv-shell-conv-msg-1", prov, stub)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-shell-conv-msg-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("commit"),
	})

	// The provider's second call receives the tool result from conv.Messages.
	// If the rejection was properly appended, toolResults will contain it.
	found := false
	for _, r := range prov.toolResults {
		if strings.Contains(r, reviewShellRejectMsg) {
			found = true
		}
	}
	if !found {
		t.Errorf("REQ-6: rejection must flow through conv.Messages to provider; got: %v", prov.toolResults)
	}
}

// ---------------------------------------------------------------------------
// W-1: Scenario 4.7 — metachar rejected even when shell Execute would otherwise
// run unconditionally (AllowAll-independence, REQ-4 Scenario 4.7)
// ---------------------------------------------------------------------------

// TestReviewModeShellArgGate_AllowAll_MetacharStillRejected pins Spec REQ-4
// Scenario 4.7: the arg gate in modes.go rejects a metacharacter command in
// review mode regardless of whether the underlying shell tool would execute it
// unconditionally (i.e., regardless of ShellToolConfig.AllowAll).
//
// Structural guarantee: isArgAllowed operates on ModeDefinition alone — it has
// no reference to ShellToolConfig and therefore cannot be overridden by AllowAll.
// ShellToolConfig.AllowAll only affects shell.go's Execute path, which the gate
// runs BEFORE (loop.go:728). shellExecStub.Execute represents the Execute path
// of an AllowAll=true shell: it executes any command without further checks.
// If the gate were bypassed, stub.calls would be nonzero and the test would fail.
func TestReviewModeShellArgGate_AllowAll_MetacharStillRejected(t *testing.T) {
	t.Parallel()

	// shellExecStub.Execute unconditionally executes (mirrors AllowAll=true shell).
	stub := &shellExecStub{result: tool.ToolResult{Content: "would execute if gate bypassed"}}
	// Use a metachar command that would pass the prefix-allowlist if the metachar
	// check were skipped: "git diff; echo pwned" — leading two tokens are "git diff"
	// (in allowlist), but the semicolon MUST be caught first (REQ-4, AD-2 Step 1).
	prov := &shellArgGateProvider{command: "git diff; echo pwned"}
	ag := newShellArgGateAgent(t, "review", "conv-shell-allowall-meta-1", prov, stub)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-shell-allowall-meta-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("show diff"),
	})

	// Gate MUST have blocked: Execute should never have been called.
	if stub.calls != 0 {
		t.Errorf("REQ-4/Scenario-4.7: arg gate did not block metachar command; Execute was called %d time(s) (AllowAll-independence violated)", stub.calls)
	}

	// The rejection message MUST appear in the tool result seen by the provider.
	found := false
	for _, r := range prov.toolResults {
		if strings.Contains(r, reviewShellRejectMsg) {
			found = true
		}
	}
	if !found {
		t.Errorf("REQ-4/Scenario-4.7: expected rejection message %q; got: %v", reviewShellRejectMsg, prov.toolResults)
	}
}

// ---------------------------------------------------------------------------
// S-1: Scenario 6.2 — bus tool.end event emitted on arg-rejected call (REQ-6)
// ---------------------------------------------------------------------------

// TestReviewModeShellArgGate_BusEventEmitted_OnArgRejection pins Spec REQ-6
// Scenario 6.2: a shell_exec call rejected by the arg gate STILL emits a
// notify.EventToolEnd bus event with IsError=true. The event flows through the
// unconditional bus-emit path in loop.go, identical to name-level blocks.
func TestReviewModeShellArgGate_BusEventEmitted_OnArgRejection(t *testing.T) {
	t.Parallel()

	rb := &recordingBus{}
	stub := &shellExecStub{}
	prov := &shellArgGateProvider{command: "git commit -m 'audit this'"}
	ag := newShellArgGateAgent(t, "review", "conv-shell-bus-event-1", prov, stub).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "conv-shell-bus-event-1-ch",
		SenderID:  "u1",
		Content:   content.TextBlock("commit"),
	})

	// Execute must NOT have been called (gate blocked).
	if stub.calls != 0 {
		t.Errorf("REQ-6/S6-2: Execute should not be called on rejected arg; got calls=%d", stub.calls)
	}

	// The bus MUST have received at least one EventToolEnd with IsError=true.
	ends := rb.filterByType(notify.EventToolEnd)
	if len(ends) == 0 {
		t.Fatal("REQ-6/S6-2: no EventToolEnd emitted on bus for arg-rejected shell_exec call")
	}
	var foundError bool
	for _, ev := range ends {
		if ev.ToolName == "shell_exec" && ev.IsError {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Errorf("REQ-6/S6-2: expected EventToolEnd with ToolName=shell_exec and IsError=true; got events: %+v", ends)
	}
}
