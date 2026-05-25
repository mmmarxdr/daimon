package tui

// overlay_palette_test.go — strict TDD tests for PR3a: slash command palette.
//
// Tests are written FIRST (RED), then the implementation is added to make them GREEN.
// All tests are white-box (package tui) so they can access unexported types.
//
// Test groups:
//   1. commandPalette HandleMsg — filtering, navigation, dispatch, confirm, esc
//   2. handleChatKey "/" trigger — palette pushed / not pushed
//   3. model.Update — popOverlayMsg, dispatchCommandMsg, commandResultMsg
//   4. View — overlay composited when palette is active

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"daimon/internal/agent"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// testCmds is the static command list used in palette tests.
// Two commands: one normal, one destructive.
var testCmds = []agent.CommandInfo{
	{Name: "ping", Description: "pings the agent", Source: "builtin", Destructive: false},
	{Name: "reset", Description: "reset conversation state", Source: "builtin", Destructive: true},
	{Name: "help", Description: "show help text", Source: "builtin", Destructive: false},
}

// execCmd executes a tea.Cmd and returns the emitted tea.Msg (or nil).
func execCmd(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	return cmd()
}

// keyRunes builds a tea.KeyMsg carrying the given character string.
func keyRunes(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// keySpecial builds a tea.KeyMsg with the given tea.KeyType.
func keySpecial(t tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: t}
}

// ---------------------------------------------------------------------------
// Group 1: commandPalette HandleMsg
// ---------------------------------------------------------------------------

func TestCommandPalette_ID(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	if got := p.ID(); got != "command-palette" {
		t.Errorf("ID() = %q, want %q", got, "command-palette")
	}
}

func TestCommandPalette_InitialState(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	if len(p.filtered) != len(testCmds) {
		t.Errorf("initial filtered len = %d, want %d (all commands)", len(p.filtered), len(testCmds))
	}
	if p.selIdx != 0 {
		t.Errorf("initial selIdx = %d, want 0", p.selIdx)
	}
}

func TestCommandPalette_TypingRunes_FiltersDown(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Type "pi" — should match only "ping"
	next, _, consumed := p.HandleMsg(keyRunes("p"))
	p = next.(commandPalette)
	if !consumed {
		t.Error("typing rune should be consumed")
	}
	next, _, _ = p.HandleMsg(keyRunes("i"))
	p = next.(commandPalette)
	if len(p.filtered) != 1 {
		t.Errorf("after typing 'pi': filtered len = %d, want 1 (only 'ping')", len(p.filtered))
	}
	if p.filtered[0].Name != "ping" {
		t.Errorf("filtered[0].Name = %q, want 'ping'", p.filtered[0].Name)
	}
}

func TestCommandPalette_TypingRunes_MatchesDescription(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Type "help text" — description-only match
	for _, r := range "help text" {
		next, _, _ := p.HandleMsg(keyRunes(string(r)))
		p = next.(commandPalette)
	}
	if len(p.filtered) != 1 {
		t.Errorf("after typing 'help text': filtered len = %d, want 1 (description match)", len(p.filtered))
	}
	if p.filtered[0].Name != "help" {
		t.Errorf("filtered[0].Name = %q, want 'help'", p.filtered[0].Name)
	}
}

func TestCommandPalette_Backspace_TrimsQuery(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Type "pi" then backspace
	next, _, _ := p.HandleMsg(keyRunes("p"))
	p = next.(commandPalette)
	next, _, _ = p.HandleMsg(keyRunes("i"))
	p = next.(commandPalette)
	if len(p.filtered) != 1 {
		t.Fatalf("pre-backspace filtered len = %d, want 1", len(p.filtered))
	}
	next, _, consumed := p.HandleMsg(keySpecial(tea.KeyBackspace))
	p = next.(commandPalette)
	if !consumed {
		t.Error("backspace should be consumed")
	}
	// After erasing 'i', query is "p".
	// "ping" matches (name contains 'p'), "help" matches (name contains 'p'),
	// "reset" does NOT match 'p'.
	if len(p.filtered) != 2 {
		t.Errorf("after backspace: filtered len = %d, want 2 (ping and help match 'p')", len(p.filtered))
	}
	if p.query != "p" {
		t.Errorf("query after backspace = %q, want %q", p.query, "p")
	}
}

