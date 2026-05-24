package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"daimon/internal/audit"
	"daimon/internal/channel"
	"daimon/internal/config"
	"daimon/internal/content"
	"daimon/internal/skill"
)

// ---------------------------------------------------------------------------
// WU5 RED tests: /cd built-in command (REQ-5, REQ-23)
// ---------------------------------------------------------------------------

// makeTestAgent creates a minimal agent for /cd command tests.
func makeTestAgent(t *testing.T) *Agent {
	t.Helper()
	return New(defaultCfg(), defaultLimits(), config.FilterConfig{}, &mockChannel{}, &mockProvider{}, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)
}

// makeCdCC creates a CommandContext for /cd tests with the given args.
func makeCdCC(ag *Agent, cr *capturedReply, args string) CommandContext {
	return CommandContext{
		Ctx:       context.Background(),
		ChannelID: "chan:42",
		SenderID:  "user:7",
		Args:      args,
		Config:    &ag.config,
		Reply:     cr.reply,
		Registry:  ag.commands,
	}
}

// TestCmdCd_ValidPath_SetsOverride verifies that /cd with a valid directory
// sets the per-(channel,sender) override and replies with confirmation.
func TestCmdCd_ValidPath_SetsOverride(t *testing.T) {
	dir := t.TempDir()
	ag := makeTestAgent(t)
	cr := &capturedReply{}

	if err := ag.cmdCd(makeCdCC(ag, cr, dir)); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	got, ok := ag.shellCwd.Get(key)
	if !ok {
		t.Fatal("expected override to be set")
	}
	// Resolve the TempDir path to compare (it might contain symlinks on macOS).
	wantResolved, _ := filepath.EvalSymlinks(dir)
	if got != wantResolved {
		t.Errorf("expected override %q, got %q", wantResolved, got)
	}
}

// TestCmdCd_NoArg_ReportsCurrentCwd verifies that /cd with no arg replies with
// the current working directory (no state change).
func TestCmdCd_NoArg_ReportsCurrentCwd(t *testing.T) {
	ag := makeTestAgent(t)
	cr := &capturedReply{}

	if err := ag.cmdCd(makeCdCC(ag, cr, "")); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.Contains(cr.messages[0], "Working directory:") {
		t.Errorf("expected 'Working directory:' in reply, got: %q", cr.messages[0])
	}

	// Override map must remain empty.
	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	_, ok := ag.shellCwd.Get(key)
	if ok {
		t.Error("expected no override after /cd with no arg")
	}
}

