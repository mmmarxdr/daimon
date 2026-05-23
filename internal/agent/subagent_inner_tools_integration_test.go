package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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
// WU10 integration helpers
// ---------------------------------------------------------------------------

// buildSubagentAgent builds an agent with two tools that simulates a subagent
// (conv has ParentConvID set) and is wired to the given bus.
func buildSubagentAgent(bus notify.Bus, subagentID string, tools map[string]tool.Tool, toolCalls []provider.ToolCall) *Agent {
	subConv := &store.Conversation{
		ID:           "sub_" + subagentID,
		ChannelID:    "subagent",
		ParentConvID: "conv-parent",
		Metadata: map[string]string{
			"subagent_id": subagentID,
			"batch_id":    "batch-1",
			"skill":       "test-skill",
		},
	}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{ToolCalls: toolCalls},
			{Content: "all done"},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{conv: subConv}

	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		tools, nil, skill.SkillIndex{}, 4, false,
	).withBus(bus)
	return ag
}

// ---------------------------------------------------------------------------
// REQ-20.1 — subagent with 2 tools → 4 ordered events on filtered subscription
// ---------------------------------------------------------------------------

// TestSubagentInnerToolEvents_FilteredSubscriberReceivesFourOrderedEvents verifies
// that a subscriber filtered by subagent_id receives exactly 4 tool lifecycle
// events: start-A, end-A, start-B, end-B in emission order, with IsError=false.
func TestSubagentInnerToolEvents_FilteredSubscriberReceivesFourOrderedEvents(t *testing.T) {
	realBus := notify.NewEventBus(256, 1000, 5*time.Second)
	defer realBus.Close()

	subagentID := "sub-X"

	// Set up a subscriber filtered by subagent_id.
	received := make(chan notify.Event, 32)
	realBus.Subscribe(func(ev notify.Event) {
		if ev.Meta["subagent_id"] != subagentID {
			return
		}
		if ev.Type != notify.EventToolStart && ev.Type != notify.EventToolEnd {
			return
		}
		select {
		case received <- ev:
		default:
		}
	})

	toolsMap := map[string]tool.Tool{
		"tool_A": &mockTool{name: "tool_A", result: tool.ToolResult{Content: "a"}},
		"tool_B": &mockTool{name: "tool_B", result: tool.ToolResult{Content: "b"}},
	}
	toolCalls := []provider.ToolCall{
		{ID: "tc-A", Name: "tool_A", Input: json.RawMessage(`{}`)},
		{ID: "tc-B", Name: "tool_B", Input: json.RawMessage(`{}`)},
	}

	ag := buildSubagentAgent(realBus, subagentID, toolsMap, toolCalls)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ConversationID: "sub_" + subagentID,
		ChannelID:      "subagent",
		Content:        content.TextBlock("do two tools"),
	})

	// Collect exactly 4 events.
	var evs []notify.Event
	deadline := time.After(2 * time.Second)
	for len(evs) < 4 {
		select {
		case ev := <-received:
			evs = append(evs, ev)
		case <-deadline:
			t.Fatalf("timeout waiting for 4 events; got %d: %v", len(evs), evs)
		}
	}

	// Verify types in order: start, end, start, end.
	wantTypes := []string{
		notify.EventToolStart,
		notify.EventToolEnd,
		notify.EventToolStart,
		notify.EventToolEnd,
	}
	for i, ev := range evs {
		if ev.Type != wantTypes[i] {
			t.Errorf("event[%d]: got type %q, want %q", i, ev.Type, wantTypes[i])
		}
	}

	// Verify all end events have IsError=false.
	for i, ev := range evs {
		if ev.Type == notify.EventToolEnd && ev.IsError {
			t.Errorf("event[%d] (tool.end) has IsError=true, want false", i)
		}
	}

	// Verify all events carry the subagent_id.
	for i, ev := range evs {
		if ev.Meta["subagent_id"] != subagentID {
			t.Errorf("event[%d] missing subagent_id; Meta=%v", i, ev.Meta)
		}
	}

	// Drain any extras (should be none, but be safe).
	select {
	case extra := <-received:
		t.Errorf("unexpected extra event: %v", extra)
	case <-time.After(100 * time.Millisecond):
		// ok
	}
}

// ---------------------------------------------------------------------------
// REQ-20.2 — top-level events NOT visible in subagent-filtered subscription
// ---------------------------------------------------------------------------

// TestSubagentInnerToolEvents_TopLevelEvents_NotInSubagentFilter verifies that
// top-level agent tool events are not delivered to a filter for subagent "sub-X".
func TestSubagentInnerToolEvents_TopLevelEvents_NotInSubagentFilter(t *testing.T) {
	realBus := notify.NewEventBus(256, 1000, 5*time.Second)
	defer realBus.Close()

	subagentID := "sub-X"

	// Subscribe filtering for subagent sub-X.
	received := make(chan notify.Event, 16)
	realBus.Subscribe(func(ev notify.Event) {
		if ev.Meta["subagent_id"] != subagentID {
			return
		}
		if ev.Type == notify.EventToolStart || ev.Type == notify.EventToolEnd {
			select {
			case received <- ev:
			default:
			}
		}
	})

	// Run a top-level agent (no subagent conv).
	topProv := &mockProvider{
		responses: []provider.ChatResponse{
			{
				ToolCalls: []provider.ToolCall{
					{ID: "tc-top", Name: "top_tool", Input: json.RawMessage(`{}`)},
				},
			},
			{Content: "done"},
		},
	}
	topCh := &mockChannel{}
	topSt := &mockStore{} // no pre-seeded conv → fresh top-level conv
	topAg := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		topCh, topProv, topSt, audit.NoopAuditor{},
		map[string]tool.Tool{
			"top_tool": &mockTool{name: "top_tool", result: tool.ToolResult{Content: "ok"}},
		},
		nil, skill.SkillIndex{}, 4, false,
	).withBus(realBus)

	topAg.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-top",
		Content:   content.TextBlock("top level"),
	})

	// Give a brief window for any events to arrive.
	select {
	case ev := <-received:
		t.Errorf("top-level event leaked into subagent filter: %v", ev)
	case <-time.After(200 * time.Millisecond):
		// correct: no events for subagent sub-X from top-level agent
	}
}
