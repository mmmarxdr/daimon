package agent

import (
	"context"
	"encoding/json"
	"testing"

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
// REQ-9.2 — per-turn agent.tokens.usage emitted once at turn end
// ---------------------------------------------------------------------------

// TestProcessMessage_TokensUsage_EmittedOnce_AtTurnEnd verifies exactly one
// agent.tokens.usage event is emitted on the bus at the end of a turn.
func TestProcessMessage_TokensUsage_EmittedOnce_AtTurnEnd(t *testing.T) {
	rb := &recordingBus{}

	// Provider: one tool call followed by a final text response.
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-1", Name: "my_tool", Input: json.RawMessage(`{}`)},
				},
				Usage: provider.UsageStats{InputTokens: 500, OutputTokens: 100},
			},
			{
				Content: "final answer",
				Usage:   provider.UsageStats{InputTokens: 600, OutputTokens: 200},
			},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{
			"my_tool": &mockTool{name: "my_tool", result: tool.ToolResult{Content: "ok"}},
		},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("do work"),
	})

	usageEvs := rb.filterByType(notify.EventTokensUsage)
	if len(usageEvs) != 1 {
		t.Fatalf("expected exactly 1 agent.tokens.usage event, got %d", len(usageEvs))
	}
	ev := usageEvs[0]

	// TokenCount = total output tokens across all iterations.
	wantOutput := 100 + 200
	if ev.TokenCount != wantOutput {
		t.Errorf("TokenCount = %d, want %d (sum of all output tokens)", ev.TokenCount, wantOutput)
	}
	// Meta must carry input_tokens, output_tokens, elapsed_ms, conv_id.
	if ev.Meta["input_tokens"] == "" {
		t.Error("Meta[input_tokens] must be set")
	}
	if ev.Meta["output_tokens"] == "" {
		t.Error("Meta[output_tokens] must be set")
	}
	if ev.Meta["elapsed_ms"] == "" {
		t.Error("Meta[elapsed_ms] must be set")
	}
	if ev.Meta["conv_id"] == "" {
		t.Error("Meta[conv_id] must be set")
	}
	if ev.Origin != notify.OriginAgent {
		t.Errorf("Origin = %v, want OriginAgent", ev.Origin)
	}
}

// ---------------------------------------------------------------------------
// REQ-9.3 — nil bus: no panic, no emission
// ---------------------------------------------------------------------------

func TestProcessMessage_TokensUsage_NilBus_NoPanic(t *testing.T) {
	// Agent with nil bus — must not panic.
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "text only", Usage: provider.UsageStats{InputTokens: 100, OutputTokens: 50}},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	)
	// ag.bus is nil — must not panic.
	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-nil",
		Content:   content.TextBlock("no bus"),
	})
}

// ---------------------------------------------------------------------------
// REQ-9.2 — fields match totals across all iterations
// ---------------------------------------------------------------------------

func TestProcessMessage_TokensUsage_FieldsMatchTotals(t *testing.T) {
	rb := &recordingBus{}

	// Two iterations: 1st returns tool call, 2nd returns final text.
	// Token counts from both iterations should be summed.
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-x", Name: "adder_tool", Input: json.RawMessage(`{}`)},
				},
				Usage: provider.UsageStats{InputTokens: 1500, OutputTokens: 300},
			},
			{
				Content: "summed up",
				Usage:   provider.UsageStats{InputTokens: 200, OutputTokens: 50},
			},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{
			"adder_tool": &mockTool{name: "adder_tool", result: tool.ToolResult{Content: "42"}},
		},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("add"),
	})

	usageEvs := rb.filterByType(notify.EventTokensUsage)
	if len(usageEvs) != 1 {
		t.Fatalf("expected 1 usage event, got %d", len(usageEvs))
	}
	ev := usageEvs[0]

	wantIn := 1500 + 200
	wantOut := 300 + 50

	if ev.TokenCount != wantOut {
		t.Errorf("TokenCount (output) = %d, want %d", ev.TokenCount, wantOut)
	}

	// Meta must also reflect correct totals.
	wantInStr := "1700"
	wantOutStr := "350"
	if ev.Meta["input_tokens"] != wantInStr {
		t.Errorf("Meta[input_tokens] = %q, want %q", ev.Meta["input_tokens"], wantInStr)
	}
	if ev.Meta["output_tokens"] != wantOutStr {
		t.Errorf("Meta[output_tokens] = %q, want %q", ev.Meta["output_tokens"], wantOutStr)
	}

	// Suppress "declared and not used" for wantIn.
	_ = wantIn
}

