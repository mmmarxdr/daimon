package tui

// rail_panels_pr4b_test.go — STRICT TDD tests for PR4b: environmentPanel +
// resumeListPanel, sessionsLoadedMsg global handling, Init loads sessions.
//
// Test order:
//   1. environmentPanel.Render with all fields set → contains each field value
//   2. environmentPanel.Render with long cwd → truncates to width
//   3. environmentPanel.Render with ALL fields empty → returns ""
//   4. resumeListPanel with sessions → Render contains short id + title
//   5. resumeListPanel empty → Render returns ""
//   6. sessionsLoadedMsg on screenWelcome → m.sessions set + rail panel updated
//   7. sessionsLoadedMsg on screenSessions → same effect (screen-independent)
//   8. sessionsLoadedMsg with error → m.sessionsErr set, m.sessions unchanged
//   9. Regression: sessions screen renders after global sessionsLoadedMsg
//  10. Init when events==nil returns non-nil cmd (loadSessionsCmd)

import (
	"strings"
	"testing"
	"time"

	"daimon/internal/store"
)

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// fakeEnvPanel returns an environmentPanel with all fields populated.
func fakeEnvPanel() *environmentPanel {
	return newEnvironmentPanel(
		newTuiStyles(),
		"/home/user/project",
		"anthropic/claude-sonnet-4-6",
		"go1.22.0",
		"linux/amd64",
		"sqlite",
	)
}

func fakeSessionConvs() []store.Conversation {
	now := time.Now()
	return []store.Conversation{
		{
			ID:        "conv-abcdef12",
			ChannelID: "tui",
			Status:    "active",
			UpdatedAt: now,
			Metadata:  map[string]string{"title": "First session"},
		},
		{
			ID:        "conv-xyz99999",
			ChannelID: "tui",
			Status:    "completed",
			UpdatedAt: now.Add(-5 * time.Minute),
		},
		{
			ID:        "conv-333aaaaa",
			ChannelID: "tui",
			Status:    "active",
			UpdatedAt: now.Add(-1 * time.Hour),
			Metadata:  map[string]string{"title": "Third session"},
		},
	}
}

// ---------------------------------------------------------------------------
// 1. environmentPanel — with data renders all fields
// ---------------------------------------------------------------------------

