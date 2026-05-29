package tui

// screen_sessions_test.go — STRICT TDD tests for PR3b: sessions screen.
//
// Test order follows the spec:
//   1. updateSessions key handling (up/down/enter/esc)
//   2. sessionsLoadedMsg populates model.sessions and clamps sessionIdx
//   3. tab in chat (handleChatKey) → screenSessions + non-nil cmd
//   4. tab in welcome (updateWelcome) → screenSessions + non-nil cmd
//   5. submit(text, convID) carries ConversationID
//   6. modelPickerPanel.Render
//   7. renderSessions
//   8. footer.screen is updated on every screen transition (Fix 1)
//   9. modelPickerPanel with real provider/model values (Fix 2)
//  10. ANSI-safe truncation for CompactedSummary (Fix 3)
//  11. sessionsLoadedMsg.err surfaced in renderSessions (Fix 4)
//  12. enter-resume clears thread and sets marker item (Fix 5)

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"daimon/internal/channel"
	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func fakeConvs() []store.Conversation {
	now := time.Now()
	return []store.Conversation{
		{
			ID:        "conv-abc12345",
			ChannelID: "tui",
			Status:    "active",
			UpdatedAt: now,
			Metadata:  map[string]string{"title": "Alpha session"},
		},
		{
			ID:        "conv-def67890",
			ChannelID: "tui",
			Status:    "completed",
			UpdatedAt: now.Add(-10 * time.Minute),
			Metadata:  map[string]string{"title": "Beta session"},
		},
		{
			ID:        "conv-ghi11111",
			ChannelID: "tui",
			Status:    "active",
			UpdatedAt: now.Add(-30 * time.Minute),
		},
	}
}

func sessionModel(convs []store.Conversation) Model {
	m := newTestModel()
	m.screen = screenSessions
	m.sessions = convs
	m.sessionIdx = 0
	m.prevScreen = screenWelcome
	return m
}

// ---------------------------------------------------------------------------
// 1. updateSessions — key routing
// ---------------------------------------------------------------------------

func TestUpdateSessions_Down_IncreasesIndex(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.sessionIdx = 0

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	rm := next.(Model)

	if rm.sessionIdx != 1 {
		t.Errorf("sessionIdx after 'j' = %d, want 1", rm.sessionIdx)
	}
}

func TestUpdateSessions_Down_Arrow_IncreasesIndex(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.sessionIdx = 0

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyDown})
	rm := next.(Model)

	if rm.sessionIdx != 1 {
		t.Errorf("sessionIdx after KeyDown = %d, want 1", rm.sessionIdx)
	}
}

func TestUpdateSessions_Down_ClampsAtEnd(t *testing.T) {
	convs := fakeConvs()
	m := sessionModel(convs)
	m.sessionIdx = len(convs) - 1 // already at last

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	rm := next.(Model)

	if rm.sessionIdx != len(convs)-1 {
		t.Errorf("sessionIdx past end = %d, want %d", rm.sessionIdx, len(convs)-1)
	}
}

func TestUpdateSessions_Up_DecreasesIndex(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.sessionIdx = 2

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	rm := next.(Model)

	if rm.sessionIdx != 1 {
		t.Errorf("sessionIdx after 'k' = %d, want 1", rm.sessionIdx)
	}
}

func TestUpdateSessions_Up_Arrow_DecreasesIndex(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.sessionIdx = 2

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyUp})
	rm := next.(Model)

	if rm.sessionIdx != 1 {
		t.Errorf("sessionIdx after KeyUp = %d, want 1", rm.sessionIdx)
	}
}

func TestUpdateSessions_Up_ClampsAtZero(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.sessionIdx = 0

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	rm := next.(Model)

	if rm.sessionIdx != 0 {
		t.Errorf("sessionIdx before 0 = %d, want 0", rm.sessionIdx)
	}
}

func TestUpdateSessions_Enter_SetsActiveConvAndGoesToChat(t *testing.T) {
	convs := fakeConvs()
	m := sessionModel(convs)
	m.sessionIdx = 1 // select the second conv

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	if rm.activeConvID != convs[1].ID {
		t.Errorf("activeConvID = %q, want %q", rm.activeConvID, convs[1].ID)
	}
	if rm.screen != screenChat {
		t.Errorf("screen = %v, want screenChat", rm.screen)
	}
	if rm.focus != focusEditor {
		t.Errorf("focus = %v, want focusEditor", rm.focus)
	}
}