func TestCommandPalette_Backspace_OnEmptyQuery_DoesNothing(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	next, _, consumed := p.HandleMsg(keySpecial(tea.KeyBackspace))
	p = next.(commandPalette)
	if !consumed {
		t.Error("backspace on empty query should still be consumed (palette modal)")
	}
	if p.query != "" {
		t.Errorf("query after backspace on empty = %q, want empty", p.query)
	}
	// All commands still visible
	if len(p.filtered) != len(testCmds) {
		t.Errorf("after backspace on empty: filtered len = %d, want %d", len(p.filtered), len(testCmds))
	}
}

func TestCommandPalette_Down_MovesSelIdx(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	next, _, consumed := p.HandleMsg(keySpecial(tea.KeyDown))
	p = next.(commandPalette)
	if !consumed {
		t.Error("down arrow should be consumed")
	}
	if p.selIdx != 1 {
		t.Errorf("after down: selIdx = %d, want 1", p.selIdx)
	}
}

func TestCommandPalette_Down_ClampsAtEnd(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Move to last item (len-1 = 2)
	for i := 0; i < 10; i++ {
		next, _, _ := p.HandleMsg(keySpecial(tea.KeyDown))
		p = next.(commandPalette)
	}
	want := len(testCmds) - 1
	if p.selIdx != want {
		t.Errorf("after many downs: selIdx = %d, want %d (clamped)", p.selIdx, want)
	}
}

func TestCommandPalette_Up_MovesSelIdx(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Move down first
	next, _, _ := p.HandleMsg(keySpecial(tea.KeyDown))
	p = next.(commandPalette)
	next, _, consumed := p.HandleMsg(keySpecial(tea.KeyUp))
	p = next.(commandPalette)
	if !consumed {
		t.Error("up arrow should be consumed")
	}
	if p.selIdx != 0 {
		t.Errorf("after up: selIdx = %d, want 0", p.selIdx)
	}
}

func TestCommandPalette_Up_ClampsAtZero(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Up when already at 0
	next, _, _ := p.HandleMsg(keySpecial(tea.KeyUp))
	p = next.(commandPalette)
	if p.selIdx != 0 {
		t.Errorf("after up at 0: selIdx = %d, want 0 (clamped)", p.selIdx)
	}
}

func TestCommandPalette_CtrlN_MovesDown(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	msg := tea.KeyMsg{Type: tea.KeyCtrlN}
	next, _, consumed := p.HandleMsg(msg)
	p = next.(commandPalette)
	if !consumed {
		t.Error("ctrl+n should be consumed")
	}
	if p.selIdx != 1 {
		t.Errorf("after ctrl+n: selIdx = %d, want 1", p.selIdx)
	}
}

func TestCommandPalette_CtrlP_MovesUp(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// First move down
	next, _, _ := p.HandleMsg(keySpecial(tea.KeyDown))
	p = next.(commandPalette)
	msg := tea.KeyMsg{Type: tea.KeyCtrlP}
	next, _, consumed := p.HandleMsg(msg)
	p = next.(commandPalette)
	if !consumed {
		t.Error("ctrl+p should be consumed")
	}
	if p.selIdx != 0 {
		t.Errorf("after ctrl+p: selIdx = %d, want 0", p.selIdx)
	}
}