func TestEnvironmentPanel_Render_WithData_ContainsAllFields(t *testing.T) {
	p := fakeEnvPanel()
	// width=46: box overhead (border+padding = 4 cols) + enough inner space for
	// "model   anthropic/claude-sonnet-4-6" (35 visible chars) + 2-col lipgloss slack.
	got := p.Render(46, 20)

	if got == "" {
		t.Fatal("environmentPanel.Render with data: got empty string, want non-empty")
	}

	wantContains := []string{
		"/home/user/project",
		"anthropic/claude-sonnet-4-6",
		"go1.22.0",
		"linux/amd64",
		"sqlite",
	}
	for _, want := range wantContains {
		if !strings.Contains(got, want) {
			t.Errorf("environmentPanel.Render: expected %q in output:\n%s", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 2. environmentPanel — long cwd truncates within width
// ---------------------------------------------------------------------------

func TestEnvironmentPanel_Render_LongCwd_TruncatesWithinWidth(t *testing.T) {
	longCwd := "/very/long/path/that/should/be/truncated/because/it/exceeds/width"
	p := newEnvironmentPanel(newTuiStyles(), longCwd, "p/m", "go1.22.0", "linux/amd64", "sqlite")

	const width = 20
	got := p.Render(width, 20)

	if got == "" {
		t.Fatal("environmentPanel.Render: got empty string, want non-empty")
	}
	// No line should exceed the width in visible characters.
	for _, line := range strings.Split(got, "\n") {
		// Use raw string width check: the truncated cwd line must not contain
		// the full long path (a cheap proxy that doesn't need ansi import here).
		// The key assertion is that the long path is NOT present verbatim.
		if strings.Contains(line, longCwd) {
			t.Errorf("environmentPanel.Render(width=%d): long cwd not truncated; line = %q", width, line)
		}
	}
}

// ---------------------------------------------------------------------------
// 3. environmentPanel — ALL fields empty → returns ""
// ---------------------------------------------------------------------------

func TestEnvironmentPanel_Render_AllEmpty_ReturnsEmpty(t *testing.T) {
	p := newEnvironmentPanel(newTuiStyles(), "", "", "", "", "")
	got := p.Render(40, 20)
	if got != "" {
		t.Errorf("environmentPanel.Render with all-empty fields: got %q, want empty string", got)
	}
}

// ---------------------------------------------------------------------------
// 4. resumeListPanel — with sessions renders short id + title
// ---------------------------------------------------------------------------

func TestResumeListPanel_WithSessions_RendersIdAndTitle(t *testing.T) {
	p := newResumeListPanel(newTuiStyles())
	convs := fakeSessionConvs()
	p.setSessions(convs)

	got := p.Render(40, 20)

	if got == "" {
		t.Fatal("resumeListPanel.Render with sessions: got empty string, want non-empty")
	}
	// Short ID: first 8 chars of "conv-abcdef12" = "conv-abc"
	if !strings.Contains(got, "conv-abc") {
		t.Errorf("resumeListPanel.Render: expected short ID 'conv-abc' in output:\n%s", got)
	}
	if !strings.Contains(got, "First session") {
		t.Errorf("resumeListPanel.Render: expected title 'First session' in output:\n%s", got)
	}
}

func TestResumeListPanel_NoTitle_ShowsUntitled(t *testing.T) {
	p := newResumeListPanel(newTuiStyles())
	p.setSessions([]store.Conversation{
		{
			ID:        "conv-xyz99999",
			ChannelID: "tui",
			Status:    "active",
			UpdatedAt: time.Now(),
		},
	})

	got := p.Render(40, 20)

	if !strings.Contains(got, "(untitled)") {
		t.Errorf("resumeListPanel.Render: expected '(untitled)' for conv with no title:\n%s", got)
	}
}

func TestResumeListPanel_CapsAtFive(t *testing.T) {
	p := newResumeListPanel(newTuiStyles())
	convs := make([]store.Conversation, 7)
	for i := range convs {
		convs[i] = store.Conversation{
			ID:        "conv-" + string(rune('a'+i)) + "1234567",
			ChannelID: "tui",
			Status:    "active",
			UpdatedAt: time.Now(),
			Metadata:  map[string]string{"title": "Session " + string(rune('a'+i))},
		}
	}
	p.setSessions(convs)
	got := p.Render(40, 20)
	// Only first 5 should render; conv[5] and conv[6] should not appear.
	// conv[5] has ID starting with "conv-f1234567" → short ID "conv-f12"
	if strings.Contains(got, "Session f") || strings.Contains(got, "Session g") {
		t.Errorf("resumeListPanel.Render: rendered more than 5 sessions:\n%s", got)
	}
	// Positive: the first 5 (a–e) MUST render — guards against a regression that
	// drops all sessions (which would also satisfy the negative check above).
	for _, want := range []string{"Session a", "Session b", "Session c", "Session d", "Session e"} {
		if !strings.Contains(got, want) {
			t.Errorf("resumeListPanel.Render: missing expected session %q:\n%s", want, got)
		}
	}
}

// ---------------------------------------------------------------------------
// 5. resumeListPanel — empty → returns ""
// ---------------------------------------------------------------------------

func TestResumeListPanel_Empty_ReturnsEmpty(t *testing.T) {
	p := newResumeListPanel(newTuiStyles())
	got := p.Render(40, 20)
	if got != "" {
		t.Errorf("resumeListPanel.Render with no sessions: got %q, want empty string", got)
	}
}

func TestResumeListPanel_ImplementsPanel(t *testing.T) {
	var _ Panel = newResumeListPanel(newTuiStyles())
}

// ---------------------------------------------------------------------------
// 6. sessionsLoadedMsg GLOBAL — on screenWelcome sets m.sessions + panel
// ---------------------------------------------------------------------------

func TestGlobal_SessionsLoadedMsg_OnWelcome_SetsSessions(t *testing.T) {
	m := newTestModel()
	m.screen = screenWelcome
	// Register the resume-list panel so the global handler can update it.
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		panels[panelResumeList] = newResumeListPanel(m.styles)
	})

	convs := fakeSessionConvs()
	next, _ := m.Update(sessionsLoadedMsg{convs: convs})
	rm := next.(Model)

	if len(rm.sessions) != len(convs) {
		t.Fatalf("m.sessions len = %d after sessionsLoadedMsg on welcome, want %d", len(rm.sessions), len(convs))
	}

	// The resumeListPanel in the rail must reflect the sessions.
	p, ok := rm.rail.panels[panelResumeList].(*resumeListPanel)
	if !ok {
		t.Fatal("rail.panels[panelResumeList] is not a *resumeListPanel after sessionsLoadedMsg")
	}
	rendered := p.Render(40, 20)
	if rendered == "" {
		t.Error("resumeListPanel.Render after sessionsLoadedMsg on welcome: got empty, want content")
	}
	if !strings.Contains(rendered, "conv-abc") {
		t.Errorf("resumeListPanel.Render: expected short ID 'conv-abc' in output:\n%s", rendered)
	}
}

// ---------------------------------------------------------------------------
// 7. sessionsLoadedMsg GLOBAL — screen-independent (same effect on sessions screen)
// ---------------------------------------------------------------------------

func TestGlobal_SessionsLoadedMsg_OnSessions_SetsSessions(t *testing.T) {
	m := newTestModel()
	m.screen = screenSessions
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		panels[panelResumeList] = newResumeListPanel(m.styles)
	})

	convs := fakeSessionConvs()
	next, _ := m.Update(sessionsLoadedMsg{convs: convs})
	rm := next.(Model)

	if len(rm.sessions) != len(convs) {
		t.Fatalf("m.sessions len = %d after sessionsLoadedMsg on sessions screen, want %d", len(rm.sessions), len(convs))
	}

	// sessionIdx must be clamped to [0, len-1].
	if rm.sessionIdx >= len(rm.sessions) {
		t.Errorf("sessionIdx = %d, want < %d (clamped)", rm.sessionIdx, len(rm.sessions))
	}
}

