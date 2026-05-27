package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestTopBar_Render_AllSlotsPresent verifies that a TopBar rendered at 80
// columns contains all expected slot tokens: the brand glyph, cwd, branch,
// model, mode, cost, and status.
func TestTopBar_Render_AllSlotsPresent(t *testing.T) {
	s := newTuiStyles()
	tb := topBar{}
	// Width 120: the full topbar (⫶ daimon │ cwd · branch │ model · mode … cost · status)
	// needs adequate width for every slot to be present without truncation. At
	// narrow widths (e.g. 80) the cwd/right side legitimately truncates.
	tb.SetData("⫶", "/home/user/project", "main", "claude-3-5", "build", "$0.01", "ready")
	rendered := tb.Render(120, s)

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

// firstLine returns the first line of a multi-line string.
func firstLine(s string) string {
	if idx := strings.IndexByte(s, '\n'); idx >= 0 {
		return s[:idx]
	}
	return s
}

// TestInputBar_Render_WidthFits80 verifies that an inputBar rendered at width=80
// produces a first line whose visible width is exactly 80 columns.
//
// RED: before the fix, inputBarStyle.Width(width-2) combined with
// RoundedBorder+Padding(0,1) overhead of 4 cols produces an outer width of
// (80-2)+4 = 82, which overflows the terminal column and causes line wrapping.
func TestInputBar_Render_WidthFits80(t *testing.T) {
	s := newTuiStyles()
	ib := newInputBar()
	rendered := ib.Render(80, s)

	line := firstLine(rendered)
	got := ansi.StringWidth(line)
	if got != 80 {
		t.Errorf("inputBar.Render(80) first-line visible width = %d, want 80\nline = %q", got, line)
	}
}

// TestInputBar_Render_NarrowWidth_NoPanic verifies that rendering at a very
// narrow width (e.g. 2) does not panic and does not produce a negative Width.
func TestInputBar_Render_NarrowWidth_NoPanic(t *testing.T) {
	s := newTuiStyles()
	ib := newInputBar()
	// Must not panic at extreme narrow widths.
	for _, w := range []int{0, 1, 2, 3, 4} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("inputBar.Render(%d) panicked: %v", w, r)
				}
			}()
			_ = ib.Render(w, s)
		}()
	}
}
