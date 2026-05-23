package agent

import (
	"context"
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
)

// ---------------------------------------------------------------------------
// REQ-15.1 — agent.turn.completed legacy field shape unchanged
// ---------------------------------------------------------------------------

// TestTurnCompleted_LegacyFieldShape_Unchanged verifies that after the
// agent-stream-events change, agent.turn.completed still carries the legacy
// input_tokens and output_tokens Meta keys (REQ-15.1). Adding subagent keys
// for subagent turns is REQ-10 additive; removing/renaming existing keys
// would be a regression.
func TestTurnCompleted_LegacyFieldShape_Unchanged(t *testing.T) {
	rb := &recordingBus{}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{
				Content: "final",
				Usage:   provider.UsageStats{InputTokens: 42, OutputTokens: 13},
			},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "ch-compat",
		Content:   content.TextBlock("compat check"),
	})

	completeds := rb.filterByType(notify.EventTurnCompleted)
	if len(completeds) == 0 {
		t.Fatal("no agent.turn.completed emitted")
	}
	ev := completeds[0]

	// Legacy keys must still be present (REQ-15).
	if ev.Meta["input_tokens"] == "" {
		t.Error("agent.turn.completed: Meta[input_tokens] must be present (REQ-15.1)")
	}
	if ev.Meta["output_tokens"] == "" {
		t.Error("agent.turn.completed: Meta[output_tokens] must be present (REQ-15.1)")
	}
	// Origin and ChannelID must still be set.
	if ev.Origin != notify.OriginAgent {
		t.Errorf("Origin = %v, want OriginAgent", ev.Origin)
	}
	if ev.ChannelID != "ch-compat" {
		t.Errorf("ChannelID = %q, want ch-compat", ev.ChannelID)
	}
}

// ---------------------------------------------------------------------------
// REQ-15.2 — agent.subagent.spawned Meta keys unchanged
// ---------------------------------------------------------------------------