// ---------------------------------------------------------------------------
// Seam 2 — ADR-2/ADR-3: category fields on EventTokensUsage (RED → GREEN)
// ---------------------------------------------------------------------------

// TestEmitTokensUsage_PopulatesCategoryFields: when contextMgr has a cached
// breakdown, EventTokensUsage carries matching SysToks/MsgToks/ToolToks.
// White-box: seeds lastUsage/hasBreakdown directly (same package). The
// ContextManager is configured with strategy "none" so Manage() does not
// overwrite the seeded values, giving us a deterministic round-trip assertion.
func TestEmitTokensUsage_PopulatesCategoryFields(t *testing.T) {
	rb := &recordingBus{}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "answer", Usage: provider.UsageStats{InputTokens: 100, OutputTokens: 50}},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	// Build a ContextManager whose Manage() path is "none" so it does NOT
	// overwrite lastUsage. Then seed a known per-category breakdown directly.
	cmCfg := config.ContextConfig{
		Strategy:        "none",
		FallbackCtxSize: 200000,
	}
	ctxMgr := NewContextManager(cmCfg, func() provider.Provider { return prov }, nil)

	// Seed known values directly (same-package white-box access is valid here).
	const wantSys, wantMsg, wantTool = 1500, 4200, 800
	ctxMgr.lastUsage = TokenUsage{
		SystemPrompt: wantSys,
		Messages:     wantMsg,
		Tools:        wantTool,
		Total:        wantSys + wantMsg + wantTool,
	}
	ctxMgr.hasBreakdown = true

	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)
	ag.contextMgr = ctxMgr

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("test query"),
	})

	usageEvs := rb.filterByType(notify.EventTokensUsage)
	if len(usageEvs) != 1 {
		t.Fatalf("expected 1 EventTokensUsage, got %d", len(usageEvs))
	}
	ev := usageEvs[0]

	// Exact round-trip: the seeded breakdown must survive through the emit path.
	if ev.SysToks != wantSys {
		t.Errorf("SysToks = %d, want %d", ev.SysToks, wantSys)
	}
	if ev.MsgToks != wantMsg {
		t.Errorf("MsgToks = %d, want %d", ev.MsgToks, wantMsg)
	}
	if ev.ToolToks != wantTool {
		t.Errorf("ToolToks = %d, want %d", ev.ToolToks, wantTool)
	}
}

// TestEmitTokensUsage_NilOrNoBreakdown_ZeroCategories: nil contextMgr and
// hasBreakdown==false cases produce SysToks==MsgToks==ToolToks==0.
func TestEmitTokensUsage_NilOrNoBreakdown_ZeroCategories(t *testing.T) {
	rb := &recordingBus{}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "answer", Usage: provider.UsageStats{InputTokens: 100, OutputTokens: 50}},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}

	// Agent with nil contextMgr (legacy path, no smart strategy).
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)
	// contextMgr is nil by default in New().

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-1",
		Content:   content.TextBlock("test"),
	})

	usageEvs := rb.filterByType(notify.EventTokensUsage)
	if len(usageEvs) != 1 {
		t.Fatalf("expected 1 EventTokensUsage, got %d", len(usageEvs))
	}
	ev := usageEvs[0]
	if ev.SysToks != 0 {
		t.Errorf("nil contextMgr: SysToks = %d, want 0", ev.SysToks)
	}
	if ev.MsgToks != 0 {
		t.Errorf("nil contextMgr: MsgToks = %d, want 0", ev.MsgToks)
	}
	if ev.ToolToks != 0 {
		t.Errorf("nil contextMgr: ToolToks = %d, want 0", ev.ToolToks)
	}
}