// TestCmdCd_TildeArg_ClearsOverride verifies that /cd ~ resets any existing
// override and replies with the default working directory.
func TestCmdCd_TildeArg_ClearsOverride(t *testing.T) {
	dir := t.TempDir()
	ag := makeTestAgent(t)

	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	// Pre-set an override.
	_ = ag.shellCwd.Set(key, dir)

	cr := &capturedReply{}
	if err := ag.cmdCd(makeCdCC(ag, cr, "~")); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	// Override should be cleared.
	_, ok := ag.shellCwd.Get(key)
	if ok {
		t.Error("expected override to be cleared after /cd ~")
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.Contains(cr.messages[0], "Working directory:") {
		t.Errorf("expected 'Working directory:' reply, got: %q", cr.messages[0])
	}
}

// TestCmdCd_NonexistentPath_ErrorReply_NoMutation verifies that /cd to a
// non-existent path replies with an error and doesn't mutate the override map.
func TestCmdCd_NonexistentPath_ErrorReply_NoMutation(t *testing.T) {
	ag := makeTestAgent(t)
	cr := &capturedReply{}

	if err := ag.cmdCd(makeCdCC(ag, cr, "/this/path/does/not/exist/12345")); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.HasPrefix(cr.messages[0], "cd:") {
		t.Errorf("expected 'cd:' error prefix, got: %q", cr.messages[0])
	}

	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	_, ok := ag.shellCwd.Get(key)
	if ok {
		t.Error("expected no override after error reply")
	}
}

// TestCmdCd_OutsideSandbox_ErrorReply_NoMutation verifies that /cd to a path
// outside the configured sandbox root is rejected.
func TestCmdCd_OutsideSandbox_ErrorReply_NoMutation(t *testing.T) {
	sandbox := t.TempDir()
	outside := t.TempDir() // a different TempDir — outside the sandbox

	ag := makeTestAgent(t)
	ag.shellCwd.WithSandboxRoot(sandbox)
	cr := &capturedReply{}

	if err := ag.cmdCd(makeCdCC(ag, cr, outside)); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.Contains(cr.messages[0], "outside sandbox") {
		t.Errorf("expected 'outside sandbox' error, got: %q", cr.messages[0])
	}

	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	_, ok := ag.shellCwd.Get(key)
	if ok {
		t.Error("expected no override after sandbox rejection")
	}
}

// TestCmdCd_DotDotTraversal_Rejected verifies that /cd with ".." path components
// is rejected with an error reply (REQ-23).
func TestCmdCd_DotDotTraversal_Rejected(t *testing.T) {
	ag := makeTestAgent(t)
	cr := &capturedReply{}

	// Attempt a raw ".." traversal.
	if err := ag.cmdCd(makeCdCC(ag, cr, "../../etc")); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.Contains(cr.messages[0], "..") {
		t.Errorf("expected '..' in error reply, got: %q", cr.messages[0])
	}
}

// TestCmdCd_AbsoluteDotDotPath_RejectedBySandbox verifies the edge case where
// an absolute path with embedded ".." cleans to a real path outside the sandbox
// (filepath.Clean removes the "..", so the cleaned form contains no ".." literal).
// REQ-23 must still reject it via the sandbox HasPrefix guard.
func TestCmdCd_AbsoluteDotDotPath_RejectedBySandbox(t *testing.T) {
	sandbox := t.TempDir()
	outside := t.TempDir() // real, existing dir outside the sandbox

	ag := makeTestAgent(t)
	ag.shellCwd.WithSandboxRoot(sandbox)
	cr := &capturedReply{}

	// Build an absolute traversal path: <sandbox>/../<basename of outside>
	// After filepath.Clean this becomes the parent of sandbox + basename(outside),
	// which is NOT necessarily `outside` itself — so we craft a path that DOES
	// resolve to the outside dir via traversal through the sandbox's parent.
	// Concrete: sandbox/.. → parent dir; sandbox/../<outside-basename> → resolves
	// to a sibling of sandbox iff that sibling exists. We use `outside` directly
	// joined with /.. /.. to reach a known-real location and verify rejection.
	traversal := sandbox + "/../" + filepath.Base(outside)

	if err := ag.cmdCd(makeCdCC(ag, cr, traversal)); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.HasPrefix(cr.messages[0], "cd:") {
		t.Errorf("expected 'cd:' error prefix, got: %q", cr.messages[0])
	}

	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	if _, ok := ag.shellCwd.Get(key); ok {
		t.Error("expected no override after absolute-dotdot rejection")
	}
}

// TestCmdCd_DoesNotCallLLM verifies that /cd never triggers an LLM call.
func TestCmdCd_DoesNotCallLLM(t *testing.T) {
	prov := &mockProvider{}
	ch := &mockChannel{}
	ag := New(defaultCfg(), defaultLimits(), config.FilterConfig{}, ch, prov, &mockStore{}, audit.NoopAuditor{}, nil, nil, skill.SkillIndex{}, 4, false)

	ag.processMessage(context.Background(), makeIncomingMsg("chan:1", "user:1", "/cd"))
	if prov.callCount() != 0 {
		t.Errorf("expected 0 LLM calls for /cd, got %d", prov.callCount())
	}
}

// TestCmdCd_ReplyFormat_Set verifies the reply format when setting a path
// (Decision #7): "Changed working directory to: <path>".
func TestCmdCd_ReplyFormat_Set(t *testing.T) {
	dir := t.TempDir()
	ag := makeTestAgent(t)
	cr := &capturedReply{}

	if err := ag.cmdCd(makeCdCC(ag, cr, dir)); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.HasPrefix(cr.messages[0], "Changed working directory to: ") {
		t.Errorf("expected 'Changed working directory to: ' prefix, got: %q", cr.messages[0])
	}
}

// TestCmdCd_ReplyFormat_Clear verifies the reply format when clearing (no-arg)
// (Decision #7): "Working directory: <path>".
func TestCmdCd_ReplyFormat_Clear(t *testing.T) {
	ag := makeTestAgent(t)
	cr := &capturedReply{}

	if err := ag.cmdCd(makeCdCC(ag, cr, "")); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.HasPrefix(cr.messages[0], "Working directory: ") {
		t.Errorf("expected 'Working directory: ' prefix, got: %q", cr.messages[0])
	}
}

// ---------------------------------------------------------------------------
// Helper: build an IncomingMessage for dispatch tests
// ---------------------------------------------------------------------------

func makeIncomingMsg(channelID, senderID, text string) channel.IncomingMessage {
	return channel.IncomingMessage{
		ChannelID: channelID,
		SenderID:  senderID,
		Content:   content.TextBlock(text),
	}
}

// TestCmdCd_SetThenNoArg_ClearShowsDefault verifies that after setting an
// override, /cd with no arg clears it and shows the default.
func TestCmdCd_SetThenNoArg_ClearShowsDefault(t *testing.T) {
	dir := t.TempDir()
	ag := makeTestAgent(t)
	ag.shellCwd.WithDefaultCwd("/configured/default")

	// Set first.
	cr1 := &capturedReply{}
	if err := ag.cmdCd(makeCdCC(ag, cr1, dir)); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Clear with no arg.
	cr2 := &capturedReply{}
	if err := ag.cmdCd(makeCdCC(ag, cr2, "")); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if len(cr2.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr2.messages))
	}
	if !strings.Contains(cr2.messages[0], "/configured/default") {
		t.Errorf("expected configured default in reply, got: %q", cr2.messages[0])
	}

	// Override should be gone.
	key := cancelKey{ChannelID: "chan:42", SenderID: "user:7"}
	_, ok := ag.shellCwd.Get(key)
	if ok {
		t.Error("expected override to be cleared")
	}
}

// TestCmdCd_FileRejected verifies that /cd to an existing file (not a dir) is
// rejected.
func TestCmdCd_FileRejected(t *testing.T) {
	dir := t.TempDir()
	f, err := os.CreateTemp(dir, "test*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	f.Close()

	ag := makeTestAgent(t)
	cr := &capturedReply{}

	if err := ag.cmdCd(makeCdCC(ag, cr, f.Name())); err != nil {
		t.Fatalf("cmdCd: %v", err)
	}

	if len(cr.messages) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(cr.messages))
	}
	if !strings.Contains(cr.messages[0], "not a directory") {
		t.Errorf("expected 'not a directory' error, got: %q", cr.messages[0])
	}
}

// Ensure channel package is imported (used by makeIncomingMsg).
var _ = channel.IncomingMessage{}
