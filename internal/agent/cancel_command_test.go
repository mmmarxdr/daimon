package agent

import (
	"context"
	"strings"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// WU4 RED tests: /cancel built-in command (REQ-6)
// ---------------------------------------------------------------------------

// TestCmdCancel_ActiveTurn_RepliesConfirmation verifies that /cancel replies
// with a confirmation message when a turn is in progress for the caller.
func TestCmdCancel_ActiveTurn_RepliesConfirmation(t *testing.T) {
	cr := &capturedReply{}
	reg := NewCommandRegistry()

	cr2 := &capturedReply{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, &mockChannel{}, &mockProvider{}, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	// Register a cancel func so the /cancel handler sees an active turn.
	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	cancelCalled := false
	err := ag.cancels.Register(key, func() { cancelCalled = true })
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	registerBuiltinCommands(reg)
	cc := CommandContext{
		Ctx:       context.Background(),
		ChannelID: "chan:42",
		SenderID:  "user:7",
		Reply:     cr.reply,
		Registry:  reg,
		Config:    &config.AgentConfig{},
	}
	_ = cr2

	if err := ag.cmdCancel(cc); err != nil {
		t.Fatalf("cmdCancel returned error: %v", err)
	}

	if !cancelCalled {
		t.Error("expected cancel func to be invoked")
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d: %v", len(cr.messages), cr.messages)
	}
	if !strings.Contains(cr.messages[0], "cancellation requested") {
		t.Errorf("expected confirmation reply, got: %q", cr.messages[0])
	}
}

// TestCmdCancel_NoTurn_RepliesNeutral verifies that /cancel replies with a
// neutral message when no turn is in progress for the caller.
func TestCmdCancel_NoTurn_RepliesNeutral(t *testing.T) {
	cr := &capturedReply{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, &mockChannel{}, &mockProvider{}, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	cc := CommandContext{
		Ctx:       context.Background(),
		ChannelID: "chan:42",
		SenderID:  "user:7",
		Reply:     cr.reply,
		Config:    &config.AgentConfig{},
	}

	if err := ag.cmdCancel(cc); err != nil {
		t.Fatalf("cmdCancel returned error: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d: %v", len(cr.messages), cr.messages)
	}
	if !strings.Contains(cr.messages[0], "No turn in progress") {
		t.Errorf("expected 'No turn in progress' reply, got: %q", cr.messages[0])
	}
}

// TestCmdCancel_Idempotent_SecondCall_NeutralReply verifies that a second /cancel
// call after the first cancels returns the neutral message (idempotent).
func TestCmdCancel_Idempotent_SecondCall_NeutralReply(t *testing.T) {
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, &mockChannel{}, &mockProvider{}, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	if err := ag.cancels.Register(key, func() {}); err != nil {
		t.Fatalf("Register: %v", err)
	}

	cc := CommandContext{
		Ctx:       context.Background(),
		ChannelID: "chan:42",
		SenderID:  "user:7",
		Reply:     func(string) {}, // discard first reply
		Config:    &config.AgentConfig{},
	}

	// First call should cancel.
	if err := ag.cmdCancel(cc); err != nil {
		t.Fatalf("first cmdCancel: %v", err)
	}

	// Second call — no turn in progress.
	cr := &capturedReply{}
	cc.Reply = cr.reply
	if err := ag.cmdCancel(cc); err != nil {
		t.Fatalf("second cmdCancel: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply on second call, got %d", len(cr.messages))
	}
	if !strings.Contains(cr.messages[0], "No turn in progress") {
		t.Errorf("expected 'No turn in progress' on second call, got: %q", cr.messages[0])
	}
}

// TestCmdCancel_DoesNotCallLLM verifies that /cancel never causes an LLM call.
func TestCmdCancel_DoesNotCallLLM(t *testing.T) {
	prov := &mockProvider{}
	ch := &mockChannel{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, ch, prov, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	ag.processMessage(context.Background(), channel.IncomingMessage{
		ChannelID: "chan:1",
		SenderID:  "user:1",
		Content:   content.TextBlock("/cancel"),
	})

	if prov.callCount() != 0 {
		t.Errorf("expected 0 LLM calls for /cancel, got %d", prov.callCount())
	}
}