// ---------------------------------------------------------------------------
// 8. sessionsLoadedMsg with error → m.sessionsErr set, m.sessions unchanged
// ---------------------------------------------------------------------------

func TestGlobal_SessionsLoadedMsg_WithError_SetsSessionsErr(t *testing.T) {
	m := newTestModel()
	m.screen = screenWelcome
	// Pre-populate sessions so we can verify they remain unchanged on error.
	m.sessions = fakeSessionConvs()
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		panels[panelResumeList] = newResumeListPanel(m.styles)
	})

	storeErr := &fakeError{"store unavailable"}
	next, _ := m.Update(sessionsLoadedMsg{err: storeErr})
	rm := next.(Model)

	if rm.sessionsErr == nil {
		t.Error("m.sessionsErr should be set after sessionsLoadedMsg with error, got nil")
	}
	// Sessions must remain as previously set (not cleared).
	if len(rm.sessions) == 0 {
		t.Error("m.sessions should remain unchanged after sessionsLoadedMsg with error")
	}
}

// fakeError is a simple error for tests that avoids importing extra packages.
type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }

// ---------------------------------------------------------------------------
// 9. Regression — sessions screen renders loaded sessions after global msg
// ---------------------------------------------------------------------------

func TestRegression_SessionsScreen_RendersAfterGlobalLoad(t *testing.T) {
	m := newTestModel()
	m.screen = screenSessions
	m.width = 80
	m.height = 24
	m.rail = copyRailWith(m.rail, func(panels map[panelID]Panel) {
		panels[panelResumeList] = newResumeListPanel(m.styles)
	})

	convs := fakeSessionConvs()
	next, _ := m.Update(sessionsLoadedMsg{convs: convs})
	rm := next.(Model)

	// renderSessions reads m.sessions which global handler now sets.
	got := renderSessions(rm, 80, 20)
	if got == "" {
		t.Fatal("renderSessions after global sessionsLoadedMsg on sessions screen: got empty, want content")
	}
	if !strings.Contains(got, "conv-abc") {
		t.Errorf("renderSessions: expected short ID 'conv-abc' after global load:\n%s", got)
	}
	if !strings.Contains(got, "First session") {
		t.Errorf("renderSessions: expected 'First session' after global load:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// 10. Init when events==nil returns non-nil cmd (loadSessionsCmd)
// ---------------------------------------------------------------------------

func TestInit_EventsNil_ReturnsLoadSessionsCmd(t *testing.T) {
	m := newTestModel()
	// events == nil simulates test / no-bus path.
	// store == nil is safe because loadSessionsCmd guards nil store.
	m.events = nil
	m.store = nil

	cmd := m.Init()
	if cmd == nil {
		t.Error("Init with events==nil: got nil cmd, want non-nil loadSessionsCmd")
	}
	// Execute cmd and verify it returns a sessionsLoadedMsg (nil store → empty load).
	msg := cmd()
	if _, ok := msg.(sessionsLoadedMsg); !ok {
		t.Errorf("Init cmd() returned %T, want sessionsLoadedMsg", msg)
	}
}
