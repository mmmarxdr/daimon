package agent

import (
	"context"
	"testing"
)

// TestRunCommand_Destructive_BlockedWithoutFlag verifies that RunCommand returns
// an error when a destructive command is invoked with AllowDestructive=false.
// The handler must NOT be executed.
func TestRunCommand_Destructive_BlockedWithoutFlag(t *testing.T) {
	st := &mockStore{}
	ag := makeAgentWithStore(t, st)

	// "reset" is a known destructive command (IsDestructiveCommand returns true).
	// With AllowDestructive=false it must be rejected BEFORE the handler runs.
	_, err := ag.RunCommand(context.Background(), RunCommandRequest{
		Name:             "reset",
		Args:             "",
		ChannelID:        "tui",
		SenderID:         "local_user",
		AllowDestructive: false,
	})
	if err == nil {
		t.Fatal("RunCommand with destructive command + AllowDestructive=false must return an error")
	}

	// Verify the error message is informative.
	if !containsAny(err.Error(), "destructive", "AllowDestructive", "allow_destructive") {
		t.Errorf("error message %q should mention destructive/AllowDestructive", err.Error())
	}

	// The store must NOT have been written — handler did not execute.
	st.mu.Lock()
	saved := st.conv
	st.mu.Unlock()
	if saved != nil {
		t.Error("handler must NOT have executed: store was written despite rejection")
	}
}

// TestRunCommand_Destructive_AllowedWithFlag verifies that RunCommand proceeds
// normally when a destructive command is invoked with AllowDestructive=true.
func TestRunCommand_Destructive_AllowedWithFlag(t *testing.T) {
	st := &mockStore{}
	ag := makeAgentWithStore(t, st)

	result, err := ag.RunCommand(context.Background(), RunCommandRequest{
		Name:             "reset",
		Args:             "",
		ChannelID:        "tui",
		SenderID:         "local_user",
		AllowDestructive: true,
	})
	if err != nil {
		t.Fatalf("RunCommand with destructive command + AllowDestructive=true must succeed, got: %v", err)
	}
	// reset should reply with confirmation text.
	if result.Reply == "" {
		t.Error("expected non-empty reply from reset command")
	}
}

// TestRunCommand_NonDestructive_AlwaysPasses verifies that non-destructive
// commands are not affected by the destructive gate (whether flag is true or false).
func TestRunCommand_NonDestructive_AlwaysPasses(t *testing.T) {
	st := &mockStore{}
	ag := makeAgentWithStore(t, st)

	for _, allow := range []bool{false, true} {
		result, err := ag.RunCommand(context.Background(), RunCommandRequest{
			Name:             "ping",
			Args:             "",
			ChannelID:        "tui",
			SenderID:         "local_user",
			AllowDestructive: allow,
		})
		if err != nil {
			t.Errorf("RunCommand(ping, allow=%v): unexpected error: %v", allow, err)
		}
		if result.Reply == "" {
			t.Errorf("RunCommand(ping, allow=%v): expected non-empty reply", allow)
		}
	}
}

// containsAny reports whether s contains at least one of the substrings.
func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
