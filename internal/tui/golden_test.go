package tui

import (
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// TestModel_View_WelcomeScreen_Golden verifies that the welcome screen renders
// consistently at 80x24. Run with -update to regenerate the golden file.
//
// The golden package stores the file at testdata/<TestName>.golden.
// After generation via -update, re-run without -update to assert stability.
func TestModel_View_WelcomeScreen_Golden(t *testing.T) {
	m := newTestModel()
	m.width = 80
	m.height = 24
	m.screen = screenWelcome
	m.topBar.SetData("⫶", "/home/user/project", "main", "claude-3-5", "build", "$0.00", "ready")
	m.footer.SetScreen(screenWelcome)
	// WU-a: renderLayout now reads m.mode (cached field) instead of the live
	// modeAgent. Set m.mode to match the value passed to SetData so the golden
	// output is byte-identical — the rendered content is unchanged, only the
	// source of truth moved from topBar.mode to m.mode.
	m.mode = "build"

	got := m.View()

	// golden.RequireEqual writes testdata/TestModel_View_WelcomeScreen_Golden.golden
	// when -update is passed, otherwise compares against the stored file.
	golden.RequireEqual(t, []byte(got))
}
