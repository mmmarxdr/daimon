package agent

// mode_command_test.go — RED tests for the /mode slash command (PR3, Phase 8).
//
// Spec coverage:
//   S1-1: /mode (no args) → exact list output, current mode marked with *
//   S1-2: /mode (no args) after switching to plan → * plan marker
//   S2-1: /mode <name> swap happy path
//   S2-2: /mode <name> during active turn → ErrTurnInProgress reply
//   S3-1: /mode <unknown> → ErrInvalidMode reply (contains "unknown mode" and name)
//   S3-2: /mode (no-arg) with plan as current → * plan
//   S10-1: ErrTurnInProgress exact reply string
//   S12-1: Telemetry frame emitted on successful swap (type, mode, channel_id)
//   S12-2: Telemetry NOT emitted on error (e.g. ErrInvalidMode)
//   S12-3: Telemetry emit failure does NOT propagate (best-effort per AD-7)
//
// REQs covered: REQ-1, REQ-2, REQ-3, REQ-10, REQ-12.
// AD-11: ALL error/reply strings are contract-locked; exact wording asserted.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/config"
	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// telemetryCapturingChannel embeds mockChannel and captures EmitTelemetry calls.
// It implements channel.TelemetryEmitter so the agent can call EmitTelemetry.
type telemetryCapturingChannel struct {
	mockChannel
	mu         sync.Mutex
	frames     []map[string]any
	emitErr    error // if non-nil, EmitTelemetry returns this error
	emitCalled bool
}

func (t *telemetryCapturingChannel) EmitTelemetry(_ context.Context, _ string, frame map[string]any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.emitCalled = true
	t.frames = append(t.frames, frame)
	return t.emitErr
}

func (t *telemetryCapturingChannel) capturedFrames() []map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]map[string]any, len(t.frames))
	copy(result, t.frames)
	return result
}

// buildAgentForModeCmd constructs a minimal Agent for /mode command tests.
// The channel is a telemetryCapturingChannel so telemetry assertions can inspect emitted frames.
func buildAgentForModeCmd(t *testing.T) (*Agent, *telemetryCapturingChannel) {
	t.Helper()
	ch := &telemetryCapturingChannel{}
	st := &mockStore{}
	a := New(
		config.AgentConfig{},
		defaultLimits(),
		config.FilterConfig{},
		ch,
		&mockProvider{},
		st,
		audit.NoopAuditor{},
		nil,
		nil,
		skill.SkillIndex{},
		4,
		false,
	)
	return a, ch
}

// makeModeCC builds a CommandContext for /mode command tests.
// args is the raw argument string after the command name ("" for no-arg, "plan" for swap).
func makeModeCC(a *Agent, cr *capturedReply, args string) CommandContext {
	return CommandContext{
		Ctx:       context.Background(),
		ChannelID: "chan:test",
		SenderID:  "user:test",
		Args:      args,
		Store:     &mockStore{},
		Config:    &config.AgentConfig{},
		Reply:     cr.reply,
		Registry:  a.commands,
	}
}

// ---------------------------------------------------------------------------
// S14-1 (Phase 9): IsDestructiveCommand("mode") returns true
// ---------------------------------------------------------------------------

func TestIsDestructiveCommand_Mode_ReturnsTrue(t *testing.T) {
	if !IsDestructiveCommand("mode") {
		t.Error("IsDestructiveCommand(\"mode\") = false, want true")
	}
}

// ---------------------------------------------------------------------------
// S1-1: /mode (no args) — exact list output format, build is current
// ---------------------------------------------------------------------------

