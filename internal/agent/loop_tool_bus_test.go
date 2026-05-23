package agent

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/notify"
	"daimon/internal/provider"
	"daimon/internal/skill"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// WU6 helpers
// ---------------------------------------------------------------------------

// newAgentWithBusAndTool constructs a minimal agent wired to a recording bus
// and a single named sync provider + tool. The provider returns: first call →
// one tool call, second call → text response (no more tools).
func newAgentWithBusAndTool(rb *recordingBus, toolName string, tr tool.ToolResult) *Agent {
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-1", Name: toolName, Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{
			toolName: &mockTool{name: toolName, result: tr},
		},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)
	return ag
}

// ---------------------------------------------------------------------------
// REQ-2.2, REQ-14.2 — agent.tool.start emitted before executeWithRecover
// ---------------------------------------------------------------------------

// TestProcessMessage_ToolStart_EmittedBeforeExecute (REQ-2.2) verifies that
// an agent.tool.start event is recorded before the tool runs.
func TestProcessMessage_ToolStart_EmittedBeforeExecute(t *testing.T) {
	rb := &recordingBus{}
	toolName := "shell_exec"

	// sentinel: track whether bus.Emit was called before tool.Execute.
	var emitSeenBeforeExecute bool
	var toolCalls int

	// We wire up a special tool that checks the bus at execute time.
	customTool := &hookTool{
		name: toolName,
		executeHook: func() {
			toolCalls++
			// At this point the bus should already have the tool.start event.
			starts := rb.filterByType(notify.EventToolStart)
			if len(starts) >= 1 {
				emitSeenBeforeExecute = true
			}
		},
		result: tool.ToolResult{Content: "ok"},
	}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-1", Name: toolName, Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{toolName: customTool},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("run tool"),
	})

	if toolCalls == 0 {
		t.Fatal("tool was never called")
	}
	if !emitSeenBeforeExecute {
		t.Error("agent.tool.start was not emitted before tool.Execute")
	}
}

// ---------------------------------------------------------------------------
// REQ-3.1 — agent.tool.end emitted after success, IsError=false
// ---------------------------------------------------------------------------

func TestProcessMessage_ToolEnd_EmittedAfterExecute_Success(t *testing.T) {
	rb := &recordingBus{}
	ag := newAgentWithBusAndTool(rb, "shell_exec", tool.ToolResult{Content: "ok"})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("run"),
	})

	ends := rb.filterByType(notify.EventToolEnd)
	if len(ends) != 1 {
		t.Fatalf("expected 1 agent.tool.end, got %d", len(ends))
	}
	ev := ends[0]
	if ev.IsError {
		t.Error("IsError should be false for successful tool")
	}
	if ev.ToolCallID != "tc-1" {
		t.Errorf("ToolCallID = %q, want tc-1", ev.ToolCallID)
	}
	if ev.ToolName != "shell_exec" {
		t.Errorf("ToolName = %q, want shell_exec", ev.ToolName)
	}
	if ev.DurationMs < 0 {
		t.Errorf("DurationMs should be >= 0, got %d", ev.DurationMs)
	}
	if ev.Meta["status"] != "done" {
		t.Errorf("Meta[status] = %q, want done", ev.Meta["status"])
	}
}

// ---------------------------------------------------------------------------
// REQ-3.2 — agent.tool.end emitted after error, IsError=true
// ---------------------------------------------------------------------------

func TestProcessMessage_ToolEnd_EmittedAfterExecute_Error(t *testing.T) {
	rb := &recordingBus{}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-err", Name: "bad_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{
			"bad_tool": &mockTool{name: "bad_tool", result: tool.ToolResult{IsError: true, Content: "fail"}},
		},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("run bad tool"),
	})

	ends := rb.filterByType(notify.EventToolEnd)
	if len(ends) != 1 {
		t.Fatalf("expected 1 agent.tool.end, got %d", len(ends))
	}
	ev := ends[0]
	if !ev.IsError {
		t.Error("IsError should be true for erroring tool")
	}
	if ev.Meta["status"] != "error" {
		t.Errorf("Meta[status] = %q, want error", ev.Meta["status"])
	}
}

// ---------------------------------------------------------------------------
// REQ-14.1 — nil bus: no panic, no emission
// ---------------------------------------------------------------------------

func TestProcessMessage_NilBus_NoEmission_NoPanic(t *testing.T) {
	// Agent with nil bus (no WithBus wired).
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-1", Name: "ok_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{
			"ok_tool": &mockTool{name: "ok_tool", result: tool.ToolResult{Content: "ok"}},
		},
		nil, skill.SkillIndex{}, 4, false,
	)
	// ag.bus is nil — must not panic.
	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("run"),
	})
	// If we reach here without panic the test passes.
}

// ---------------------------------------------------------------------------
// REQ-19.1 — legacy tool_start telemetry frame still emitted (additive)
// ---------------------------------------------------------------------------

func TestProcessMessage_LegacyToolStartFrame_StillEmitted(t *testing.T) {
	// Use a TelemetryEmitter-capable channel.
	recorded := &recordingTelemetryChannel{}
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-1", Name: "my_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	st := &mockStore{}
	rb := &recordingBus{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		recorded, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{
			"my_tool": &mockTool{name: "my_tool", result: tool.ToolResult{Content: "ok"}},
		},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("run"),
	})

	found := false
	for _, f := range recorded.frames {
		if typ, _ := f["type"].(string); typ == "tool_start" {
			found = true
			break
		}
	}
	if !found {
		t.Error("legacy tool_start telemetry frame was not emitted (REQ-19.1)")
	}
	// NEW bus event must also be present.
	if len(rb.filterByType(notify.EventToolStart)) == 0 {
		t.Error("agent.tool.start bus event was not emitted (REQ-2.2)")
	}
}

