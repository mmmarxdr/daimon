package tui

import (
	"strings"
	"testing"
)

// TestTopBar_Render_AllSlotsPresent verifies that a TopBar rendered at 80
// columns contains all expected slot tokens: the brand glyph, cwd, branch,
// model, mode, cost, and status.
func TestTopBar_Render_AllSlotsPresent(t *testing.T) {
	s := newTuiStyles()
	tb := topBar{}
	tb.SetData("⫶", "/home/user/project", "main", "claude-3-5", "build", "$0.01", "ready")
	rendered := tb.Render(80, s)

	tokens := []string{"⫶", "/home/user/project", "main", "claude-3-5", "build", "$0.01", "ready"}
	for _, tok := range tokens {
		if !strings.Contains(rendered, tok) {
			t.Errorf("TopBar.Render(80) missing slot token %q\nrendered: %q", tok, rendered)
		}
	}
}

// TestInputBar_AbsentOnDiffScreen verifies that the model's View() does NOT
// contain the input bar sentinel string when the screen is screenDiff.
// (per components.md §Matrix: InputBar only on welcome, chat, error)
func TestInputBar_AbsentOnDiffScreen(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenDiff

	view := m.View()
	// The sentinel string is the placeholder text set on the inputBar.
	// On diff screen, no input bar should be rendered.
	if strings.Contains(view, inputBarSentinel) {
		t.Errorf("View() on screenDiff contains inputBar sentinel %q — InputBar must be hidden on diff screen", inputBarSentinel)
	}
}

// TestFooterHints_Render_NonEmpty verifies that FooterHints renders a
// non-empty string for every screen state.
func TestFooterHints_Render_NonEmpty(t *testing.T) {
	s := newTuiStyles()
	screens := []screenState{
		screenWelcome, screenChat, screenDiff, screenSlash,
		screenTools, screenSessions, screenError,
	}
	for _, sc := range screens {
		fh := footerHints{}
		fh.SetScreen(sc)
		rendered := fh.Render(80, s)
		if strings.TrimSpace(rendered) == "" {
			t.Errorf("FooterHints.Render(80) is empty for screen %v", sc)
		}
	}
}

// TestInputBar_PresentOnWelcome verifies InputBar is included in View() on welcome.
func TestInputBar_PresentOnWelcome(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenWelcome

	view := m.View()
	if !strings.Contains(view, inputBarSentinel) {
		t.Errorf("View() on screenWelcome missing inputBar sentinel %q", inputBarSentinel)
	}
}
