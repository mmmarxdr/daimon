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
