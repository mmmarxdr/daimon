package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestUpdateWelcome_EnterWithText_TransitionsToChat verifies the welcome→chat
// transition: pressing Enter on the welcome screen with non-empty input must
// switch to the chat screen, append the typed text as a MsgUser, and return a
// submit Cmd (which produces promptSentMsg). This is the navigation that makes
// the chat screen reachable; without it `daimon tui` is stuck on welcome.
func TestUpdateWelcome_EnterWithText_TransitionsToChat(t *testing.T) {
	m := newTestModel() // screen=welcome, focus=focusEditor, inbox=nil (submit is instant)
	m.input.ti.SetValue("Write a reverse function")

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	if rm.screen != screenChat {
		t.Errorf("screen after Enter = %v, want chat", rm.screen)
	}
	if rm.focus != focusEditor {
		t.Errorf("focus after Enter = %v, want focusEditor", rm.focus)
	}
	if rm.footer.screen != screenChat {
		t.Errorf("footer.screen after Enter = %v, want chat", rm.footer.screen)
	}
	if len(rm.thread.items) != 1 {
		t.Fatalf("thread items = %d, want 1 (the submitted MsgUser)", len(rm.thread.items))
	}
	mu, ok := rm.thread.items[0].(*MsgUser)
	if !ok {
		t.Fatalf("first thread item = %T, want *MsgUser", rm.thread.items[0])
	}
	if mu.text != "Write a reverse function" {
		t.Errorf("MsgUser.text = %q, want %q", mu.text, "Write a reverse function")
	}
	if cmd == nil {
		t.Error("expected a submit Cmd, got nil")
	}
}

// TestUpdateWelcome_EnterEmpty_StaysOnWelcome verifies that pressing Enter with
// an empty input does NOT transition (no spurious chat screen, no empty MsgUser).
func TestUpdateWelcome_EnterEmpty_StaysOnWelcome(t *testing.T) {
	m := newTestModel() // empty input by default

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	if rm.screen != screenWelcome {
		t.Errorf("screen = %v, want welcome (empty input must not transition)", rm.screen)
	}
	if len(rm.thread.items) != 0 {
		t.Errorf("thread items = %d, want 0 (empty input must not append)", len(rm.thread.items))
	}
}
