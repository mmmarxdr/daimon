package tui

// judgment_day_fixes_test.go — TDD RED-first tests for Round 1 judgment-day fixes.
//
// FIX 1: dispatchCommandMsg nil-agent missing viewport refresh
// FIX 2: screen_sessions.go resume loses thread.styles
// FIX 3: viewport size stale on screen transition into chat
//
// Written RED-first: each test FAILS before the fix is applied.

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// FIX 1 — dispatchCommandMsg nil-agent: missing viewport refresh
//
// When m.ag == nil and dispatchCommandMsg is received, a "no agent connected"
// message is appended to the thread but m.viewport is NOT refreshed.
// The viewport.View() must contain the message text after the handler returns.
// ---------------------------------------------------------------------------

// TestDispatchCommandMsg_NilAgent_ViewportRefreshed verifies that after
// receiving dispatchCommandMsg with m.ag == nil:
//   - "no agent connected" is visible in m.viewport.View()
//   - The viewport is not empty
//
// Before the fix: m = m.refreshThreadViewport() is missing → viewport shows
// stale content (empty) even though the thread has the new item.
func TestDispatchCommandMsg_NilAgent_ViewportRefreshed(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor
	// ag is nil by default in newTestModel.

	// Size the viewport so content can be pushed in.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Send dispatchCommandMsg with a nil agent path.
	upd2, _ := m.Update(dispatchCommandMsg{name: "test", allowDestructive: false})
	rm := upd2.(Model)

	// The viewport content must contain "no agent connected".
	viewContent := rm.viewport.View()
	if !strings.Contains(viewContent, "no agent connected") {
		t.Errorf("viewport.View() after nil-agent dispatchCommandMsg = %q; want 'no agent connected' to be visible",
			viewContent)
	}
}

// ---------------------------------------------------------------------------
// FIX 2 — screen_sessions.go resume loses thread.styles
//
// In updateSessions "enter" branch: `m.thread = thread{}` discards the
// styles field. After resume, m.thread.styles must equal m.styles.
// ---------------------------------------------------------------------------

// TestSessionResume_ThreadStylesPreserved verifies that after enter-resume,
// m.thread.styles is not a zero-value tuiStyles (i.e. styles are preserved).
//
// Before the fix: m.thread = thread{} loses styles → m.thread.styles is a
// zero-value tuiStyles{} where all lipgloss.Style fields have no foreground.
// We detect this by checking m.thread.styles.accent.GetForeground() against
// the zero-value (no color set) and the expected color from m.styles.accent.
func TestSessionResume_ThreadStylesPreserved(t *testing.T) {
	convs := fakeConvs()
	m := sessionModel(convs)
	m.sessionIdx = 0

	// Simulate a prior session's thread content.
	m.thread.append(&MsgUser{text: "prior message", styles: m.styles})

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	// m.styles is initialized via newTuiStyles() in sessionModel→newTestModel,
	// so m.styles.accent has a foreground color (colorAccent = "#5dbfa7").
	// A zero-value tuiStyles has no foreground set (GetForeground() returns {}).
	gotFG := rm.thread.styles.accent.GetForeground()
	wantFG := rm.styles.accent.GetForeground()
	zeroFG := tuiStyles{}.accent.GetForeground()

	// If the fix is missing: gotFG == zeroFG and wantFG != zeroFG.
	if gotFG == zeroFG && wantFG != zeroFG {
		t.Errorf("thread.styles.accent.GetForeground() = %v (zero-value); want %v (m.styles.accent fg). thread.styles was not preserved on session resume.", gotFG, wantFG)
	}
}

// ---------------------------------------------------------------------------
// FIX 3 — viewport size stale on screen transition into chat
//
// When user is on screenTools (or screenSessions) and a resize occurs,
// the viewport is sized for THAT screen's geometry. On Esc back to chat,
// the viewport retains the wrong height (h-4 instead of h-8 for chat),
// causing renderChat to overflow the center slot.
//
// The fix: add enterChatViewport() helper that recomputes size + resets content.
// Call it on chat-entry transitions.
// ---------------------------------------------------------------------------

// TestViewportSize_ToolsToChat_RecomputedOnTransition verifies that when
// returning from screenTools to screenChat via Esc, the viewport is resized
// to the chat geometry (inputHeight=4, so vh = h - 2(top) - 2(footer) - 4(input)).
//
// Before the fix: viewport retains the tools-screen sizing (inputHeight=0)
// so viewport.Height = h-4 instead of chat's h-8.
func TestViewportSize_ToolsToChat_RecomputedOnTransition(t *testing.T) {
	m := newTestModel()
	m.screen = screenTools
	m.prevScreen = screenChat
	m.focus = focusEditor

	// Size the viewport while on screenTools (inputHeight=0 for tools).
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Transition: Esc from tools → prevScreen=chat.
	upd2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := upd2.(Model)

	// After transition to chat, compute what the viewport height SHOULD be.
	// chatViewportSize uses m.screen (now screenChat) for geometry.
	_, wantVH := chatViewportSize(rm)

	if rm.viewport.Height != wantVH {
		t.Errorf("viewport.Height after tools→chat = %d, want %d (chat geometry with inputHeight=4)",
			rm.viewport.Height, wantVH)
	}
}