func TestUpdateSessions_Enter_EmptyList_DoesNothing(t *testing.T) {
	m := sessionModel(nil)

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	// Must stay on sessions screen; activeConvID must not be set.
	if rm.screen != screenSessions {
		t.Errorf("screen = %v, want screenSessions (empty list enter)", rm.screen)
	}
	if rm.activeConvID != "" {
		t.Errorf("activeConvID = %q, want empty (no selection)", rm.activeConvID)
	}
}

func TestUpdateSessions_Esc_GoesToPrevScreen(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.prevScreen = screenChat

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyEscape})
	rm := next.(Model)

	if rm.screen != screenChat {
		t.Errorf("screen after esc = %v, want screenChat (prevScreen)", rm.screen)
	}
}

// ---------------------------------------------------------------------------
// 2. sessionsLoadedMsg — handled GLOBALLY (PR4b); tests use m.Update
//
// sessionsLoadedMsg is now handled in the global switch (model.go Update) so
// both the welcome resume-list panel and the sessions screen receive the update.
// Tests that previously called updateSessions directly now use m.Update.
// ---------------------------------------------------------------------------

func TestUpdateSessions_SessionsLoadedMsg_Populates(t *testing.T) {
	m := sessionModel(nil)
	convs := fakeConvs()

	// Use m.Update — sessionsLoadedMsg is now a global message (PR4b).
	next, _ := m.Update(sessionsLoadedMsg{convs: convs})
	rm := next.(Model)

	if len(rm.sessions) != len(convs) {
		t.Fatalf("sessions len = %d, want %d", len(rm.sessions), len(convs))
	}
	if rm.sessionIdx != 0 {
		t.Errorf("sessionIdx = %d, want 0 (clamped to start)", rm.sessionIdx)
	}
}

func TestUpdateSessions_SessionsLoadedMsg_ClampsIdx(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.sessionIdx = 99 // out of range

	// Load only 1 conv via global Update.
	next, _ := m.Update(sessionsLoadedMsg{convs: fakeConvs()[:1]})
	rm := next.(Model)

	if rm.sessionIdx != 0 {
		t.Errorf("sessionIdx = %d, want 0 (clamped to len-1=0)", rm.sessionIdx)
	}
}

func TestUpdateSessions_SessionsLoadedMsg_Empty(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.sessionIdx = 1

	next, _ := m.Update(sessionsLoadedMsg{convs: nil})
	rm := next.(Model)

	if len(rm.sessions) != 0 {
		t.Errorf("sessions len = %d, want 0", len(rm.sessions))
	}
	if rm.sessionIdx != 0 {
		t.Errorf("sessionIdx = %d, want 0", rm.sessionIdx)
	}
}

func TestUpdateSessions_SessionsLoadedMsg_WithError_DoesNotPanic(t *testing.T) {
	m := sessionModel(nil)

	// Should not panic, sessions stays empty (error path sets sessionsErr only).
	next, _ := m.Update(sessionsLoadedMsg{err: context.DeadlineExceeded})
	rm := next.(Model)

	if len(rm.sessions) != 0 {
		t.Errorf("sessions len = %d after error, want 0", len(rm.sessions))
	}
}

// ---------------------------------------------------------------------------
// 3. tab in chat → cycles mode (Phase A: Tab no longer navigates to sessions)
// ---------------------------------------------------------------------------

func TestHandleChatKey_Tab_CyclesMode(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	rm := next.(Model)

	// Tab must NOT navigate to sessions (Phase A: Tab cycles mode).
	if rm.screen == screenSessions {
		t.Error("screen after tab in chat = screenSessions; Tab must cycle mode, not navigate to sessions (Phase A)")
	}
}

// ---------------------------------------------------------------------------
// 4. tab in welcome → cycles mode (Phase A: Tab no longer navigates to sessions)
// ---------------------------------------------------------------------------

