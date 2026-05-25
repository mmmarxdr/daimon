package main

import (
	"strings"
	"testing"
)

// TestRunTUICommand_MissingConfig verifies that runTUICommand returns a
// descriptive error when no config file is found (the most common failure
// mode for new users). The TTY guard is hit AFTER config load, so this test
// exercises the config-not-found path, not the TTY path.
//
// The real TTY-guard test lives in internal/tui/run_test.go (TestRunTUI_RejectsNonTTY)
// which tests runTUIWithStdin directly with a /dev/null stdin.
func TestRunTUICommand_MissingConfig(t *testing.T) {
	err := runTUICommand([]string{}, "")
	if err == nil {
		t.Fatal("runTUICommand returned nil error with no config, want error")
	}
	// Must mention config / setup so the user knows how to fix it.
	msg := err.Error()
	if !strings.Contains(msg, "config") && !strings.Contains(msg, "setup") {
		t.Errorf("error %q does not mention 'config' or 'setup' — want a helpful config-missing message", msg)
	}
}
