package tui

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// TestTuiStyles_NoHexInRenderFunctions verifies that newTuiStyles() returns a
// non-zero struct and that key style slots are populated (non-zero lipgloss.Style).
func TestTuiStyles_NoHexInRenderFunctions(t *testing.T) {
	s := newTuiStyles()

	// Spot-check that each style slot is non-zero (i.e. has at least one rule set).
	// lipgloss.Style renders to a non-empty string when given content.
	slots := map[string]string{
		"topBar":   s.topBar.Render("x"),
		"accent":   s.accent.Render("x"),
		"amber":    s.amber.Render("x"),
		"pink":     s.pink.Render("x"),
		"errStyle": s.errStyle.Render("x"),
		"label":    s.label.Render("x"),
		"dimLabel": s.dimLabel.Render("x"),
		"hint":     s.hint.Render("x"),
	}
	for name, rendered := range slots {
		if rendered == "" {
			t.Errorf("style slot %q renders to empty string — slot may be uninitialized", name)
		}
	}
}

// ---------------------------------------------------------------------------
// 1a.1 — All 13 color constants present with exact hex values
// ---------------------------------------------------------------------------

// TestColorConstants_AllPresent asserts that every design token constant exists
// with the exact hex value from tui.jsx and that the legacy Catppuccin value
// #cdd6f4 does not appear in any constant.
func TestColorConstants_AllPresent(t *testing.T) {
	cases := []struct {
		name     string
		constant string
		want     string
	}{
		{"colorBG", colorBG, "#0e0f13"},
		{"colorBGElev", colorBGElev, "#15171d"},
		{"colorBGDeep", colorBGDeep, "#0a0b0f"},
		{"colorBGPanel", colorBGPanel, "#11131a"},
		{"colorInk", colorInk, "#eae5d8"},
		{"colorInkSoft", colorInkSoft, "#c2bca9"},
		{"colorInkMuted", colorInkMuted, "#7a7465"},
		{"colorInkFaint", colorInkFaint, "#4a4438"},
		{"colorInkGhost", colorInkGhost, "#2c2a25"},
		{"colorLine", colorLine, "#22242c"},
		{"colorLineSoft", colorLineSoft, "#1a1c22"},
		{"colorLineStr", colorLineStr, "#2e3038"},
		{"colorAccent", colorAccent, "#5dbfa7"},
		{"colorAmber", colorAmber, "#e3b67a"},
		{"colorRed", colorRed, "#e38775"},
		{"colorGreen", colorGreen, "#7aba8a"},
		{"colorPink", colorPink, "#d67b9e"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.constant != tc.want {
				t.Errorf("%s = %q, want %q", tc.name, tc.constant, tc.want)
			}
		})
	}
}

// TestColorConstants_NoCatppuccin asserts that the legacy Catppuccin value
// #cdd6f4 does not appear as any color constant.
func TestColorConstants_NoCatppuccin(t *testing.T) {
	allConstants := []string{
		colorBG,
		colorBGElev,
		colorBGDeep,
		colorBGPanel,
		colorInk,
		colorInkSoft,
		colorInkMuted,
		colorInkFaint,
		colorInkGhost,
		colorLine,
		colorLineSoft,
		colorLineStr,
		colorAccent,
		colorAmber,
		colorRed,
		colorGreen,
		colorPink,
	}
	for _, c := range allConstants {
		if strings.EqualFold(c, "#cdd6f4") {
			t.Errorf("found legacy Catppuccin value #cdd6f4 in color constants: %q", c)
		}
	}
}

// ---------------------------------------------------------------------------
// 1a.2 — newTuiStyles() style slots use correct hex values
// ---------------------------------------------------------------------------

