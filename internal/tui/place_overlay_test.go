package tui

import (
	"strings"
	"testing"
)

// TestPlaceOverlay_BoxAppearsAtCenter verifies that placeOverlay places the box
// lines on top of the base at the computed center position.
// Base: 10 rows of 20 'X' chars. Box: 2 rows of 4 chars ("AB\nCD").
// Expected: box appears at center; corners of base still contain 'X'.
func TestPlaceOverlay_BoxAppearsAtCenter(t *testing.T) {
	termW, termH := 20, 10

	// Build base: 10 rows of exactly 20 'X' chars.
	baseLines := make([]string, termH)
	for i := range baseLines {
		baseLines[i] = strings.Repeat("X", termW)
	}
	base := strings.Join(baseLines, "\n")

	// Box: 2 rows, 4 visible cols.
	box := "AB\nCD"

	result := placeOverlay(base, box, termW, termH)

	// Result must contain box content.
	if !strings.Contains(result, "AB") {
		t.Errorf("placeOverlay result must contain box line 'AB'\nresult:\n%s", result)
	}
	if !strings.Contains(result, "CD") {
		t.Errorf("placeOverlay result must contain box line 'CD'\nresult:\n%s", result)
	}

	// Result must still contain 'X' from the base (base NOT discarded).
	if !strings.Contains(result, "X") {
		t.Errorf("placeOverlay result must retain base 'X' content (base not discarded)\nresult:\n%s", result)
	}
}

// TestPlaceOverlay_BaseNotFullyReplaced verifies that the base content is NOT
// entirely overwritten — base chars appear outside the box rectangle.
func TestPlaceOverlay_BaseNotFullyReplaced(t *testing.T) {
	termW, termH := 20, 10
	baseLines := make([]string, termH)
	for i := range baseLines {
		baseLines[i] = strings.Repeat("X", termW)
	}
	base := strings.Join(baseLines, "\n")

	// Use a 4x2 box.
	box := "AABB\nCCDD"

	result := placeOverlay(base, box, termW, termH)
	lines := strings.Split(result, "\n")

	// The first line must begin with 'X' (before center offset).
	if len(lines) > 0 && len(lines[0]) > 0 && lines[0][0] != 'X' {
		t.Errorf("line[0] should start with 'X' (base content), got: %q", lines[0])
	}
	// The last line must begin with 'X'.
	last := lines[len(lines)-1]
	if len(last) > 0 && last[0] != 'X' {
		t.Errorf("last line should start with 'X' (base content), got: %q", last)
	}
}

// TestPlaceOverlay_EmptyBoxReturnsBase verifies that an empty box string
// returns the base unchanged.
func TestPlaceOverlay_EmptyBoxReturnsBase(t *testing.T) {
	base := "HELLO WORLD"
	result := placeOverlay(base, "", 80, 24)
	if result != base {
		t.Errorf("placeOverlay with empty box: want base unchanged, got %q", result)
	}
}

// TestModel_View_WithPalette_ContainsBothPaletteAndBaseContent verifies that
// View() with an active palette contains BOTH palette content AND base/chat content.
func TestModel_View_WithPalette_ContainsBothPaletteAndBaseContent(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenChat
	m.focus = focusEditor

	// Add a thread item so base has visible chat content.
	m.thread.append(&MsgDaimon{text: "CHAT_BASE_MARKER", styles: m.styles})

	// Push a palette.
	m.overlays.Push(newCommandPalette(testCmds, newTuiStyles()))

	rendered := m.View()

	// Must contain palette marker.
	if !strings.Contains(rendered, "commands") {
		t.Errorf("View() with active palette must contain 'commands' palette title\nrendered:\n%s", rendered)
	}

	// Must ALSO contain base content (not discarded).
	if !strings.Contains(rendered, "CHAT_BASE_MARKER") {
		t.Errorf("View() with active palette must contain base chat content 'CHAT_BASE_MARKER' (not discarded)\nrendered:\n%s", rendered)
	}
}