func TestCommandPalette_Enter_NonDestructive_EmitsDispatch(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Default selection is "ping" (index 0, not destructive)
	next, cmd, consumed := p.HandleMsg(keySpecial(tea.KeyEnter))
	_ = next
	if !consumed {
		t.Error("enter should be consumed")
	}
	if cmd == nil {
		t.Fatal("enter on non-destructive cmd: expected non-nil tea.Cmd")
	}
	msg := execCmd(cmd)
	dispatch, ok := msg.(dispatchCommandMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want dispatchCommandMsg", msg)
	}
	if dispatch.name != "ping" {
		t.Errorf("dispatch.name = %q, want %q", dispatch.name, "ping")
	}
	if dispatch.allowDestructive {
		t.Error("dispatch.allowDestructive = true, want false for non-destructive command")
	}
}

func TestCommandPalette_Enter_Destructive_FirstSetsConfirming(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Move to "reset" (index 1, destructive)
	next, _, _ := p.HandleMsg(keySpecial(tea.KeyDown))
	p = next.(commandPalette)
	if p.filtered[p.selIdx].Name != "reset" {
		t.Fatalf("expected selIdx to point to 'reset', got %q", p.filtered[p.selIdx].Name)
	}

	next, cmd, consumed := p.HandleMsg(keySpecial(tea.KeyEnter))
	p = next.(commandPalette)
	if !consumed {
		t.Error("enter should be consumed")
	}
	if !p.confirmingDestructive {
		t.Error("first enter on destructive cmd should set confirmingDestructive=true")
	}
	if cmd != nil {
		// cmd must return nil or a no-op; definitely NOT a dispatchCommandMsg yet
		msg := execCmd(cmd)
		if _, isDispatch := msg.(dispatchCommandMsg); isDispatch {
			t.Error("first enter on destructive cmd must NOT dispatch yet (need confirmation)")
		}
	}
}

func TestCommandPalette_Enter_Destructive_SecondEnterEmitsDispatch(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Navigate to "reset"
	next, _, _ := p.HandleMsg(keySpecial(tea.KeyDown))
	p = next.(commandPalette)
	// First enter — set confirming
	next, _, _ = p.HandleMsg(keySpecial(tea.KeyEnter))
	p = next.(commandPalette)
	if !p.confirmingDestructive {
		t.Fatal("expected confirmingDestructive=true after first enter")
	}
	// Second enter — dispatch
	next, cmd, consumed := p.HandleMsg(keySpecial(tea.KeyEnter))
	_ = next
	if !consumed {
		t.Error("second enter should be consumed")
	}
	if cmd == nil {
		t.Fatal("second enter on destructive: expected non-nil tea.Cmd")
	}
	msg := execCmd(cmd)
	dispatch, ok := msg.(dispatchCommandMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want dispatchCommandMsg", msg)
	}
	if dispatch.name != "reset" {
		t.Errorf("dispatch.name = %q, want %q", dispatch.name, "reset")
	}
	if !dispatch.allowDestructive {
		t.Error("dispatch.allowDestructive = false, want true for confirmed destructive")
	}
}

func TestCommandPalette_Enter_EmptyFiltered_IsNoOp(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Type a query that matches nothing
	for _, r := range "zzzzzz" {
		next, _, _ := p.HandleMsg(keyRunes(string(r)))
		p = next.(commandPalette)
	}
	if len(p.filtered) != 0 {
		t.Fatalf("expected empty filtered list after 'zzzzzz', got %d", len(p.filtered))
	}
	_, cmd, consumed := p.HandleMsg(keySpecial(tea.KeyEnter))
	if !consumed {
		t.Error("enter on empty list should still be consumed (modal)")
	}
	if cmd != nil {
		msg := execCmd(cmd)
		if _, isDispatch := msg.(dispatchCommandMsg); isDispatch {
			t.Error("enter on empty filtered list must not dispatch")
		}
	}
}

