package tui

import (
	"os"
	"testing"
)

// TestRunTUI_RejectsNonTTY verifies that RunTUI returns an error containing
// "requires a TTY" when stdin is not a terminal.
// In CI / test environments stdin is always non-TTY, so this test is always run.
func TestRunTUI_RejectsNonTTY(t *testing.T) {
	// Open /dev/null as a non-TTY file to guarantee non-terminal stdin in test.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open /dev/null: %v", err)
	}
	defer f.Close()

	err = runTUIWithStdin(nil, nil, nil, nil, f)
	if err == nil {
		t.Fatal("RunTUI returned nil error on non-TTY stdin, want error")
	}
	if !containsCI(err.Error(), "requires a TTY") {
		t.Errorf("error %q does not contain %q", err.Error(), "requires a TTY")
	}
}

// TestRequireTTY_RejectsNonTTY tests the requireTTY helper directly.
func TestRequireTTY_RejectsNonTTY(t *testing.T) {
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skipf("cannot open /dev/null: %v", err)
	}
	defer f.Close()

	if err := requireTTY(f); err == nil {
		t.Error("requireTTY(/dev/null) returned nil, want error")
	}
}

// containsCI is a case-insensitive substring check for test assertions.
func containsCI(s, sub string) bool {
	return len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if equalFold(s[i:i+len(sub)], sub) {
				return true
			}
		}
		return false
	}()
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}
