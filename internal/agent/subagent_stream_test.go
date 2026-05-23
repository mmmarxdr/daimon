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
	"daimon/internal/store"
	"daimon/internal/tool"
)

// ---------------------------------------------------------------------------
// WU8 helpers
// ---------------------------------------------------------------------------

// newSubagentAgent builds a minimal Agent simulating a subagent context.
// The agent's store is pre-seeded with a conversation that has ParentConvID
// set so mergeSubagentMeta returns the 4 attribution keys.
func newSubagentAgent(rb *recordingBus, toolName string, tr tool.ToolResult) *Agent {
	subConv := &store.Conversation{
		ID:           "sub_abc",
		ChannelID:    "subagent",
		ParentConvID: "conv-parent-123",
		Metadata: map[string]string{
			"subagent_id": "sub-abc",
			"batch_id":    "batch-7",
			"skill":       "summarize",
		},
	}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-sub-1", Name: toolName, Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{conv: subConv}

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
// REQ-2.3, REQ-10.2 — subagent tool.start carries all 4 attribution keys
// ---------------------------------------------------------------------------

func TestProcessMessage_SubagentTurn_ToolStartCarriesAttribution(t *testing.T) {
	rb := &recordingBus{}
	ag := newSubagentAgent(rb, "my_tool", tool.ToolResult{Content: "ok"})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ConversationID: "sub_abc",
		ChannelID:      "subagent",
		Content:        content.TextBlock("do it"),
	})

	starts := rb.filterByType(notify.EventToolStart)
	if len(starts) == 0 {
		t.Fatal("no agent.tool.start emitted")
	}
	ev := starts[0]

	checkAttribution(t, ev.Meta)
}

// ---------------------------------------------------------------------------
// REQ-3.3 — subagent tool.end carries all 4 attribution keys
// ---------------------------------------------------------------------------

func TestProcessMessage_SubagentTurn_ToolEndCarriesAttribution(t *testing.T) {
	rb := &recordingBus{}
	ag := newSubagentAgent(rb, "my_tool", tool.ToolResult{Content: "ok"})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ConversationID: "sub_abc",
		ChannelID:      "subagent",
		Content:        content.TextBlock("do it"),
	})

	ends := rb.filterByType(notify.EventToolEnd)
	if len(ends) == 0 {
		t.Fatal("no agent.tool.end emitted")
	}
	ev := ends[0]

	checkAttribution(t, ev.Meta)
}

// ---------------------------------------------------------------------------
// REQ-10.2 — subagent turn events also carry attribution keys (turn.completed)
// ---------------------------------------------------------------------------

func TestProcessMessage_SubagentTurn_TurnEventsCarryAttribution(t *testing.T) {
	rb := &recordingBus{}
	ag := newSubagentAgent(rb, "my_tool", tool.ToolResult{Content: "ok"})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ConversationID: "sub_abc",
		ChannelID:      "subagent",
		Content:        content.TextBlock("do it"),
	})

	completeds := rb.filterByType(notify.EventTurnCompleted)
	if len(completeds) == 0 {
		t.Fatal("no agent.turn.completed emitted")
	}
	ev := completeds[0]
	checkAttribution(t, ev.Meta)
}

// ---------------------------------------------------------------------------
// REQ-10.2 — turn.started also carries attribution keys (symmetric with
// turn.completed). The emit site is reordered after conv load so subagentMeta
// is available; consumers that filter by subagent_id from TurnStarted onward
// MUST receive the keys.
// ---------------------------------------------------------------------------

func TestProcessMessage_SubagentTurn_TurnStartedCarriesAttribution(t *testing.T) {
	rb := &recordingBus{}
	ag := newSubagentAgent(rb, "my_tool", tool.ToolResult{Content: "ok"})

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ConversationID: "sub_abc",
		ChannelID:      "subagent",
		Content:        content.TextBlock("do it"),
	})

	starts := rb.filterByType(notify.EventTurnStarted)
	if len(starts) == 0 {
		t.Fatal("no agent.turn.started emitted")
	}
	ev := starts[0]
	checkAttribution(t, ev.Meta)
}

// ---------------------------------------------------------------------------
// REQ-10.1 — top-level turn.started MUST NOT carry subagent attribution keys.
// Guards the symmetric invariant: the reorder must not leak meta to top-level.
// ---------------------------------------------------------------------------

func TestProcessMessage_TopLevelTurn_TurnStartedNoSubagentMeta(t *testing.T) {
	rb := &recordingBus{}
	prov := &mockProvider{
		responses: []provider.ChatResponse{{Content: "ok"}},
	}
	ch := &mockChannel{}
	st := &mockStore{} // no pre-seeded conv → fresh top-level conv
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-top",
		Content:   content.TextBlock("hi"),
	})

	starts := rb.filterByType(notify.EventTurnStarted)
	if len(starts) == 0 {
		t.Fatal("no agent.turn.started emitted")
	}
	ev := starts[0]
	if _, ok := ev.Meta["subagent_id"]; ok {
		t.Errorf("top-level turn.started must not carry subagent_id, got Meta=%v", ev.Meta)
	}
	if _, ok := ev.Meta["parent_conv_id"]; ok {
		t.Errorf("top-level turn.started must not carry parent_conv_id, got Meta=%v", ev.Meta)
	}
}

// ---------------------------------------------------------------------------
// REQ-10.1 — top-level tool.start MUST NOT carry subagent attribution keys
// ---------------------------------------------------------------------------

func TestProcessMessage_TopLevelTurn_NoSubagentMeta(t *testing.T) {
	rb := &recordingBus{}
	// Top-level agent: no pre-seeded subagent conversation.
	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-top", Name: "top_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{} // no conv → agent creates fresh top-level conv
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		map[string]tool.Tool{
			"top_tool": &mockTool{name: "top_tool", result: tool.ToolResult{Content: "ok"}},
		},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-top",
		Content:   content.TextBlock("run"),
	})

	starts := rb.filterByType(notify.EventToolStart)
	if len(starts) == 0 {
		t.Fatal("no agent.tool.start emitted")
	}
	ev := starts[0]

	if _, ok := ev.Meta["subagent_id"]; ok {
		t.Errorf("top-level tool.start must not carry subagent_id, got Meta=%v", ev.Meta)
	}
	if _, ok := ev.Meta["parent_conv_id"]; ok {
		t.Errorf("top-level tool.start must not carry parent_conv_id, got Meta=%v", ev.Meta)
	}
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

func checkAttribution(t *testing.T, meta map[string]string) {
	t.Helper()
	checks := map[string]string{
		"subagent_id":    "sub-abc",
		"parent_conv_id": "conv-parent-123",
		"batch_id":       "batch-7",
		"skill":          "summarize",
	}
	for k, want := range checks {
		if got := meta[k]; got != want {
			t.Errorf("Meta[%q] = %q, want %q", k, got, want)
		}
	}
}