func TestCommandPalette_Esc_EmitsPopOverlay(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	_, cmd, consumed := p.HandleMsg(keySpecial(tea.KeyEsc))
	if !consumed {
		t.Error("esc should be consumed")
	}
	if cmd == nil {
		t.Fatal("esc should return a non-nil cmd emitting popOverlayMsg")
	}
	msg := execCmd(cmd)
	if _, ok := msg.(popOverlayMsg); !ok {
		t.Fatalf("esc cmd returned %T, want popOverlayMsg", msg)
	}
}

func TestCommandPalette_Navigation_ClearsConfirming(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Navigate to reset and start confirming
	next, _, _ := p.HandleMsg(keySpecial(tea.KeyDown))
	p = next.(commandPalette)
	next, _, _ = p.HandleMsg(keySpecial(tea.KeyEnter))
	p = next.(commandPalette)
	if !p.confirmingDestructive {
		t.Fatal("expected confirmingDestructive=true")
	}
	// Moving up should clear confirming
	next, _, _ = p.HandleMsg(keySpecial(tea.KeyUp))
	p = next.(commandPalette)
	if p.confirmingDestructive {
		t.Error("navigation (up) should clear confirmingDestructive")
	}
}

func TestCommandPalette_TypingRunes_ClearsConfirming(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Navigate to reset and start confirming
	next, _, _ := p.HandleMsg(keySpecial(tea.KeyDown))
	p = next.(commandPalette)
	next, _, _ = p.HandleMsg(keySpecial(tea.KeyEnter))
	p = next.(commandPalette)
	if !p.confirmingDestructive {
		t.Fatal("expected confirmingDestructive=true")
	}
	// Typing clears confirming
	next, _, _ = p.HandleMsg(keyRunes("r"))
	p = next.(commandPalette)
	if p.confirmingDestructive {
		t.Error("typing runes should clear confirmingDestructive")
	}
}

func TestCommandPalette_OtherKeys_AreSwallowed(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Random key like F1 should be consumed (modal)
	_, _, consumed := p.HandleMsg(tea.KeyMsg{Type: tea.KeyF1})
	if !consumed {
		t.Error("unrecognized key in palette should be consumed (modal)")
	}
}

func TestCommandPalette_SelIdx_ResetOnNewQuery(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Move down
	next, _, _ := p.HandleMsg(keySpecial(tea.KeyDown))
	p = next.(commandPalette)
	if p.selIdx != 1 {
		t.Fatalf("expected selIdx=1 after down, got %d", p.selIdx)
	}
	// Type a char — selIdx must reset to 0
	next, _, _ = p.HandleMsg(keyRunes("h"))
	p = next.(commandPalette)
	if p.selIdx != 0 {
		t.Errorf("after typing char: selIdx = %d, want 0 (reset)", p.selIdx)
	}
}

// ---------------------------------------------------------------------------
// Group 1b: Render — basic content checks (no golden file; structural only)
// ---------------------------------------------------------------------------

func TestCommandPalette_Render_ContainsTitle(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	s := newTuiStyles()
	rendered := p.Render(80, 24, s)
	if !strings.Contains(rendered, "commands") {
		t.Errorf("Render should contain 'commands' in title, got:\n%s", rendered)
	}
}

func TestCommandPalette_Render_ContainsCommandNames(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	s := newTuiStyles()
	rendered := p.Render(80, 24, s)
	for _, cmd := range testCmds {
		if !strings.Contains(rendered, cmd.Name) {
			t.Errorf("Render: expected command name %q in output\n%s", cmd.Name, rendered)
		}
	}
}

func TestCommandPalette_Render_ConfirmingDestructive_ShowsWarning(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	// Navigate to reset and trigger confirming state
	next, _, _ := p.HandleMsg(keySpecial(tea.KeyDown))
	p = next.(commandPalette)
	next, _, _ = p.HandleMsg(keySpecial(tea.KeyEnter))
	p = next.(commandPalette)
	if !p.confirmingDestructive {
		t.Fatal("expected confirmingDestructive=true")
	}
	s := newTuiStyles()
	rendered := p.Render(80, 24, s)
	if !strings.Contains(rendered, "destructive") {
		t.Errorf("Render in confirming state should contain 'destructive'\n%s", rendered)
	}
}

