package main

import "testing"

// TestDefaultToTUI verifies the rule for launching the power-user TUI on a bare
// `daimon` invocation. The TUI is the default ONLY when ALL hold:
//   - interactive: both stdin AND stdout are terminals (the caller passes
//     isTTY(os.Stdin) && isTTY(os.Stdout) — gating on stdin alone would let a
//     redirected-stdout run like `daimon > file` dump alt-screen escapes into
//     the file);
//   - --web is NOT set (that's agent + web dashboard mode);
//   - --daemon is NOT set (headless background daemon, no interactive channel);
//   - no leftover positional args after flag parsing (hasPositionalArgs) — only
//     a truly bare invocation defaults to the TUI; `daimon tuii` (an unknown
//     subcommand) must fall through to the channel-agent path, not silently
//     launch the TUI.
//
// Any of those failing falls through to the channel-agent path instead.
//
// The table is the EXHAUSTIVE truth table for the 4-input predicate (all 2^4
// combinations): only the fully-bare interactive row yields true, so any
// accidental flip in the boolean chain is caught on every input path.
func TestDefaultToTUI(t *testing.T) {
	tests := []struct {
		name              string
		interactive       bool
		webFlag           bool
		daemonFlag        bool
		hasPositionalArgs bool
		want              bool
	}{
		// interactive (both stdin+stdout are terminals).
		{"bare interactive, no flags, no args -> TUI", true, false, false, false, true},
		{"interactive, --web -> channel agent + web", true, true, false, false, false},
		{"interactive, --daemon -> headless daemon", true, false, true, false, false},
		{"interactive, --web --daemon -> daemon path", true, true, true, false, false},
		{"interactive, positional args -> not bare, channel agent", true, false, false, true, false},
		{"interactive, --web + positional args", true, true, false, true, false},
		{"interactive, --daemon + positional args", true, false, true, true, false},
		{"interactive, --web --daemon + positional args", true, true, true, true, false},
		// non-interactive (piped/redirected stdin or stdout).
		{"non-interactive, no flags -> channel agent", false, false, false, false, false},
		{"non-interactive, --web -> channel agent + web", false, true, false, false, false},
		{"non-interactive, --daemon -> headless daemon", false, false, true, false, false},
		{"non-interactive, --web --daemon -> daemon path", false, true, true, false, false},
		{"non-interactive, positional args", false, false, false, true, false},
		{"non-interactive, --web + positional args", false, true, false, true, false},
		{"non-interactive, --daemon + positional args", false, false, true, true, false},
		{"non-interactive, --web --daemon + positional args", false, true, true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultToTUI(tt.interactive, tt.webFlag, tt.daemonFlag, tt.hasPositionalArgs); got != tt.want {
				t.Errorf("defaultToTUI(interactive=%v, web=%v, daemon=%v, hasPositionalArgs=%v) = %v, want %v",
					tt.interactive, tt.webFlag, tt.daemonFlag, tt.hasPositionalArgs, got, tt.want)
			}
		})
	}
}
