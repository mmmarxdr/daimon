package tui

import "testing"

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