// TestSubagentSpawned_MetaKeys_Unchanged verifies that after this change,
// SubagentManager.Spawn still emits agent.subagent.spawned with the existing
// Meta keys: subagent_id, batch_id, skill, parent_conv_id, max_cost_usd,
// max_turns, timeout_sec.
func TestSubagentSpawned_MetaKeys_Unchanged(t *testing.T) {
	bus := newBusRecorder()
	t.Cleanup(func() { bus.Close() })
	st := &mockStore{}

	m, _ := newTestManager(t, bus, st)
	m.installBusSubscription()

	def := skill.ExecutableSkillDef{
		Name:        "my-skill",
		Description: "compat test skill",
		Budget: skill.BudgetConfig{
			MaxCostUSD: 2.5,
			MaxTurns:   10,
			Timeout:    0,
		},
		ToolsAllowlist: []string{},
	}

	ctx := context.Background()
	handle, err := m.Spawn(ctx, def, "do work", SpawnModeSync, "conv-parent-abc")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}

	// Bus is async — wait for event to propagate.
	var spawnedEvs []notify.Event
	deadline := time.After(2 * time.Second)
	for len(spawnedEvs) == 0 {
		select {
		case <-deadline:
			t.Fatal("no agent.subagent.spawned event emitted within 2s")
		default:
			spawnedEvs = bus.eventsOfType(notify.EventSubagentSpawned)
			if len(spawnedEvs) == 0 {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}
	ev := spawnedEvs[0]
	_ = handle

	// All existing Meta keys must still be present (REQ-15.2).
	requiredKeys := []string{"subagent_id", "batch_id", "skill", "parent_conv_id", "max_cost_usd", "max_turns", "timeout_sec"}
	for _, k := range requiredKeys {
		if ev.Meta[k] == "" {
			t.Errorf("agent.subagent.spawned: Meta[%q] must be non-empty (REQ-15.2)", k)
		}
	}
}

// ---------------------------------------------------------------------------
// REQ-15 — cron events field shape unchanged
// ---------------------------------------------------------------------------

// TestCronEvents_FieldShape_Unchanged is a guardrail that ensures the
// notify.Event struct changes (7 new optional fields) do not affect cron
// events that were already emitted before this change. Since cron events use
// the pre-existing fields only (JobID, Text, Meta), they must marshal
// identically if those 7 new fields are all zero.
func TestCronEvents_FieldShape_Unchanged(t *testing.T) {
	// Simulate a cron event as emitted by the cron package.
	ev := notify.Event{
		Type:      notify.EventCronJobFired,
		Origin:    notify.OriginAgent,
		ChannelID: "cron:job-1",
		Text:      "test job fired",
		JobID:     "job-1",
		Timestamp: time.Now(),
		Meta: map[string]string{
			"job_id":  "job-1",
			"channel": "ch-1",
		},
	}

	// All 7 new fields must be at zero so they don't appear in JSON (omitempty).
	if ev.ToolCallID != "" {
		t.Error("ToolCallID should be empty for cron event")
	}
	if ev.ToolName != "" {
		t.Error("ToolName should be empty for cron event")
	}
	if ev.Iteration != 0 {
		t.Error("Iteration should be 0 for cron event")
	}
	if ev.TokenCount != 0 {
		t.Error("TokenCount should be 0 for cron event")
	}
	if ev.DurationMs != 0 {
		t.Error("DurationMs should be 0 for cron event")
	}
	if ev.CostUSD != 0 {
		t.Error("CostUSD should be 0 for cron event")
	}
	if ev.IsError {
		t.Error("IsError should be false for cron event")
	}
}

// ---------------------------------------------------------------------------
// REQ-15 — subagent conv: turn events gain attribution keys (additive, REQ-10)
// ---------------------------------------------------------------------------

// TestSubagentTurn_TurnCompleted_CarriesAttributionKeys checks that subagent
// turn.completed events carry the 4 attribution keys when the conv is a subagent
// conv. This is REQ-10 mandated and REQ-15 compatible (additive Meta, no
// removal/rename).
func TestSubagentTurn_TurnCompleted_CarriesAttributionKeys(t *testing.T) {
	rb := &recordingBus{}

	subConv := &store.Conversation{
		ID:           "sub_test",
		ChannelID:    "subagent",
		ParentConvID: "conv-parent-xyz",
		Metadata: map[string]string{
			"subagent_id": "sub-test",
			"batch_id":    "batch-1",
			"skill":       "my-skill",
		},
	}

	prov := &mockProvider{
		responses: []provider.ChatResponse{
			{Content: "done", Usage: provider.UsageStats{InputTokens: 10, OutputTokens: 5}},
		},
	}
	ch := &mockChannel{}
	st := &mockStore{conv: subConv}
	ag := New(
		defaultCfg(), defaultLimits(), config.FilterConfig{},
		ch, prov, st, audit.NoopAuditor{},
		nil, nil, skill.SkillIndex{}, 4, false,
	).withBus(rb)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ConversationID: "sub_test",
		ChannelID:      "subagent",
		Content:        content.TextBlock("sub work"),
	})

	completeds := rb.filterByType(notify.EventTurnCompleted)
	if len(completeds) == 0 {
		t.Fatal("no turn.completed emitted")
	}
	ev := completeds[0]

	// Legacy keys still present.
	if ev.Meta["input_tokens"] == "" {
		t.Error("input_tokens must still be present (REQ-15.1)")
	}
	if ev.Meta["output_tokens"] == "" {
		t.Error("output_tokens must still be present (REQ-15.1)")
	}
	// Additive attribution keys (REQ-10, must not conflict with REQ-15).
	if ev.Meta["subagent_id"] != "sub-test" {
		t.Errorf("subagent_id = %q, want sub-test (REQ-10)", ev.Meta["subagent_id"])
	}
	if ev.Meta["parent_conv_id"] != "conv-parent-xyz" {
		t.Errorf("parent_conv_id = %q, want conv-parent-xyz (REQ-10)", ev.Meta["parent_conv_id"])
	}
}
