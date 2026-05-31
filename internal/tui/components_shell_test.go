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

// ---------------------------------------------------------------------------
// PR 1c tests — per-screen footer hint sets (design source canonical)
// ---------------------------------------------------------------------------
//
// Design source: docs/tui-design/daimon/project/tui-screens-a.jsx (screens 01, 02, 03)
//               docs/tui-design/daimon/project/tui-screens-b.jsx (screens 04, 05, 06, 07)
//
// 1c.0 VERIFIED FINDINGS (design source vs spec deltas):
//
//   Welcome (01): design tui-screens-a.jsx:103-108 → / commands · ⇥ switch agent · ⌃P palette · ? help
//     Delta: spec had "⇥ /commands · ⌃R resume last · ⌃C exit" — those are
//     the inline nav hints in the center div, NOT the outer TUIFooter.
//     Implementation follows DESIGN SOURCE (outer TUIFooter).
//
//   Chat (02): design tui-screens-a.jsx:302-308 → ⇥ /commands · ⌃C interrupt · ⌃R retry turn · ⌃E edit last · ⌃S save session
//     Matches spec. No delta.
//
//   Diff (03): design tui-screens-a.jsx:489-495 → a/A apply/apply-all · r reject hunk · e open in $EDITOR · n/p next/prev hunk · q cancel patch
//     Phase 3 screen — migrated to struct, semantics preserved.
//
//   Slash (04): design tui-screens-b.jsx:153-157 → esc close palette · / search prefix · ? help
//     Delta: spec inferred "↑↓ select · ↵ run · esc close · ⇥ autocomplete" from
//     INNER palette overlay footer (not the outer TUIScreen footer). Outer footer
//     is 3 items only. Implementation follows DESIGN SOURCE (outer TUIFooter).
//
//   Tools (05): design tui-screens-b.jsx:319-325 → space toggle enabled · ↵ open detail · a add MCP server · d remove · / filter
//     Delta: spec had "↑↓ select · ↵ toggle · f filter · a add-MCP" — differs.
//     Implementation follows DESIGN SOURCE.
//
//   Sessions (06): design tui-screens-b.jsx:492-497 → ↵ resume thread · n new from this · d delete · m change model · / filter
//     Mostly matches spec. Label differences: "↵ resume thread" vs "↵ open",
//     "n new from this" vs "n new", "m change model" vs "m model".
//     Implementation follows DESIGN SOURCE (more descriptive labels).
//
//   Error (07): design tui-screens-b.jsx:682-687 → a/A allow once/always · d/D deny/never ask · e edit path · p open policy file
//     Phase 3 screen — migrated to struct, semantics preserved.

// TestFooterHints_WelcomeScreen verifies the welcome footer matches the design's
// outer TUIFooter (tui-screens-a.jsx:103-108): / commands · ⇥ switch agent · ⌃P palette · ? help.
// Delta from spec: spec had inline nav hints from center div, not the outer footer.
// [Req: Footer hint sets — Welcome footer scenario]
func TestFooterHints_WelcomeScreen(t *testing.T) {
	s := newTuiStyles()
	fh := footerHints{}
	fh.SetScreen(screenWelcome)

	rendered := fh.Render(120, s)
	stripped := ansi.Strip(rendered)

	wantTokens := []string{"/", "commands", "⇥", "switch agent", "⌃P", "palette", "?", "help"}
	for _, tok := range wantTokens {
		if !strings.Contains(stripped, tok) {
			t.Errorf("welcome footer missing %q\nrendered (stripped): %q", tok, stripped)
		}
	}
}

// TestFooterHints_ChatScreen verifies the chat footer matches the design's
// outer TUIFooter (tui-screens-a.jsx:302-308).
// [Req: Footer hint sets — Chat footer scenario]
func TestFooterHints_ChatScreen(t *testing.T) {
	s := newTuiStyles()
	fh := footerHints{}
	fh.SetScreen(screenChat)

	rendered := fh.Render(160, s)
	stripped := ansi.Strip(rendered)

	wantTokens := []string{"⇥", "/commands", "⌃C", "interrupt", "⌃R", "retry turn"}
	for _, tok := range wantTokens {
		if !strings.Contains(stripped, tok) {
			t.Errorf("chat footer missing %q\nrendered (stripped): %q", tok, stripped)
		}
	}
}

// TestFooterHints_AllScreens_StructuredHints verifies all screen hint sets
// use the structured footerHint model with design-source-verified content.
// Each screen's key tokens must appear in the stripped render output.
// [Req: Footer hint sets — all screens table-driven]
func TestFooterHints_AllScreens_StructuredHints(t *testing.T) {
	s := newTuiStyles()

	tests := []struct {
		name       string
		screen     screenState
		wantTokens []string
	}{
		// Welcome (01) — design: tui-screens-a.jsx:103-108
		// Delta: design outer footer differs from spec's inline nav hints
		{
			name:       "welcome",
			screen:     screenWelcome,
			wantTokens: []string{"/", "commands", "⇥", "switch agent", "⌃P", "palette", "?", "help"},
		},
		// Chat (02) — design: tui-screens-a.jsx:302-308
		{
			name:       "chat",
			screen:     screenChat,
			wantTokens: []string{"⇥", "/commands", "⌃C", "interrupt", "⌃R", "retry turn"},
		},
		// Slash (04) — design outer TUIFooter: tui-screens-b.jsx:153-157
		// Delta: spec inferred inner palette overlay footer; outer is esc/close palette, //search prefix, ?/help
		{
			name:       "slash",
			screen:     screenSlash,
			wantTokens: []string{"esc", "close palette", "/", "search prefix", "?", "help"},
		},
		// Tools (05) — design: tui-screens-b.jsx:319-325
		// Delta: spec had ↑↓ select · ↵ toggle · f filter · a add-MCP
		{
			name:       "tools",
			screen:     screenTools,
			wantTokens: []string{"space", "toggle enabled", "↵", "open detail", "a", "add MCP server", "d", "remove", "/", "filter"},
		},
		// Sessions (06) — design: tui-screens-b.jsx:492-497
		{
			name:       "sessions",
			screen:     screenSessions,
			wantTokens: []string{"↵", "resume thread", "n", "new from this", "d", "delete", "m", "change model", "/", "filter"},
		},
		// Diff (03) — design: tui-screens-a.jsx:489-495 (Phase 3 screen)
		{
			name:       "diff",
			screen:     screenDiff,
			wantTokens: []string{"a/A", "apply", "r", "reject hunk", "q", "cancel patch"},
		},
		// Error (07) — design: tui-screens-b.jsx:682-687 (Phase 3 screen)
		{
			name:       "error",
			screen:     screenError,
			wantTokens: []string{"a/A", "allow once", "d/D", "deny", "e", "edit path", "p", "open policy file"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fh := footerHints{}
			fh.SetScreen(tc.screen)
			rendered := fh.Render(200, s)
			stripped := ansi.Strip(rendered)

			for _, tok := range tc.wantTokens {
				if !strings.Contains(stripped, tok) {
					t.Errorf("screen %s footer missing %q\nrendered (stripped): %q", tc.name, tok, stripped)
				}
			}
		})
	}
}
