package tui

// tui_test.go — package-level test infrastructure: update flag for golden files
// and smoke import of teatest to ensure the dependency is present.

import (
	"testing"

	// teatest must be importable — task 1.1/1.2 gate.
	_ "github.com/charmbracelet/x/exp/teatest"
)

// TestPackage_Imports verifies the package compiles with teatest present.
func TestPackage_Imports(t *testing.T) {
	// If this file compiles, teatest is available.
	t.Log("teatest dependency present")
}