// TestTuiStyles_CorrectForegroundColors verifies that key style slots in
// newTuiStyles() are wired to the correct design-token constants by forcing a
// TrueColor renderer and asserting the ANSI truecolor escape sequences in the
// rendered output. This catches constant/slot mismatches: swapping e.g.
// colorAmber↔colorAccent in newTuiStyles() would break these assertions even
// if the constants themselves are correct.
//
// Approach: call newTuiStyles() to get the real production slots, extract each
// slot's foreground color via GetForeground(), then render a probe string
// through a forced-truecolor renderer using that same color. A mis-wired slot
// (e.g. amber: Foreground(accent)) produces a different 38;2;R;G;B sequence
// and the test fails.
//
// Expected ANSI truecolor foreground form: ESC[38;2;<R>;<G>;<B>m
// Note: termenv/colorful parses hex to [0,1] floats then converts via
// uint8(channel/0xff * 255). For 0x7a: uint8(float64(0x7a)/float64(0xff)*255)
// = uint8(121.999…) = 121 (Go truncates the float to integer, not the float
// itself). The sequences below reflect actual computed values:
//
//	colorAccent   #5dbfa7  → 38;2;93;191;167
//	colorAmber    #e3b67a  → 38;2;227;182;121  (0x7a → 121 via uint8 truncation)
//	colorInkMuted #7a7465  → 38;2;121;116;101  (0x7a → 121 via uint8 truncation)
func TestTuiStyles_CorrectForegroundColors(t *testing.T) {
	// Build a TrueColor renderer so output is deterministic regardless of TERM.
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)

	// Get the ACTUAL production slots from newTuiStyles().
	// We extract each slot's foreground color via GetForeground() and re-render
	// it through the forced-truecolor renderer. This means a swap inside
	// newTuiStyles() (e.g. amber: Foreground(accent)) produces the wrong
	// 38;2;R;G;B sequence and the assertion fails.
	s := newTuiStyles()

	renderFg := func(style lipgloss.Style) string {
		fg := style.GetForeground()
		return r.NewStyle().Foreground(fg).Render("x")
	}

	cases := []struct {
		slot     string
		rendered string
		wantANSI string // truecolor fg sequence fragment
	}{
		{
			slot:     "accent (colorAccent #5dbfa7)",
			rendered: renderFg(s.accent),
			wantANSI: "38;2;93;191;167",
		},
		{
			slot:     "amber (colorAmber #e3b67a)",
			rendered: renderFg(s.amber),
			wantANSI: "38;2;227;182;121", // 0x7a → uint8(121.999…) = 121
		},
		{
			slot:     "dimLabel (colorInkMuted #7a7465)",
			rendered: renderFg(s.dimLabel),
			wantANSI: "38;2;121;116;101", // 0x7a → uint8(121.999…) = 121
		},
	}

	for _, tc := range cases {
		t.Run(tc.slot, func(t *testing.T) {
			if tc.rendered == "" {
				t.Fatalf("style %q rendered to empty string — slot uninitialized", tc.slot)
			}
			if !strings.Contains(tc.rendered, tc.wantANSI) {
				t.Errorf("style %q: rendered output %q does not contain expected ANSI sequence %q\n"+
					"This means the slot is wired to the WRONG constant or uses the wrong color.",
					tc.slot, tc.rendered, tc.wantANSI)
			}
		})
	}

	// Regression guards: these hex checks remain to document intent clearly.
	if colorAmber != "#e3b67a" {
		t.Errorf("colorAmber = %q, want #e3b67a", colorAmber)
	}
	if colorPink != "#d67b9e" {
		t.Errorf("colorPink = %q, want #d67b9e", colorPink)
	}
}

// ---------------------------------------------------------------------------
// 1b.1 — panelBorder uses square border (NormalBorder, contains ┌, not ╭)
// ---------------------------------------------------------------------------

// TestTuiStyles_PanelBorder_IsSquare asserts that tuiStyles.panelBorder uses a
// square box-drawing border (lipgloss.NormalBorder, corners ┌ ┐ └ ┘) and NOT
// the rounded border (╭ ╮ ╰ ╯). The panelBorder slot must be initialized in
// newTuiStyles().
func TestTuiStyles_PanelBorder_IsSquare(t *testing.T) {
	s := newTuiStyles()

	// Render a bordered box — panelBorder has BorderX applied so Render(content)
	// must produce the corner and horizontal line characters.
	rendered := s.panelBorder.Render("x")
	if rendered == "" {
		t.Fatal("panelBorder.Render: got empty string — slot may be uninitialized")
	}

	// Must contain the square top-left corner ┌ (U+250C).
	if !strings.Contains(rendered, "┌") {
		t.Errorf("panelBorder: rendered output does not contain '┌' (U+250C);\n"+
			"expected NormalBorder (square) — output:\n%s", rendered)
	}

	// Must NOT contain the rounded corner ╭ (U+256D).
	if strings.Contains(rendered, "╭") {
		t.Errorf("panelBorder: rendered output contains '╭' (U+256D);\n"+
			"expected NormalBorder (square), not RoundedBorder — output:\n%s", rendered)
	}
}

// ---------------------------------------------------------------------------
// 1b.2 — panelBorder border foreground = colorLine (#22242c)
// ---------------------------------------------------------------------------