// TestViewportSize_SessionsToChat_RecomputedOnTransition verifies that when
// resuming a session (enter in sessions → screenChat), the viewport is resized
// to the chat geometry.
//
// Before the fix: sessions Enter only calls SetContent + GotoTop + refreshThreadViewport
// but does NOT recompute the viewport Width/Height — so if the last resize
// occurred on sessions screen, the viewport has the sessions geometry.
func TestViewportSize_SessionsToChat_RecomputedOnTransition(t *testing.T) {
	convs := fakeConvs()
	m := sessionModel(convs)
	m.sessionIdx = 0

	// Size the viewport while on screenSessions (no inputBar on sessions).
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Resume: Enter in sessions → screenChat.
	upd2, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyEnter})
	rm := upd2.(Model)

	// After transition to chat, compute what the viewport height SHOULD be.
	_, wantVH := chatViewportSize(rm)

	if rm.viewport.Height != wantVH {
		t.Errorf("viewport.Height after sessions→chat = %d, want %d (chat geometry with inputHeight=4)",
			rm.viewport.Height, wantVH)
	}
}

// TestViewportOverflow_ToolsToChat_ViewLineCount verifies that after tools→chat
// transition, the rendered View() line count does not exceed m.height.
// This is the overflow symptom: input bar + footer get pushed off screen.
func TestViewportOverflow_ToolsToChat_ViewLineCount(t *testing.T) {
	m := newTestModel()
	m.screen = screenTools
	m.prevScreen = screenChat
	m.focus = focusEditor

	// Size the model while on screenTools.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Transition to chat via Esc.
	upd2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := upd2.(Model)

	// Count lines in the rendered view.
	view := rm.View()
	lines := strings.Split(view, "\n")
	if len(lines) > rm.height {
		t.Errorf("View() after tools→chat has %d lines, want <= %d (m.height) — viewport overflow",
			len(lines), rm.height)
	}
}

// ---------------------------------------------------------------------------
// Round 2 Fix 1 — sessions Esc → chat does not re-size the viewport
//
// Sessions has no input bar; chat reserves 4 input rows. When the user is on
// sessions and presses Esc back to chat (prevScreen==screenChat), the viewport
// keeps the sessions geometry (height h-4) instead of recomputing to the chat
// geometry (height h-8). This is the same class as the tools-Esc fix from
// Round 1: updateTools Esc already does the guarded enterChatViewport() call;
// updateSessions Esc must do the same.
// ---------------------------------------------------------------------------

// TestViewportSize_SessionsEsc_ToChat_RecomputedOnTransition verifies that
// when the user presses Esc from screenSessions back to screenChat, the
// viewport is resized to the correct chat geometry (inputHeight=4, vh=h-8).
//
// Sequence:
//  1. Build model on screenSessions with prevScreen=screenChat.
//  2. Drive WindowSizeMsg{80,24} → viewport is sized for sessions (h-4=20).
//  3. Press Esc → should land on screenChat with viewport sized for chat (h-8=16).
//
// Before the fix: the Esc branch does not call enterChatViewport(), so
// viewport.Height remains 20 (sessions geometry) instead of becoming 16
// (chat geometry). The test MUST fail before the fix is applied (RED).
func TestViewportSize_SessionsEsc_ToChat_RecomputedOnTransition(t *testing.T) {
	m := newTestModel()
	m.screen = screenSessions
	m.prevScreen = screenChat

	// Size the viewport while on screenSessions (no input bar → inputHeight=0).
	// After WindowSizeMsg{80,24}: sessions geometry → vh = 24-2-2-0 = 20.
	upd, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = upd.(Model)

	// Sanity: sessions geometry gives vh=20 (no input bar).
	if m.viewport.Height != 20 {
		t.Logf("pre-Esc viewport.Height=%d (expected 20 for sessions geometry)", m.viewport.Height)
	}

	// Esc → prevScreen = screenChat.
	upd2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	rm := upd2.(Model)

	// Chat geometry: inputHeight=4, vh = 24-2-2-4 = 16.
	_, wantVH := chatViewportSize(rm)

	if rm.viewport.Height != wantVH {
		t.Errorf("viewport.Height after sessions Esc→chat = %d, want %d (chat geometry with inputHeight=4)",
			rm.viewport.Height, wantVH)
	}

	// Bonus: View() line count must not exceed m.height.
	view := rm.View()
	lines := strings.Split(view, "\n")
	if len(lines) > rm.height {
		t.Errorf("View() after sessions Esc→chat has %d lines, want <= %d (m.height) — viewport overflow",
			len(lines), rm.height)
	}
}
