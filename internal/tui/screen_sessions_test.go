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

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

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
// 2. sessionsLoadedMsg — populates sessions and clamps sessionIdx
// ---------------------------------------------------------------------------

func TestUpdateSessions_SessionsLoadedMsg_Populates(t *testing.T) {
	m := sessionModel(nil)
	convs := fakeConvs()

	next, _ := m.updateSessions(sessionsLoadedMsg{convs: convs})
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

	// Load only 1 conv.
	next, _ := m.updateSessions(sessionsLoadedMsg{convs: fakeConvs()[:1]})
	rm := next.(Model)

	if rm.sessionIdx != 0 {
		t.Errorf("sessionIdx = %d, want 0 (clamped to len-1=0)", rm.sessionIdx)
	}
}

func TestUpdateSessions_SessionsLoadedMsg_Empty(t *testing.T) {
	m := sessionModel(fakeConvs())
	m.sessionIdx = 1

	next, _ := m.updateSessions(sessionsLoadedMsg{convs: nil})
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

	// Should not panic, sessions stays empty.
	next, _ := m.updateSessions(sessionsLoadedMsg{err: context.DeadlineExceeded})
	rm := next.(Model)

	if len(rm.sessions) != 0 {
		t.Errorf("sessions len = %d after error, want 0", len(rm.sessions))
	}
}

// ---------------------------------------------------------------------------
// 3. tab in chat → screenSessions + non-nil cmd
// ---------------------------------------------------------------------------

func TestHandleChatKey_Tab_NavigatesToSessions(t *testing.T) {
	m := newTestModel()
	m.screen = screenChat
	m.focus = focusEditor

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	rm := next.(Model)

	if rm.screen != screenSessions {
		t.Errorf("screen after tab in chat = %v, want screenSessions", rm.screen)
	}
	if rm.prevScreen != screenChat {
		t.Errorf("prevScreen = %v, want screenChat", rm.prevScreen)
	}
	if cmd == nil {
		t.Error("tab in chat: expected a non-nil cmd (loadSessionsCmd), got nil")
	}
}

// ---------------------------------------------------------------------------
// 4. tab in welcome → screenSessions + non-nil cmd
// ---------------------------------------------------------------------------

func TestUpdateWelcome_Tab_NavigatesToSessions(t *testing.T) {
	m := newTestModel()
	m.screen = screenWelcome

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	rm := next.(Model)

	if rm.screen != screenSessions {
		t.Errorf("screen after tab in welcome = %v, want screenSessions", rm.screen)
	}
	if rm.prevScreen != screenWelcome {
		t.Errorf("prevScreen = %v, want screenWelcome", rm.prevScreen)
	}
	if cmd == nil {
		t.Error("tab in welcome: expected a non-nil cmd (loadSessionsCmd), got nil")
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