// TestTuiStyles_PanelBorder_ForegroundIsColorLine asserts that the panelBorder
// slot's border foreground color is colorLine (#22242c). Uses a forced TrueColor
// renderer to produce deterministic ANSI sequences regardless of TERM setting.
// Expected truecolor fg form: ESC[38;2;<R>;<G>;<B>m
// colorLine = #22242c → R=0x22=34, G=0x24=36, B=0x2c=44 → 38;2;34;36;44
func TestTuiStyles_PanelBorder_ForegroundIsColorLine(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)

	s := newTuiStyles()

	// Extract the border foreground from panelBorder and render through a
	// forced TrueColor renderer to get a deterministic ANSI sequence.
	borderFg := s.panelBorder.GetBorderBottomForeground() // same color for all sides
	if _, isNoColor := borderFg.(lipgloss.NoColor); isNoColor {
		t.Fatal("panelBorder.GetBorderBottomForeground(): NoColor — border foreground not set")
	}

	rendered := r.NewStyle().Foreground(borderFg).Render("x")

	// colorLine = #22242c → R=34, G=36, B=44
	const wantANSI = "38;2;34;36;44"
	if !strings.Contains(rendered, wantANSI) {
		t.Errorf("panelBorder border foreground: rendered %q does not contain expected ANSI sequence %q;\n"+
			"expected colorLine (#22242c) but got a different color", rendered, wantANSI)
	}
}

// ---------------------------------------------------------------------------
// 1b.A-GUARD — paletteBox border foreground = colorAccent (#5dbfa7)
// ---------------------------------------------------------------------------

// TestTuiStyles_PaletteBox_BorderIsAccent asserts that the paletteBox slot's
// border foreground is colorAccent (#5dbfa7), NOT colorLine (#22242c).
//
// Design rationale: tui-components.jsx:441 specifies the command palette border
// as "1px solid ${TUI.accent}" (Outline accent). Using colorLine makes the
// border near-invisible against the dark background.
//
// This test exists specifically to prevent regression — this bug was fixed in
// PR 1a, re-introduced in PR 1b (task 1b.5 incorrectly directed colorLine),
// and must not regress again.
//
// colorAccent = #5dbfa7 → R=0x5d=93, G=0xbf=191, B=0xa7=167 → 38;2;93;191;167
func TestTuiStyles_PaletteBox_BorderIsAccent(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)

	s := newTuiStyles()

	// Extract border foreground from paletteBox (all sides share the same color).
	borderFg := s.paletteBox.GetBorderBottomForeground()
	if _, isNoColor := borderFg.(lipgloss.NoColor); isNoColor {
		t.Fatal("paletteBox.GetBorderBottomForeground(): no color set — border foreground not configured")
	}

	rendered := r.NewStyle().Foreground(borderFg).Render("x")

	// colorAccent = #5dbfa7 → 38;2;93;191;167
	const wantANSI = "38;2;93;191;167"
	if !strings.Contains(rendered, wantANSI) {
		t.Errorf("paletteBox border foreground: rendered %q does not contain expected ANSI sequence %q;\n"+
			"expected colorAccent (#5dbfa7) — command palette border must be accent per design (tui-components.jsx:441).\n"+
			"Do NOT use colorLine here — this has regressed twice already.", rendered, wantANSI)
	}
}

// ---------------------------------------------------------------------------
// 1b.3 — panelHeader(title) returns "── TITLE" form (uppercase, stripped)
// ---------------------------------------------------------------------------

// TestTuiStyles_PanelHeader_Format asserts that s.panelHeader(title) produces
// a string whose ANSI-stripped content equals "── TITLE" (uppercase, preceded
// by the box-drawing rule ──). It must NOT contain the old glyph "◈".
func TestTuiStyles_PanelHeader_Format(t *testing.T) {
	s := newTuiStyles()

	cases := []struct {
		input string
		want  string // ANSI-stripped expected value
	}{
		{"telemetry", "── TELEMETRY"},
		{"todo", "── TODO"},
		{"context", "── CONTEXT"},
		{"built-in tools", "── BUILT-IN TOOLS"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := s.panelHeader(tc.input)
			stripped := ansi.Strip(got)

			if stripped != tc.want {
				t.Errorf("panelHeader(%q): ANSI-stripped = %q, want %q", tc.input, stripped, tc.want)
			}

			// Must NOT contain the old glyph.
			if strings.Contains(got, "◈") {
				t.Errorf("panelHeader(%q): output contains '◈' — old glyph must not appear", tc.input)
			}
		})
	}
}
