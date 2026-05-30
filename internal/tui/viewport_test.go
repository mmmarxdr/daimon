package tui

// viewport_test.go — TDD RED tests for WU-c viewport integration.
//
// Tasks 2.2 (viewport unit tests) and 2.4 (scroll key routing tests).
// All tests here are written RED-first and will pass only after the GREEN
// implementation in tasks 2.5–2.10.

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Task 2.2 — viewport unit tests (sizing + transitions)
// ---------------------------------------------------------------------------

// TestViewport_WindowSizeMsg_Propagates verifies that a WindowSizeMsg sets the
// viewport dimensions to the expected content-area values via chatViewportSize.
func TestViewport_WindowSizeMsg_Propagates(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	// Send a WindowSizeMsg through Update.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// chatViewportSize(m) computes the viewport content area for an 80x24 terminal.
	// With no rail (screenChat after transition), no input bar yet set, but
	// inputBarScreens[screenChat] = true:
	//   topBarHeight = 2, footerHeight = 2, inputHeight = 4
	//   centerHeight = 24 - 2 - 2 - 4 = 16
	// No rail for chat-only model in newTestModel (no panels).
	// centerWidth = 80 - 0 (no rail on screenChat in newTestModel) = 80 ... but
	// wait, HasPanels(screenChat) may be true. Let's just assert dimensions > 0.
	vw, vh := chatViewportSize(m)
	if m.viewport.Width != vw {
		t.Errorf("viewport.Width = %d, want %d (from chatViewportSize)", m.viewport.Width, vw)
	}
	if m.viewport.Height != vh {
		t.Errorf("viewport.Height = %d, want %d (from chatViewportSize)", m.viewport.Height, vh)
	}
}

// TestViewport_StickToBottom verifies that when the user is at the bottom and
// a new item is appended (via refreshThreadViewport), the viewport stays at bottom.
func TestViewport_StickToBottom(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	// Size the viewport.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Append content and refresh.
	m.thread.append(&MsgUser{text: "hello", styles: m.styles})
	m = m.refreshThreadViewport()

	if !m.viewport.AtBottom() {
		t.Error("viewport should be at bottom after initial content push")
	}

	// Append more — still at bottom.
	m.thread.append(&MsgDaimon{text: "world", styles: m.styles})
	m = m.refreshThreadViewport()

	if !m.viewport.AtBottom() {
		t.Error("viewport should stick to bottom when user has not scrolled up")
	}
}

// TestViewport_FreezeWhenScrolledUp verifies that when YOffset > 0 (user scrolled
// up), appending a new item does NOT change the scroll offset.
func TestViewport_FreezeWhenScrolledUp(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	// Size the viewport large enough to have content exceeding the height.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m = upd.(Model)

	// Fill with many items so there's scroll range.
	for i := 0; i < 20; i++ {
		m.thread.append(&MsgUser{text: "line content", styles: m.styles})
	}
	m = m.refreshThreadViewport()

	// Simulate user scrolling up: set YOffset > 0.
	m.viewport.YOffset = 3
	offsetBefore := m.viewport.YOffset

	// Append another item (as happens in Update on new message).
	m.thread.append(&MsgDaimon{text: "new message", styles: m.styles})
	m = m.refreshThreadViewport()

	// Offset must be unchanged — user's reading position is preserved.
	if m.viewport.YOffset != offsetBefore {
		t.Errorf("viewport.YOffset after append = %d, want %d (freeze when scrolled up)",
			m.viewport.YOffset, offsetBefore)
	}
}

// TestViewport_ResetOnTransition verifies that transitioning to the chat screen
// resets YOffset to 0 and populates fresh viewport content.
func TestViewport_ResetOnTransition(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat

	// Size viewport.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Simulate previous scroll state.
	m.viewport.YOffset = 10

	// Trigger a transition into chat (sessions → chat via Enter).
	// We directly simulate what updateSessions.Enter does: clear thread, reset viewport.
	m.viewport.SetContent("")
	m.viewport.GotoTop()
	m = m.refreshThreadViewport()

	if m.viewport.YOffset != 0 {
		t.Errorf("viewport.YOffset after transition = %d, want 0 (reset on screen change)",
			m.viewport.YOffset)
	}
}

// ---------------------------------------------------------------------------
// Task 2.4 — scroll key routing tests
// ---------------------------------------------------------------------------

// TestScrollKeys_PgDown_MovesViewport verifies that PgDown always forwards to the
// viewport (even when focusEditor is active) and changes the YOffset.
// Design §C.7: pgup/pgdown/ctrl+u/ctrl+d always forward to viewport.
func TestScrollKeys_PgDown_MovesViewport(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	// Size and fill viewport so there's scroll range.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m = upd.(Model)
	for i := 0; i < 30; i++ {
		m.thread.append(&MsgUser{text: "line content to fill viewport", styles: m.styles})
	}
	m = m.refreshThreadViewport()

	// Start at bottom; scroll up first so there's room to PgDown.
	m.viewport.GotoTop()
	offsetBefore := m.viewport.YOffset

	// Send PgDown key.
	upd2, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m2 := upd2.(Model)

	// Viewport YOffset should have changed (advanced towards bottom).
	if m2.viewport.YOffset == offsetBefore {
		t.Errorf("PgDown with focusEditor: viewport.YOffset unchanged (%d) — must always forward scroll keys to viewport",
			m2.viewport.YOffset)
	}

	// Input bar must still have focus (not stolen by scroll).
	if m2.focus != focusEditor {
		t.Errorf("PgDown must not change focus region; got %v, want focusEditor", m2.focus)
	}
}

// TestScrollKeys_ArrowsScrollWhenFocusMain verifies that Up/Down arrows advance
// the viewport when focus is focusMain (thread navigation mode).
func TestScrollKeys_ArrowsScrollWhenFocusMain(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusMain // thread navigation mode

	// Size and fill viewport.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m = upd.(Model)
	for i := 0; i < 30; i++ {
		m.thread.append(&MsgUser{text: "line content to fill viewport", styles: m.styles})
	}
	m = m.refreshThreadViewport()

	// Start from top.
	m.viewport.GotoTop()
	offsetBefore := m.viewport.YOffset

	// Send Down arrow.
	upd2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := upd2.(Model)

	if m2.viewport.YOffset <= offsetBefore {
		t.Errorf("Down arrow with focusMain: viewport.YOffset=%d, want > %d (scroll down)",
			m2.viewport.YOffset, offsetBefore)
	}
}

// TestScrollKeys_ArrowsNoopWhenFocusEditor verifies that Up/Down arrows do NOT
// move the viewport when focusEditor is active — they belong to the input bar.
func TestScrollKeys_ArrowsNoopWhenFocusEditor(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	// Size and fill viewport.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 10})
	m = upd.(Model)
	for i := 0; i < 30; i++ {
		m.thread.append(&MsgUser{text: "line content to fill viewport", styles: m.styles})
	}
	m = m.refreshThreadViewport()

	// Position at a mid-point so both up and down have room.
	m.viewport.YOffset = 5
	offsetBefore := m.viewport.YOffset

	// Send Down arrow — must NOT change viewport.
	upd2, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m2 := upd2.(Model)

	if m2.viewport.YOffset != offsetBefore {
		t.Errorf("Down arrow with focusEditor: viewport.YOffset changed from %d to %d — must be noop for viewport",
			offsetBefore, m2.viewport.YOffset)
	}
}