func TestCommandPalette_Render_NoPanic_NarrowWidth(t *testing.T) {
	p := newCommandPalette(testCmds, newTuiStyles())
	s := newTuiStyles()
	// Should not panic on very narrow widths
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Render panicked on narrow width: %v", r)
		}
	}()
	_ = p.Render(10, 5, s)
}

// ---------------------------------------------------------------------------
// Group 2: handleChatKey "/" trigger
// ---------------------------------------------------------------------------

func TestHandleChatKey_Slash_EmptyInput_PushPalette(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor
	m.input.Reset() // ensure empty

	msg := keyRunes("/")
	next, _ := m.handleChatKey(msg)
	nm := next.(Model)

	if !nm.overlays.Active() {
		t.Error("after '/' with empty input: overlays.Active() = false, want true (palette pushed)")
	}
	if nm.overlays.Top().ID() != "command-palette" {
		t.Errorf("pushed overlay ID = %q, want %q", nm.overlays.Top().ID(), "command-palette")
	}
}

func TestHandleChatKey_Slash_NonEmptyInput_DoesNotPushPalette(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor
	// Set non-empty input by feeding rune messages through the input bar.
	m.input.ti.SetValue("hello")

	msg := keyRunes("/")
	next, _ := m.handleChatKey(msg)
	nm := next.(Model)

	if nm.overlays.Active() {
		t.Error("after '/' with non-empty input: overlays.Active() = true, want false (/ should go to input bar)")
	}
}

// TestHandleChatKey_Slash_NonEmptyInput_ForwardsSlashToInput verifies that
// pressing '/' when the input bar already has content forwards '/' to the input
// (the character becomes part of the typed message).
func TestHandleChatKey_Slash_NonEmptyInput_ForwardsSlashToInput(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor
	m.input.ti.SetValue("hello")

	msg := keyRunes("/")
	next, _ := m.handleChatKey(msg)
	nm := next.(Model)

	// The input value must now contain "/".
	val := nm.input.Value()
	if !strings.Contains(val, "/") {
		t.Errorf("after '/' with non-empty input: input value = %q, want it to contain '/'", val)
	}
}

// ---------------------------------------------------------------------------
// Group 3: model.Update messages
// ---------------------------------------------------------------------------

func TestModel_Update_PopOverlayMsg_PopsOverlay(t *testing.T) {
	m := newTestModel()
	// Manually push an overlay
	m.overlays.Push(newCommandPalette(testCmds, newTuiStyles()))
	if !m.overlays.Active() {
		t.Fatal("overlay should be active before test")
	}

	next, cmd := m.Update(popOverlayMsg{})
	nm := next.(Model)

	if nm.overlays.Active() {
		t.Error("after popOverlayMsg: overlays.Active() = true, want false")
	}
	if cmd != nil {
		t.Errorf("popOverlayMsg should return nil cmd, got %T", cmd)
	}
}

func TestModel_Update_DispatchCommandMsg_PopsOverlay(t *testing.T) {
	m := newTestModel()
	m.overlays.Push(newCommandPalette(testCmds, newTuiStyles()))

	// With ag == nil: overlay is popped and "no agent connected" is appended.
	next, _ := m.Update(dispatchCommandMsg{name: "ping", allowDestructive: false})
	nm := next.(Model)

	if nm.overlays.Active() {
		t.Error("after dispatchCommandMsg: overlay should be popped")
	}
}

