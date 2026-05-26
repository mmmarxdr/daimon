package main

import "testing"

// TestDefaultToTUI verifies the rule for launching the power-user TUI on a bare
// `daimon` invocation: TUI is the default ONLY in an interactive terminal and
// ONLY when --web is not set. Non-interactive runs (services/bots/piped stdin)
// and `daimon --web` (agent + web dashboard) fall through to the channel-agent
// path instead.
func TestDefaultToTUI(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		webFlag     bool
		want        bool
	}{
		{"interactive terminal, no --web -> TUI", true, false, true},
		{"interactive terminal, --web -> channel agent + web", true, true, false},
		{"non-interactive (service/bot), no --web -> channel agent", false, false, false},
		{"non-interactive, --web -> channel agent + web", false, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultToTUI(tt.interactive, tt.webFlag); got != tt.want {
				t.Errorf("defaultToTUI(interactive=%v, web=%v) = %v, want %v",
					tt.interactive, tt.webFlag, got, tt.want)
			}
		})
	}
}