func TestUpdateWelcome_Tab_CyclesMode(t *testing.T) {
	m := newTestModel()
	m.screen = screenWelcome

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	rm := next.(Model)

	// Tab must NOT navigate to sessions (Phase A: Tab cycles mode).
	if rm.screen == screenSessions {
		t.Error("screen after tab in welcome = screenSessions; Tab must cycle mode, not navigate to sessions (Phase A)")
	}
}

// ---------------------------------------------------------------------------
// 5. submit(text, convID) carries ConversationID
// ---------------------------------------------------------------------------

func TestTUIChannel_Submit_WithConvID_SetsConversationID(t *testing.T) {
	inbox := make(chan channel.IncomingMessage, 1)
	ch := newTUIChannel()
	_ = ch.Start(context.Background(), inbox)

	wantConvID := "conv-abc12345"
	cmd := ch.submit("hello", wantConvID)
	msg := cmd()

	if _, ok := msg.(promptSentMsg); !ok {
		t.Fatalf("cmd() returned %T, want promptSentMsg", msg)
	}

	select {
	case im := <-inbox:
		if im.ConversationID != wantConvID {
			t.Errorf("ConversationID = %q, want %q", im.ConversationID, wantConvID)
		}
		if im.Content.TextOnly() != "hello" {
			t.Errorf("Content = %q, want %q", im.Content.TextOnly(), "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: no IncomingMessage received")
	}
}

func TestTUIChannel_Submit_EmptyConvID_DoesNotBreak(t *testing.T) {
	inbox := make(chan channel.IncomingMessage, 1)
	ch := newTUIChannel()
	_ = ch.Start(context.Background(), inbox)

	cmd := ch.submit("hello", "")
	msg := cmd()

	if _, ok := msg.(promptSentMsg); !ok {
		t.Fatalf("cmd() returned %T, want promptSentMsg", msg)
	}

	select {
	case im := <-inbox:
		if im.ConversationID != "" {
			t.Errorf("ConversationID = %q, want empty", im.ConversationID)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout: no IncomingMessage received")
	}
}

// ---------------------------------------------------------------------------
// 6. modelPickerPanel.Render
// ---------------------------------------------------------------------------

func TestModelPickerPanel_Render_WithData(t *testing.T) {
	p := newModelPickerPanel(newTuiStyles(), "anthropic", "claude-opus-4-5")
	got := p.Render(40, 20)

	if got == "" {
		t.Fatal("modelPickerPanel.Render: got empty, want content")
	}
	if !strings.Contains(got, "anthropic") {
		t.Errorf("Render: expected 'anthropic' in output:\n%s", got)
	}
	if !strings.Contains(got, "claude-opus-4-5") {
		t.Errorf("Render: expected 'claude-opus-4-5' in output:\n%s", got)
	}
}

func TestModelPickerPanel_Render_EmptyProviderModel_ReturnsEmpty(t *testing.T) {
	p := newModelPickerPanel(newTuiStyles(), "", "")
	got := p.Render(40, 20)

	if got != "" {
		t.Errorf("modelPickerPanel.Render with empty provider/model: got %q, want empty", got)
	}
}

func TestModelPickerPanel_ImplementsPanel(t *testing.T) {
	var _ Panel = newModelPickerPanel(newTuiStyles(), "p", "m")
}

// ---------------------------------------------------------------------------
// 7. renderSessions
// ---------------------------------------------------------------------------

func TestRenderSessions_EmptyList_ShowsNoSessionsHint(t *testing.T) {
	m := newTestModel()
	m.screen = screenSessions
	m.width = 80
	m.height = 24

	got := renderSessions(m, 80, 20)

	if !strings.Contains(got, "no sessions") {
		t.Errorf("renderSessions with empty list: expected 'no sessions' in output:\n%s", got)
	}
}

func TestRenderSessions_WithSessions_ShowsShortIDAndTitle(t *testing.T) {
	convs := fakeConvs()
	m := newTestModel()
	m.screen = screenSessions
	m.sessions = convs
	m.sessionIdx = 0
	m.width = 80
	m.height = 24

	got := renderSessions(m, 80, 20)

	if got == "" {
		t.Fatal("renderSessions: got empty, want content")
	}
	// short ID = first 8 chars of "conv-abc12345"
	if !strings.Contains(got, "conv-abc") {
		t.Errorf("renderSessions: expected short ID 'conv-abc' in output:\n%s", got)
	}
	if !strings.Contains(got, "Alpha session") {
		t.Errorf("renderSessions: expected title 'Alpha session' in output:\n%s", got)
	}
}

func TestRenderSessions_NoTitle_ShowsUntitled(t *testing.T) {
	m := newTestModel()
	m.screen = screenSessions
	m.sessions = []store.Conversation{
		{ID: "conv-ghi11111", ChannelID: "tui", Status: "active", UpdatedAt: time.Now()},
	}
	m.sessionIdx = 0

	got := renderSessions(m, 80, 20)

	if !strings.Contains(got, "(untitled)") {
		t.Errorf("renderSessions: expected '(untitled)' for conv with no title:\n%s", got)
	}
}

func TestRenderSessions_WithBranch_ShowsBranchMarker(t *testing.T) {
	m := newTestModel()
	m.screen = screenSessions
	m.sessions = []store.Conversation{
		{
			ID:           "conv-fork1234",
			ChannelID:    "tui",
			Status:       "active",
			UpdatedAt:    time.Now(),
			ParentConvID: "conv-parent01",
			Metadata:     map[string]string{"title": "Forked chat"},
		},
	}
	m.sessionIdx = 0

	got := renderSessions(m, 80, 20)

	// Branch marker must appear for convs with a parent.
	if !strings.Contains(got, "⎇") {
		t.Errorf("renderSessions: expected branch marker '⎇' for forked conv:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// 8. footer.screen is updated on every screen transition (Fix 1)
// ---------------------------------------------------------------------------

// TestFooter_TabFromChat_FooterUnchanged verifies that tab in chat (now: mode cycle)
// does NOT switch the footer to screenSessions (Phase A: Tab cycles mode).
func TestFooter_TabFromChat_FooterUnchanged(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor
	m.footer = footerHints{screen: screenChat}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	rm := next.(Model)

	// Footer must NOT change to sessions (Tab cycles mode now).
	if rm.footer.screen == screenSessions {
		t.Error("footer.screen after tab from chat = screenSessions; Tab must cycle mode (Phase A)")
	}
}

// TestFooter_TabFromWelcome_FooterUnchanged verifies that tab in welcome (now: mode cycle)
// does NOT switch the footer to screenSessions (Phase A: Tab cycles mode).
func TestFooter_TabFromWelcome_FooterUnchanged(t *testing.T) {
	m := newTestModel()
	m.screen = screenWelcome
	m.footer = footerHints{screen: screenWelcome}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	rm := next.(Model)

	// Footer must NOT change to sessions (Tab cycles mode now).
	if rm.footer.screen == screenSessions {
		t.Error("footer.screen after tab from welcome = screenSessions; Tab must cycle mode (Phase A)")
	}
}

// TestFooter_EnterResume_SetsChatScreen verifies that enter-resume in sessions
// updates m.footer.screen to screenChat (Fix 1).
func TestFooter_EnterResume_SetsChatScreen(t *testing.T) {
	convs := fakeConvs()
	m := sessionModel(convs)
	m.footer = footerHints{screen: screenSessions}

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	if rm.footer.screen != screenChat {
		t.Errorf("footer.screen after enter-resume = %v, want screenChat", rm.footer.screen)
	}
}

// TestFooter_EscFromSessions_SetsPrevScreen verifies that esc in sessions
// updates m.footer.screen to prevScreen (Fix 1).
func TestFooter_EscFromSessions_SetsPrevScreen(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.prevScreen = screenChat
	m.footer = footerHints{screen: screenSessions}

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyEscape})
	rm := next.(Model)

	if rm.footer.screen != screenChat {
		t.Errorf("footer.screen after esc (prevScreen=chat) = %v, want screenChat", rm.footer.screen)
	}
}

// ---------------------------------------------------------------------------
// 9. modelPickerPanel with real provider/model values (Fix 2)
// ---------------------------------------------------------------------------

// TestModelPickerPanel_WithRealValues_RendersProviderAndModel verifies that
// constructing the panel with a real provider and model renders both strings.
// This mirrors how RunTUI injects cfg.Models.Default values (Fix 2).
func TestModelPickerPanel_WithRealValues_RendersProviderAndModel(t *testing.T) {
	p := newModelPickerPanel(newTuiStyles(), "anthropic", "claude-sonnet-4-6")
	got := p.Render(40, 20)

	if got == "" {
		t.Fatal("modelPickerPanel.Render with real values: got empty, want content")
	}
	if !strings.Contains(got, "anthropic") {
		t.Errorf("modelPickerPanel.Render: expected 'anthropic' in output:\n%s", got)
	}
	if !strings.Contains(got, "claude-sonnet-4-6") {
		t.Errorf("modelPickerPanel.Render: expected 'claude-sonnet-4-6' in output:\n%s", got)
	}
}

// TestModelPickerPanel_Empty_ReturnsEmpty verifies the zero-value sentinel
// (provider="" or model="") renders "" (Fix 2: empty → "").
func TestModelPickerPanel_Empty_ReturnsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		model    string
	}{
		{"both empty", "", ""},
		{"only provider", "anthropic", ""},
		{"only model", "", "claude-sonnet-4-6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := newModelPickerPanel(newTuiStyles(), tt.provider, tt.model)
			got := p.Render(40, 20)
			if got != "" {
				t.Errorf("modelPickerPanel.Render(%q,%q): got %q, want empty", tt.provider, tt.model, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 10. ANSI-safe truncation for CompactedSummary (Fix 3)
// ---------------------------------------------------------------------------

// TestRenderSessions_CompactedSummary_TruncatesAnsiSafe verifies that a
// CompactedSummary longer than maxSummaryLen is truncated at a rune boundary,
// not a byte boundary. With ansi.Truncate the result is a clean visible-width
// limit; with a byte-slice the summary variable contains invalid UTF-8 before
// it reaches the outer ansi.Truncate, which is the bug (Fix 3).
//
// We verify by checking that the rendered summary line does not exceed the
// width limit and contains the "summary" label — both would fail if
// the truncation panicked or produced garbage.
func TestRenderSessions_CompactedSummary_TruncatesAnsiSafe(t *testing.T) {
	// Wide emoji (2-column each) ensure visible-width truncation is correct.
	// 60 emoji × 2 cols = 120 visible cols — well above maxSummaryLen (120 chars).
	craftedSummary := strings.Repeat("🎯", 60) // 60 × 4 bytes = 240 bytes total

	m := newTestModel()
	m.screen = screenSessions
	m.sessions = []store.Conversation{
		{
			ID:               "conv-cjk12345",
			ChannelID:        "tui",
			Status:           "active",
			UpdatedAt:        time.Now(),
			CompactedSummary: craftedSummary,
			Metadata:         map[string]string{"title": "CJK test"},
		},
	}
	m.sessionIdx = 0

	// Must not panic; summary label must appear.
	got := renderSessions(m, 80, 20)
	if !strings.Contains(got, "summary") {
		t.Errorf("renderSessions: expected 'summary' label in output:\n%s", got)
	}
	// None of the rendered lines should exceed 80 visible columns.
	for _, line := range strings.Split(got, "\n") {
		w := ansi.StringWidth(line)
		if w > 80 {
			t.Errorf("renderSessions: line exceeds width 80 (got %d): %q", w, line)
		}
	}
}

// ---------------------------------------------------------------------------
// 11. sessionsLoadedMsg.err surfaced in renderSessions (Fix 4)
// ---------------------------------------------------------------------------

// TestUpdateSessions_SessionsLoadedMsg_WithError_SetsSessionsErr verifies that
// when sessionsLoadedMsg carries an error, renderSessions shows an error message
// rather than "no sessions yet" (Fix 4). Uses global m.Update (PR4b).
func TestUpdateSessions_SessionsLoadedMsg_WithError_SetsSessionsErr(t *testing.T) {
	m := sessionModel(nil)

	next, _ := m.Update(sessionsLoadedMsg{err: context.DeadlineExceeded})
	rm := next.(Model)

	// renderSessions must show error indication, not "no sessions yet".
	got := renderSessions(rm, 80, 20)
	if strings.Contains(got, "no sessions yet") {
		t.Errorf("renderSessions after load error: got 'no sessions yet', want error message:\n%s", got)
	}
	// The error string or some error indication must appear.
	if !strings.Contains(got, "error") && !strings.Contains(got, "failed") && !strings.Contains(got, "DeadlineExceeded") {
		t.Errorf("renderSessions after load error: expected error indication in output:\n%s", got)
	}
}

// TestUpdateSessions_SessionsLoadedMsg_Success_ClearsSessionsErr verifies that
// a successful reload clears a previously set error (Fix 4). Uses global m.Update (PR4b).
func TestUpdateSessions_SessionsLoadedMsg_Success_ClearsSessionsErr(t *testing.T) {
	m := sessionModel(nil)
	// First: set an error.
	next, _ := m.Update(sessionsLoadedMsg{err: context.DeadlineExceeded})
	rm := next.(Model)

	// Second: successful reload clears the error.
	next2, _ := rm.Update(sessionsLoadedMsg{convs: fakeConvs()})
	rm2 := next2.(Model)

	got := renderSessions(rm2, 80, 20)
	if strings.Contains(got, "error") || strings.Contains(got, "failed") {
		t.Errorf("renderSessions after successful reload: still shows error:\n%s", got)
	}
	// Successful load shows sessions.
	if !strings.Contains(got, "conv-abc") {
		t.Errorf("renderSessions after successful reload: expected session in output:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// 12. enter-resume clears thread and sets marker item (Fix 5)
// ---------------------------------------------------------------------------

// TestUpdateSessions_EnterResume_ClearsThreadAndSetsMarker verifies that
// after enter-resume: m.thread does NOT contain pre-resume items, DOES contain
// the resumed marker, m.activeConvID == sel.ID, m.screen == screenChat (Fix 5).
func TestUpdateSessions_EnterResume_ClearsThreadAndSetsMarker(t *testing.T) {
	convs := fakeConvs()
	m := sessionModel(convs)
	m.sessionIdx = 1

	// Put pre-resume items in the thread so we can verify they're gone.
	m.thread.append(&MsgUser{text: "old message", styles: m.styles})
	m.thread.append(&MsgDaimon{text: "old reply", styles: m.styles})

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	// Screen and ID must be set correctly.
	if rm.screen != screenChat {
		t.Errorf("screen after enter-resume = %v, want screenChat", rm.screen)
	}
	if rm.activeConvID != convs[1].ID {
		t.Errorf("activeConvID = %q, want %q", rm.activeConvID, convs[1].ID)
	}

	// Old items must be gone.
	for _, item := range rm.thread.items {
		if mu, ok := item.(*MsgUser); ok && mu.text == "old message" {
			t.Error("thread still contains pre-resume MsgUser 'old message'")
		}
		if md, ok := item.(*MsgDaimon); ok && md.text == "old reply" {
			t.Error("thread still contains pre-resume MsgDaimon 'old reply'")
		}
	}

	// Must contain at least one item (the resumed marker).
	if len(rm.thread.items) == 0 {
		t.Error("thread is empty after enter-resume; expected at least a resumed marker item")
	}

	// The marker item must be a MsgDaimon containing the resume indicator.
	found := false
	for _, item := range rm.thread.items {
		if md, ok := item.(*MsgDaimon); ok {
			if strings.Contains(md.text, "resumed") || strings.Contains(md.text, "↩") {
				found = true
				break
			}
		}
	}
	if !found {
		t.Errorf("thread after enter-resume: no resume marker item found; items = %v", rm.thread.items)
	}
}

// TestUpdateSessions_EnterResume_ResetsBreadcrumb verifies the breadcrumb's
// per-session counters do not bleed into a resumed session.
func TestUpdateSessions_EnterResume_ResetsBreadcrumb(t *testing.T) {
	convs := fakeConvs()
	m := sessionModel(convs)
	m.sessionIdx = 1

	// Simulate an accumulated breadcrumb from the prior session.
	m.breadcrumb = breadcrumb{styles: m.styles, label: "old-session", turns: 12, tokensIn: 9000, tokensOut: 1200, ago: "5m ago"}

	next, _ := m.updateSessions(tea.KeyMsg{Type: tea.KeyEnter})
	rm := next.(Model)

	bc := rm.breadcrumb
	if bc.turns != 0 || bc.tokensIn != 0 || bc.tokensOut != 0 || bc.label != "" || bc.ago != "" {
		t.Errorf("breadcrumb not reset on resume: %+v", bc)
	}
}