// TestModel_Update_DispatchCommandMsg_NilAg_AppendsErrorMessage verifies the
// nil-ag guard: when ag == nil, dispatchCommandMsg appends "no agent connected"
// to the thread (non-vacuous: drives the full dispatch→result path).
func TestModel_Update_DispatchCommandMsg_NilAg_AppendsErrorMessage(t *testing.T) {
	m := newTestModel() // ag is nil by default in newTestModel
	m.overlays.Push(newCommandPalette(testCmds, newTuiStyles()))

	// Execute dispatchCommandMsg — nil-ag guard must fire.
	next, cmd := m.Update(dispatchCommandMsg{name: "ping", allowDestructive: false})
	nm := next.(Model)

	// cmd must be nil (nil-ag guard returns nil, not runCommandCmd).
	if cmd != nil {
		t.Errorf("nil-ag guard: expected nil cmd, got %T", cmd)
	}

	// The thread must contain the "no agent connected" message.
	if len(nm.thread.items) == 0 {
		t.Fatal("nil-ag guard: thread must have an item appended")
	}
	md, ok := nm.thread.items[len(nm.thread.items)-1].(*MsgDaimon)
	if !ok {
		t.Fatalf("nil-ag guard: last thread item is %T, want *MsgDaimon", nm.thread.items[len(nm.thread.items)-1])
	}
	if !strings.Contains(md.text, "no agent connected") {
		t.Errorf("nil-ag guard: MsgDaimon.text = %q, want to contain 'no agent connected'", md.text)
	}
}

func TestModel_Update_CommandResultMsg_AppendsMsgDaimon(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	next, cmd := m.Update(commandResultMsg{reply: "pong", err: nil})
	nm := next.(Model)

	if cmd != nil {
		t.Errorf("commandResultMsg should return nil cmd, got %T", cmd)
	}
	if len(nm.thread.items) != 1 {
		t.Fatalf("after commandResultMsg: thread.items len = %d, want 1", len(nm.thread.items))
	}
	md, ok := nm.thread.items[0].(*MsgDaimon)
	if !ok {
		t.Fatalf("thread.items[0] is %T, want *MsgDaimon", nm.thread.items[0])
	}
	if !strings.Contains(md.text, "pong") {
		t.Errorf("MsgDaimon.text = %q, want to contain 'pong'", md.text)
	}
}

func TestModel_Update_CommandResultMsg_Error_AppendsMsgDaimonWithError(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	testErr := fmt.Errorf("command failed: %w", fmt.Errorf("timeout"))
	next, _ := m.Update(commandResultMsg{reply: "", err: testErr})
	nm := next.(Model)

	if len(nm.thread.items) != 1 {
		t.Fatalf("thread.items len = %d, want 1", len(nm.thread.items))
	}
	md, ok := nm.thread.items[0].(*MsgDaimon)
	if !ok {
		t.Fatalf("thread.items[0] is %T, want *MsgDaimon", nm.thread.items[0])
	}
	// Error text must be present in the thread item
	if !strings.Contains(md.text, "timeout") && !strings.Contains(md.text, "failed") {
		t.Errorf("MsgDaimon.text = %q, want error info present", md.text)
	}
}

// ---------------------------------------------------------------------------
// Group 4: View — overlay composited when palette is active
// ---------------------------------------------------------------------------

func TestModel_View_WithPalette_ContainsCommands(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenChat
	m.focus = focusEditor

	// Push a palette with some commands
	m.overlays.Push(newCommandPalette(testCmds, newTuiStyles()))

	rendered := m.View()
	if !strings.Contains(rendered, "commands") {
		t.Errorf("View() with active palette should contain 'commands' title\nrendered:\n%s", rendered)
	}
}

func TestModel_View_WithoutPalette_DoesNotContainPaletteMarker(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenChat
	m.focus = focusEditor
	// No overlay pushed

	rendered := m.View()
	// "⫶ commands" should NOT appear when no palette is active
	// (the topBar glyph is just "⫶", the title "commands" only appears in the palette)
	if strings.Contains(rendered, "⫶ commands") {
		t.Errorf("View() without active palette should not contain palette title\nrendered:\n%s", rendered)
	}
}
