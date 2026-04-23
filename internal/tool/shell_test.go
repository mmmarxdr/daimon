//go:build !windows

package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"daimon/internal/config"
)

func mkWhitelistShell(allowed ...string) *ShellTool {
	return NewShellTool(config.ShellToolConfig{
		Enabled:         true,
		AllowAll:        false,
		AllowedCommands: allowed,
	})
}

func mkAllowAllShell() *ShellTool {
	return NewShellTool(config.ShellToolConfig{Enabled: true, AllowAll: true})
}

func runShell(t *testing.T, s *ShellTool, cmd string) ToolResult {
	t.Helper()
	params, err := json.Marshal(shellParams{Command: cmd})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res, err := s.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute returned err: %v", err)
	}
	return res
}

func TestExecute_RejectsEmpty(t *testing.T) {
	res := runShell(t, mkWhitelistShell("echo"), "")
	if !res.IsError || !strings.Contains(res.Content, "empty") {
		t.Fatalf("expected empty error, got %+v", res)
	}
}

func TestExecute_RejectsNonWhitelisted(t *testing.T) {
	res := runShell(t, mkWhitelistShell("echo"), "cat /etc/passwd")
	if !res.IsError || !strings.Contains(res.Content, "not in the allowed list") {
		t.Fatalf("expected whitelist rejection, got %+v", res)
	}
}

func TestExecute_WhitelistedEchoSucceeds(t *testing.T) {
	res := runShell(t, mkWhitelistShell("echo"), "echo hello")
	if res.IsError {
		t.Fatalf("expected success, got %+v", res)
	}
	if !strings.Contains(res.Content, "hello") {
		t.Fatalf("expected output to contain 'hello', got %q", res.Content)
	}
}

func TestExecute_WhitelistedMultipleArgsSucceed(t *testing.T) {
	res := runShell(t, mkWhitelistShell("echo"), "echo hello world")
	if res.IsError {
		t.Fatalf("expected success, got %+v", res)
	}
	if !strings.Contains(res.Content, "hello world") {
		t.Fatalf("expected 'hello world' in output, got %q", res.Content)
	}
}

// TestExecute_WhitelistBypass_SemicolonIsRejected is the regression test for
// the RCE. Before the fix, `echo ok; touch <marker>` would pass the whitelist
// (baseCmd=echo) and then sh -c would run BOTH statements, creating <marker>.
// After the fix, the metachar guard rejects the input before any exec and the
// marker MUST NOT exist.
func TestExecute_WhitelistBypass_SemicolonIsRejected(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwn-marker")
	injected := fmt.Sprintf("echo ok; touch %s", marker)

	res := runShell(t, mkWhitelistShell("echo", "touch"), injected)

	if !res.IsError {
		t.Fatalf("expected rejection, got success: %+v", res)
	}
	if !strings.Contains(res.Content, "metacharacter") {
		t.Fatalf("expected metachar error, got %q", res.Content)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("RCE REGRESSED: injected `touch %s` was executed despite rejection error", marker)
	}
}

// TestExecute_WhitelistBypass_CommandSubIsRejected covers $(...) — another
// classic shell-injection vector. Uses a whitelisted base so we know the
// metachar guard (not the whitelist check) is what rejects it.
func TestExecute_WhitelistBypass_CommandSubIsRejected(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "pwn-cmdsub")
	injected := fmt.Sprintf("echo $(touch %s && echo done)", marker)

	res := runShell(t, mkWhitelistShell("echo", "touch"), injected)

	if !res.IsError {
		t.Fatalf("expected rejection, got success: %+v", res)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("RCE REGRESSED: $(...) executed despite rejection error")
	}
}

func TestExecute_RejectsAllShellMetachars(t *testing.T) {
	s := mkWhitelistShell("echo", "cat")
	cases := []struct {
		name string
		cmd  string
	}{
		{"semicolon", "echo a;echo b"},
		{"and_and", "echo a && echo b"},
		{"or_or", "echo a || echo b"},
		{"pipe", "echo a | cat"},
		{"redirect_out", "echo a > /tmp/does-not-matter"},
		{"redirect_in", "cat < /tmp/does-not-matter"},
		{"command_sub_dollar", "echo $(whoami)"},
		{"command_sub_backtick", "echo `whoami`"},
		{"brace_expand", "echo {a,b}"},
		{"background", "echo a &"},
		{"env_var", "echo $HOME"},
		{"newline", "echo a\necho b"},
		{"carriage_return", "echo a\rb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := runShell(t, s, tc.cmd)
			if !res.IsError {
				t.Fatalf("expected rejection for %q, got success: %+v", tc.cmd, res)
			}
			if !strings.Contains(res.Content, "metacharacter") {
				t.Fatalf("expected metachar error for %q, got %q", tc.cmd, res.Content)
			}
		})
	}
}

func TestExecute_AllowAll_MetacharsStillWork(t *testing.T) {
	s := mkAllowAllShell()

	chain := runShell(t, s, "echo a; echo b")
	if chain.IsError {
		t.Fatalf("chained commands should work with allow_all, got %+v", chain)
	}
	if !strings.Contains(chain.Content, "a") || !strings.Contains(chain.Content, "b") {
		t.Fatalf("expected both 'a' and 'b' in chained output, got %q", chain.Content)
	}

	pipe := runShell(t, s, "echo hello | cat")
	if pipe.IsError {
		t.Fatalf("pipes should work with allow_all, got %+v", pipe)
	}
	if !strings.Contains(pipe.Content, "hello") {
		t.Fatalf("expected 'hello' through pipe, got %q", pipe.Content)
	}
}

// TestFirstShellMetachar covers the helper directly so the reject set is
// pinned by a unit test, not only by integration cases above.
func TestFirstShellMetachar(t *testing.T) {
	reject := []struct {
		in   string
		want rune
	}{
		{"foo;bar", ';'},
		{"foo&bar", '&'},
		{"foo|bar", '|'},
		{"foo<bar", '<'},
		{"foo>bar", '>'},
		{"foo$bar", '$'},
		{"foo`bar", '`'},
		{"foo(bar", '('},
		{"foo)bar", ')'},
		{"foo{bar", '{'},
		{"foo}bar", '}'},
		{"foo\nbar", '\n'},
		{"foo\rbar", '\r'},
	}
	for _, tc := range reject {
		got, ok := firstShellMetachar(tc.in)
		if !ok || got != tc.want {
			t.Errorf("firstShellMetachar(%q) = (%q,%v), want (%q,true)", tc.in, got, ok, tc.want)
		}
	}

	benign := []string{
		"",
		"echo hello world",
		"ls -la /tmp",
		"cat foo.txt",
		"grep -r pattern src",
		// Globs, tildes, brackets, question marks are NOT rejected — they
		// are literal args when no shell is involved.
		"ls *.go",
		"cat ~/file",
		"cat foo[12].txt",
		"ls foo?bar",
	}
	for _, in := range benign {
		if _, ok := firstShellMetachar(in); ok {
			t.Errorf("firstShellMetachar(%q) flagged a benign input", in)
		}
	}
}
