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
