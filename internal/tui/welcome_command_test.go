package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// welcome_command_test.go — TDD tests for the fix: slash commands run from the
// welcome screen must transition to screenChat and show their output.
//
// RED: tests written BEFORE the fix; they must FAIL with the current code.
// GREEN: tests must pass after the commandResultMsg handler is patched to
// call enterChatViewport() when m.screen != screenChat.

// TestCommandResult_FromWelcome_TransitionsToChat is the primary regression
// test. It verifies that receiving commandResultMsg while on the welcome
// screen transitions to screenChat and makes the reply visible in the viewport.
//
// Before the fix:
//   - m.screen stays screenWelcome (FAIL)
//   - m.viewport.View() is empty / doesn't contain the reply (FAIL)
func TestCommandResult_FromWelcome_TransitionsToChat(t *testing.T) {
	m := newTestModel()
	// Ensure we start on the welcome screen.
	if m.screen != screenWelcome {
		t.Fatalf("precondition: screen = %v, want screenWelcome", m.screen)
	}

	// Size the model so the viewport can actually hold content.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Simulate receiving the result of a /status command from the welcome screen.
	reply := "Agent Status:\n  Provider: x"
	upd, _ = m.Update(commandResultMsg{name: "status", reply: reply})
	m = upd.(Model)

	// (a) Screen must have transitioned to screenChat.
	if m.screen != screenChat {
		t.Errorf("screen = %v, want screenChat after commandResultMsg from welcome", m.screen)
	}

	// (b) The reply must be visible in the viewport.
	view := m.viewport.View()
	if !strings.Contains(view, "Agent Status") {
		t.Errorf("viewport.View() = %q; want it to contain %q", view, "Agent Status")
	}
}

// TestCommandResult_FromChat_StaysAndShows is the no-regression test.
// When the model is already on screenChat, commandResultMsg must NOT reset
// the viewport (keep scroll position) and the reply must appear.
func TestCommandResult_FromChat_StaysAndShows(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor
	m.footer = footerHints{screen: screenChat}

	// Size the viewport.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Deliver a commandResultMsg while already in chat.
	reply := "hello from command"
	upd, _ = m.Update(commandResultMsg{name: "status", reply: reply})
	m = upd.(Model)

	// Screen must stay chat.
	if m.screen != screenChat {
		t.Errorf("screen = %v, want screenChat (must not change when already in chat)", m.screen)
	}

	// Reply must be visible in the viewport.
	view := m.viewport.View()
	if !strings.Contains(view, "hello from command") {
		t.Errorf("viewport.View() = %q; want it to contain %q", view, "hello from command")
	}
}

// TestDispatchCommand_NilAgent_FromWelcome_TransitionsToChat verifies that
// the nil-agent branch of dispatchCommandMsg also transitions to screenChat
// when the model is on the welcome screen, making "no agent connected" visible.
//
// This mirrors the same pattern as the commandResultMsg fix for consistency.
func TestDispatchCommand_NilAgent_FromWelcome_TransitionsToChat(t *testing.T) {
	m := newTestModel()
	// Verify nil agent (default in newTestModel).
	if m.ag != nil {
		t.Skip("newTestModel has a non-nil agent; test requires nil agent")
	}
	if m.screen != screenWelcome {
		t.Fatalf("precondition: screen = %v, want screenWelcome", m.screen)
	}

	// Size the model.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Send a dispatchCommandMsg — ag is nil so it takes the nil-agent branch.
	upd, _ = m.Update(dispatchCommandMsg{name: "status"})
	m = upd.(Model)

	// Screen must have transitioned to screenChat.
	if m.screen != screenChat {
		t.Errorf("screen = %v, want screenChat after dispatchCommandMsg (nil agent) from welcome", m.screen)
	}

	// "no agent connected" must be visible in the viewport.
	view := m.viewport.View()
	if !strings.Contains(view, "no agent connected") {
		t.Errorf("viewport.View() = %q; want it to contain %q", view, "no agent connected")
	}
}