func TestCmdMode_S1_1_NoArg_ListsAllModes_BuildCurrent(t *testing.T) {
	a, _ := buildAgentForModeCmd(t)
	cr := &capturedReply{}
	cc := makeModeCC(a, cr, "")

	if err := a.cmdMode(cc); err != nil {
		t.Fatalf("cmdMode returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]

	// AD-11 exact per-line wording
	if !strings.Contains(reply, "Available modes:") {
		t.Errorf("reply missing 'Available modes:': %q", reply)
	}
	// build is the default current mode
	if !strings.Contains(reply, "* build") {
		t.Errorf("reply missing '* build' (current mode marker): %q", reply)
	}
	// non-current modes use two-space indent
	if !strings.Contains(reply, "  plan") {
		t.Errorf("reply missing '  plan': %q", reply)
	}
	if !strings.Contains(reply, "  review") {
		t.Errorf("reply missing '  review': %q", reply)
	}
	// usage footer
	if !strings.Contains(reply, "Use /mode <name> to switch.") {
		t.Errorf("reply missing usage line: %q", reply)
	}
}

// ---------------------------------------------------------------------------
// S1-2: /mode (no args) after switching to plan → * plan
// ---------------------------------------------------------------------------

func TestCmdMode_S1_2_NoArg_AfterPlanSwitch_PlanMarked(t *testing.T) {
	a, _ := buildAgentForModeCmd(t)
	// Set mode to plan directly via the cache (avoids store dependency).
	a.modeMu.Lock()
	a.currentMode = "plan"
	a.modeMu.Unlock()

	cr := &capturedReply{}
	cc := makeModeCC(a, cr, "")

	if err := a.cmdMode(cc); err != nil {
		t.Fatalf("cmdMode returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]

	if !strings.Contains(reply, "* plan") {
		t.Errorf("reply missing '* plan': %q", reply)
	}
	if !strings.Contains(reply, "  build") {
		t.Errorf("reply missing '  build': %q", reply)
	}
	if !strings.Contains(reply, "  review") {
		t.Errorf("reply missing '  review': %q", reply)
	}
}

// ---------------------------------------------------------------------------
// S2-1: /mode <name> swap happy path
// ---------------------------------------------------------------------------

func TestCmdMode_S2_1_Swap_HappyPath(t *testing.T) {
	a, _ := buildAgentForModeCmd(t)
	cr := &capturedReply{}
	cc := makeModeCC(a, cr, "plan")

	if err := a.cmdMode(cc); err != nil {
		t.Fatalf("cmdMode returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	// Must acknowledge the swap — normative reply per AD-11
	if !strings.Contains(reply, "plan") {
		t.Errorf("expected confirmation reply to mention 'plan', got: %q", reply)
	}
	// currentMode must have been updated
	snap := a.modeSnapshot()
	if snap.Name != "plan" {
		t.Errorf("modeSnapshot().Name = %q after swap, want %q", snap.Name, "plan")
	}
}

// ---------------------------------------------------------------------------
// S3-1: /mode <unknown> → reply contains "unknown mode" and the name (AD-11)
// ---------------------------------------------------------------------------

func TestCmdMode_S3_1_UnknownMode_ReplyContainsExactWording(t *testing.T) {
	a, _ := buildAgentForModeCmd(t)
	cr := &capturedReply{}
	cc := makeModeCC(a, cr, "banana")

	if err := a.cmdMode(cc); err != nil {
		t.Fatalf("cmdMode returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]
	if !strings.Contains(reply, "unknown mode") {
		t.Errorf("reply missing 'unknown mode': %q", reply)
	}
	if !strings.Contains(reply, "banana") {
		t.Errorf("reply missing the invalid mode name 'banana': %q", reply)
	}
}

// ---------------------------------------------------------------------------
// S3-2: ErrInvalidMode sentinel is returned by SetMode for unknown names
// ---------------------------------------------------------------------------

func TestSetMode_S3_2_EmptyName_ReturnsErrInvalidMode(t *testing.T) {
	a, _ := buildAgentForModeCmd(t)
	err := a.SetMode(context.Background(), "ch", "user", "")
	if err == nil {
		t.Fatal("expected error for empty mode name, got nil")
	}
	// SetMode returns ErrInvalidMode for unknown names. The error message from
	// LookupMode wraps ErrInvalidMode whose sentinel contains "invalid mode name".
	// The user-facing "unknown mode" wording is surfaced by cmdModeSwap, not SetMode.
	if !errors.Is(err, ErrInvalidMode) {
		t.Errorf("expected errors.Is(err, ErrInvalidMode) = true, err = %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// S2-2 / S10-1: /mode during active turn → ErrTurnInProgress exact reply
// ---------------------------------------------------------------------------

func TestCmdMode_S10_1_TurnInProgress_ExactReply(t *testing.T) {
	a, _ := buildAgentForModeCmd(t)

	// Inject a fake turn in flight.
	key := cancelKey{ChannelID: "chan:test", SenderID: "user:test"}
	if err := a.cancels.Register(key, func() {}); err != nil {
		t.Fatalf("failed to register fake turn: %v", err)
	}
	t.Cleanup(func() { a.cancels.Cancel(key) })

	cr := &capturedReply{}
	cc := makeModeCC(a, cr, "plan")

	if err := a.cmdMode(cc); err != nil {
		t.Fatalf("cmdMode returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	// AD-11 exact string
	want := "A turn is currently in progress. Try again in a moment, or use /cancel first."
	if cr.messages[0] != want {
		t.Errorf("got reply:\n  %q\nwant:\n  %q", cr.messages[0], want)
	}

	// currentMode must be unchanged (still build)
	snap := a.modeSnapshot()
	if snap.Name != "build" {
		t.Errorf("currentMode changed during turn-in-progress; got %q, want %q", snap.Name, "build")
	}
}

// ---------------------------------------------------------------------------
// S12-1: Telemetry frame emitted on successful swap
// ---------------------------------------------------------------------------

func TestCmdMode_S12_1_TelemetryEmittedOnSuccess(t *testing.T) {
	a, ch := buildAgentForModeCmd(t)
	cr := &capturedReply{}
	cc := makeModeCC(a, cr, "plan")

	if err := a.cmdMode(cc); err != nil {
		t.Fatalf("cmdMode returned error: %v", err)
	}
	if !ch.emitCalled {
		t.Fatal("expected EmitTelemetry to be called on successful swap, but it was not")
	}
	frames := ch.capturedFrames()
	if len(frames) == 0 {
		t.Fatal("expected at least one telemetry frame, got none")
	}
	frame := frames[0]

	// Must contain type, mode fields per REQ-12
	if frame["type"] != "mode.changed" {
		t.Errorf("frame[\"type\"] = %q, want %q", frame["type"], "mode.changed")
	}
	if frame["mode"] != "plan" {
		t.Errorf("frame[\"mode\"] = %q, want %q", frame["mode"], "plan")
	}
}

// ---------------------------------------------------------------------------
// S12-2: Telemetry NOT emitted when SetMode fails (e.g., invalid mode name)
// ---------------------------------------------------------------------------

func TestCmdMode_S12_2_TelemetryNotEmittedOnError(t *testing.T) {
	a, ch := buildAgentForModeCmd(t)
	cr := &capturedReply{}
	cc := makeModeCC(a, cr, "banana")

	if err := a.cmdMode(cc); err != nil {
		t.Fatalf("cmdMode returned error: %v", err)
	}
	if ch.emitCalled {
		t.Error("expected EmitTelemetry NOT to be called on failed swap, but it was")
	}
}

// ---------------------------------------------------------------------------
// S12-3: Telemetry emit failure does NOT propagate (best-effort per AD-7)
// ---------------------------------------------------------------------------

func TestCmdMode_S12_3_TelemetryEmitFailureDoesNotPropagate(t *testing.T) {
	a, ch := buildAgentForModeCmd(t)
	// Configure the channel to fail on EmitTelemetry.
	ch.emitErr = errors.New("network failure")

	cr := &capturedReply{}
	cc := makeModeCC(a, cr, "plan")

	// cmdMode must not return an error even when telemetry emit fails.
	if err := a.cmdMode(cc); err != nil {
		t.Fatalf("cmdMode returned error (should be nil even on emit failure): %v", err)
	}
	// Must still reply with the success message.
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.Contains(cr.messages[0], "plan") {
		t.Errorf("expected success reply mentioning 'plan', got: %q", cr.messages[0])
	}
}

// ---------------------------------------------------------------------------
// S1: /mode is registered as a builtin command
// ---------------------------------------------------------------------------

func TestCmdMode_RegisteredAsBuiltin(t *testing.T) {
	a, _ := buildAgentForModeCmd(t)
	entry, ok := a.commands.commands["mode"]
	if !ok {
		t.Fatal("command 'mode' not found in registry")
	}
	if entry.source != SourceBuiltin {
		t.Errorf("command 'mode' source = %q, want %q", entry.source, SourceBuiltin)
	}
}

// ---------------------------------------------------------------------------
// S11-2: /mode appears in Built-in section of /help output
// ---------------------------------------------------------------------------

func TestCmdMode_AppearsInHelpBuiltinSection(t *testing.T) {
	a, _ := buildAgentForModeCmd(t)
	cr := &capturedReply{}
	cc := CommandContext{
		Ctx:      context.Background(),
		Reply:    cr.reply,
		Registry: a.commands,
	}

	if err := cmdHelp(cc); err != nil {
		t.Fatalf("cmdHelp returned error: %v", err)
	}
	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	reply := cr.messages[0]

	if !strings.Contains(reply, "Built-in commands:") {
		t.Fatalf("reply missing 'Built-in commands:' section")
	}
	if !strings.Contains(reply, "/mode") {
		t.Fatalf("reply missing '/mode' in output: %q", reply)
	}
}
