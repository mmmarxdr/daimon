package agent

import (
	"context"
	"encoding/json"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/provider"
	"daimon/internal/skill"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// WU6 RED tests: processMessage injects effective cwd into shell tool ctx
// (REQ-4, REQ-5, closes W1 from verify-pr2)
// ---------------------------------------------------------------------------

// ctxCapturingTool is a mock tool that captures the context it receives during
// Execute. Used to verify that the agent injects the effective cwd into the ctx
// before invoking the tool.
type ctxCapturingTool struct {
	name        string
	capturedCtx context.Context
	result      tool.ToolResult
}

func (m *ctxCapturingTool) Name() string            { return m.name }
func (m *ctxCapturingTool) Description() string     { return "ctx capturing tool" }
func (m *ctxCapturingTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (m *ctxCapturingTool) Execute(ctx context.Context, params json.RawMessage) (tool.ToolResult, error) {
	m.capturedCtx = ctx
	return m.result, nil
}

// TestProcessMessage_InjectsCwdOverride_WhenOverrideSet verifies that when a
// per-(channelID, senderID) shell cwd override is set on the agent, processMessage
// injects tool.WithEffectiveCwd into the tool execution context.
func TestProcessMessage_InjectsCwdOverride_WhenOverrideSet(t *testing.T) {
	const overridePath = "/tmp/test-cwd-override"

	captureTool := &ctxCapturingTool{
		name:   "mock_tool",
		result: tool.ToolResult{Content: "captured"},
	}

	// Two-response provider: first returns a tool call, second returns text.
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "t1", Name: "mock_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}

	ag := New(
		defaultCfg(),
		defaultLimits(),
		config.FilterConfig{},
		&mockChannel{},
		prov,
		&mockStore{},
		audit.NoopAuditor{},
		map[string]tool.Tool{"mock_tool": captureTool},
		nil,
		skill.SkillIndex{},
		4,
		false,
	)

	// Set a cwd override for this (channel, sender) pair.
	key := cancelKey{ChannelID: "chan:1", SenderID: "user:1"}
	if err := ag.shellCwd.Set(key, overridePath); err != nil {
		t.Fatalf("Set cwd override: %v", err)
	}

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:1",
		SenderID:  "user:1",
		Content:   content.TextBlock("do something"),
	})

	// The tool should have been called.
	if captureTool.capturedCtx == nil {
		t.Fatal("expected tool to be called, but capturedCtx is nil")
	}

	// The context should carry the override path.
	gotCwd, ok := tool.EffectiveCwdFromCtx(captureTool.capturedCtx)
	if !ok {
		t.Fatal("EffectiveCwdFromCtx returned ok=false; expected the cwd override to be injected into tool ctx")
	}
	if gotCwd != overridePath {
		t.Errorf("expected effective cwd %q, got %q", overridePath, gotCwd)
	}
}

// TestProcessMessage_NoOverride_NoCwdInCtx verifies that when no cwd override
// is set, the tool ctx does NOT carry an effective cwd value.
func TestProcessMessage_NoOverride_NoCwdInCtx(t *testing.T) {
	captureTool := &ctxCapturingTool{
		name:   "mock_tool",
		result: tool.ToolResult{Content: "captured"},
	}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "t1", Name: "mock_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}

	ag := New(
		defaultCfg(),
		defaultLimits(),
		config.FilterConfig{},
		&mockChannel{},
		prov,
		&mockStore{},
		audit.NoopAuditor{},
		map[string]tool.Tool{"mock_tool": captureTool},
		nil,
		skill.SkillIndex{},
		4,
		false,
	)

	// No override set.

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:2",
		SenderID:  "user:2",
		Content:   content.TextBlock("do something"),
	})

	if captureTool.capturedCtx == nil {
		t.Fatal("expected tool to be called")
	}

	_, ok := tool.EffectiveCwdFromCtx(captureTool.capturedCtx)
	if ok {
		t.Error("expected no effective cwd in ctx when no override is set, but EffectiveCwdFromCtx returned ok=true")
	}
}

// TestProcessMessage_CwdOverride_PerUserIsolation verifies that cwd overrides
// are per-(channelID, senderID) and don't bleed across users.
func TestProcessMessage_CwdOverride_PerUserIsolation(t *testing.T) {
	const cwdA = "/tmp/user-a-cwd"
	// user-b has NO override

	captureTool := &ctxCapturingTool{
		name:   "mock_tool",
		result: tool.ToolResult{Content: "captured"},
	}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "t1", Name: "mock_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
			{
				ToolCalls: []provider.ToolCall{
					{ID: "t2", Name: "mock_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}

	ag := New(
		defaultCfg(),
		defaultLimits(),
		config.FilterConfig{},
		&mockChannel{},
		prov,
		&mockStore{},
		audit.NoopAuditor{},
		map[string]tool.Tool{"mock_tool": captureTool},
		nil,
		skill.SkillIndex{},
		4,
		false,
	)

	// Set override only for user-a.
	keyA := cancelKey{ChannelID: "chan:1", SenderID: "user-a"}
	if err := ag.shellCwd.Set(keyA, cwdA); err != nil {
		t.Fatalf("Set cwd for user-a: %v", err)
	}

	// Message from user-a — should see the override.
	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:1",
		SenderID:  "user-a",
		Content:   content.TextBlock("from a"),
	})
	if captureTool.capturedCtx == nil {
		t.Fatal("user-a: expected tool call")
	}
	gotA, okA := tool.EffectiveCwdFromCtx(captureTool.capturedCtx)
	if !okA || gotA != cwdA {
		t.Errorf("user-a: expected cwd %q, got %q (ok=%v)", cwdA, gotA, okA)
	}

	// Reset capture.
	captureTool.capturedCtx = nil

	// Message from user-b on the same channel — should NOT see any cwd.
	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:1",
		SenderID:  "user-b",
		Content:   content.TextBlock("from b"),
	})
	if captureTool.capturedCtx == nil {
		t.Fatal("user-b: expected tool call")
	}
	_, okB := tool.EffectiveCwdFromCtx(captureTool.capturedCtx)
	if okB {
		t.Error("user-b: expected no cwd override in ctx, but got one")
	}
}