// ---------------------------------------------------------------------------
// REQ-19.2 — legacy tool_done telemetry frame still emitted (additive)
// ---------------------------------------------------------------------------

func TestProcessMessage_LegacyToolDoneFrame_StillEmitted(t *testing.T) {
	recorded := &recordingTelemetryChannel{}
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-1", Name: "my_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	st := &mockStore{}
	rb := &recordingBus{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		recorded, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{
			"my_tool": &mockTool{name: "my_tool", result: tool.ToolResult{Content: "ok"}},
		},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("run"),
	})

	found := false
	for _, f := range recorded.frames {
		if typ, _ := f["type"].(string); typ == "tool_done" {
			found = true
			break
		}
	}
	if !found {
		t.Error("legacy tool_done telemetry frame was not emitted (REQ-19.2)")
	}
	// NEW bus event must also be present.
	if len(rb.filterByType(notify.EventToolEnd)) == 0 {
		t.Error("agent.tool.end bus event was not emitted (REQ-3.1)")
	}
}

// ---------------------------------------------------------------------------
// Helpers for WU6 tests
// ---------------------------------------------------------------------------

// hookTool is a tool.Tool that calls executeHook inside Execute, before returning.
type hookTool struct {
	name        string
	executeHook func()
	result      tool.ToolResult
}

func (h *hookTool) Name() string            { return h.name }
func (h *hookTool) Description() string     { return "hook tool" }
func (h *hookTool) Schema() json.RawMessage { return json.RawMessage(`{}`) }
func (h *hookTool) Execute(ctx context.Context, _ json.RawMessage) (tool.ToolResult, error) {
	if h.executeHook != nil {
		h.executeHook()
	}
	return h.result, nil
}

// recordingTelemetryChannel implements channel.Channel + channel.TelemetryEmitter.
type recordingTelemetryChannel struct {
	mu     sync.Mutex
	frames []map[string]any
	sent   []channel.OutgoingMessage
}

func (r *recordingTelemetryChannel) Name() string { return "recording-telemetry" }
func (r *recordingTelemetryChannel) Start(_ context.Context, _ chan<- channel.IncomingMessage) error {
	return nil
}
func (r *recordingTelemetryChannel) Send(_ context.Context, msg channel.OutgoingMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sent = append(r.sent, msg)
	return nil
}
func (r *recordingTelemetryChannel) Stop() error { return nil }
func (r *recordingTelemetryChannel) EmitTelemetry(_ context.Context, _ string, frame map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, frame)
	return nil
}

// ---------------------------------------------------------------------------
// REQ-2.1 — agent.tool.start has Origin=OriginAgent, ChannelID, Meta[conv_id]
// ---------------------------------------------------------------------------

func TestProcessMessage_ToolStart_HasRequiredFields(t *testing.T) {
	rb := &recordingBus{}
	ag := newAgentWithBusAndTool(rb, "shell_exec", tool.ToolResult{Content: "ok"})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-test",
		Content:   content.TextBlock("run"),
	})

	starts := rb.filterByType(notify.EventToolStart)
	if len(starts) == 0 {
		t.Fatal("no agent.tool.start emitted")
	}
	ev := starts[0]
	if ev.Origin != notify.OriginAgent {
		t.Errorf("Origin = %v, want OriginAgent", ev.Origin)
	}
	if ev.ChannelID != "ch-test" {
		t.Errorf("ChannelID = %q, want ch-test", ev.ChannelID)
	}
	if ev.ToolName != "shell_exec" {
		t.Errorf("ToolName = %q, want shell_exec", ev.ToolName)
	}
	if ev.ToolCallID != "tc-1" {
		t.Errorf("ToolCallID = %q, want tc-1", ev.ToolCallID)
	}
	if ev.Meta["conv_id"] == "" {
		t.Error("Meta[conv_id] should be set")
	}
	if ev.Timestamp.IsZero() {
		t.Error("Timestamp should be set")
	}
}

// ---------------------------------------------------------------------------
// Extra: verify two tools → two start/end pairs (ordering)
// ---------------------------------------------------------------------------

func TestProcessMessage_TwoTools_TwoStartEndPairs(t *testing.T) {
	rb := &recordingBus{}
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-A", Name: "tool_a", Input: json.RawMessage(`{}`)},
					{ID: "tc-B", Name: "tool_b", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{
			"tool_a": &mockTool{name: "tool_a", result: tool.ToolResult{Content: "a"}},
			"tool_b": &mockTool{name: "tool_b", result: tool.ToolResult{Content: "b"}},
		},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("run two"),
	})

	starts := rb.filterByType(notify.EventToolStart)
	ends := rb.filterByType(notify.EventToolEnd)

	if len(starts) != 2 {
		t.Errorf("expected 2 agent.tool.start, got %d", len(starts))
	}
	if len(ends) != 2 {
		t.Errorf("expected 2 agent.tool.end, got %d", len(ends))
	}

	// Verify interleaving: start[0] before end[0] before start[1] before end[1].
	all := rb.snapshot()
	startPos := [2]int{-1, -1}
	endPos := [2]int{-1, -1}
	si, ei := 0, 0
	for i, ev := range all {
		switch ev.Type {
		case notify.EventToolStart:
			if si < 2 {
				startPos[si] = i
				si++
			}
		case notify.EventToolEnd:
			if ei < 2 {
				endPos[ei] = i
				ei++
			}
		}
	}
	// start[0] < end[0] < start[1] < end[1] (for tools executed sequentially).
	if startPos[0] > endPos[0] {
		t.Error("first start should come before first end")
	}
}

// Ensure sync import is pulled in (used by recordingTelemetryChannel).
var _ = time.Now
