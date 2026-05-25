package main

import (
	"os"
	"testing"
)

// TestRunTUICommand_RejectsNonTTY verifies that runTUICommand returns a non-nil
// error when stdin is not a TTY, matching the TTY guard in RunTUI.
// In CI / test environments stdin is always a non-TTY pipe, so this always runs.
func TestRunTUICommand_RejectsNonTTY(t *testing.T) {
	// Use /dev/null as explicit non-TTY stdin.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open /dev/null: %v", err)
	}
	defer f.Close()

	// Redirect stdin to /dev/null for the duration of this test.
	orig := os.Stdin
	os.Stdin = f
	defer func() { os.Stdin = orig }()

	err = runTUICommand([]string{}, "")
	if err == nil {
		t.Fatal("runTUICommand returned nil error on non-TTY stdin, want error")
	}
}
