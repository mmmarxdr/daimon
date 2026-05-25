package tui

// layout_test.go — tests for centerText and renderWelcomeCenter layout math.

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestCenterText_ANSIStyledString verifies that centerText correctly centers
// a string containing ANSI escape sequences by measuring visible width, not
// raw byte/rune length. With ANSI escape sequences len([]rune(s)) over-counts
// the string width, producing too-small padding and off-centre output.
//
// RED: before the fix, len([]rune(styled)) = 30 (counts escape runes) but
// ansi.StringWidth = 8 (visible "⫶ daimon"), so the leading pad is 25 instead
// of the correct 36 — text appears 11 columns too far left on real terminals.
func TestCenterText_ANSIStyledString(t *testing.T) {
	// Use a manually crafted ANSI escape to simulate what lipgloss produces on
	// a real TTY (lipgloss strips escapes in non-terminal test contexts).
	// This is the accent color (#5dbfa7) wrapping "⫶ daimon".
	styled := "\x1b[38;2;93;191;167m⫶ daimon\x1b[0m"
	const width = 80

	line := centerText(styled, width)

	// Strip ANSI escapes from the result to count visible leading spaces.
	visible := ansi.Strip(line)

	// The correct pad is based on visible character width (8), not rune count (30).
	visibleContent := ansi.StringWidth(styled) // = 8
	wantPad := (width - visibleContent) / 2    // = 36

	leadingSpaces := 0
	for _, ch := range visible {
		if ch != ' ' {
			break
		}
		leadingSpaces++
	}

	if leadingSpaces != wantPad {
		t.Errorf("centerText with ANSI-styled string: leading spaces = %d, want %d\n"+
			"raw rune len = %d, visible width = %d, width = %d\nline = %q",
			leadingSpaces, wantPad, len([]rune(styled)), visibleContent, width, line)
	}
}

// TestCenterText_PlainString verifies the plain (no ANSI) path still works.
func TestCenterText_PlainString(t *testing.T) {
	const width = 80
	s := "hello"
	line := centerText(s, width)
	visible := ansi.Strip(line)

	wantPad := (width - len(s)) / 2
	leadingSpaces := 0
	for _, ch := range visible {
		if ch != ' ' {
			break
		}
		leadingSpaces++
	}
	if leadingSpaces != wantPad {
		t.Errorf("centerText plain: leading spaces = %d, want %d", leadingSpaces, wantPad)
	}
}

// TestRenderWelcomeCenter_WidthRespected verifies the welcome screen logo line
// does not exceed the given terminal width (measured with ansi.StringWidth).
func TestRenderWelcomeCenter_WidthRespected(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24

	center := renderWelcomeCenter(m, 80, 20)
	for i, line := range strings.Split(center, "\n") {
		w := ansi.StringWidth(line)
		if w > 80 {
			t.Errorf("renderWelcomeCenter line %d visible width = %d > 80: %q", i, w, line)
		}
	}
}
