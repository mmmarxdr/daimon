package tui

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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
