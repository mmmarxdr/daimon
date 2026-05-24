package agent

// commands_new_builtins.go — New built-in command handlers added in PR2+.
//
// Contains:
//   - cmdCancel   (WU4, REQ-6): cancel the in-progress LLM turn for a (channel, sender)
//   - IsDestructiveCommand (WU4, REQ-17): authoritative destructive-command table
//   - cmdCd       (WU5, REQ-5, REQ-23): per-(channel, sender) shell cwd override

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ---------------------------------------------------------------------------
// Destructive command table (REQ-17, Decision #12)
//
// Placed in the agent package so *Agent.Commands() can populate
// CommandInfo.Destructive without importing the web layer.
// ---------------------------------------------------------------------------

// destructiveCommands is the authoritative list of commands that require
// allow_destructive=true when invoked via the REST /api/commands/run endpoint.
// The list matches the spec REQ-17 table exactly.
var destructiveCommands = map[string]bool{
	"reset":               true,
	"fork":                true,
	"save":                true,
	"cancel":              true,
	"task-cancel":         true,
	"task-cancel-confirm": true,
	"cd":                  true,
	"export":              true,
	"resume":              true,
	"schedule":            true,
	"retry":               true,
	"compact":             true,
}

// IsDestructiveCommand reports whether name is in the destructive-command set.
// Used by the REST handler layer (handler_commands.go) to gate allow_destructive.
func IsDestructiveCommand(name string) bool {
	return destructiveCommands[strings.ToLower(name)]
}

// ---------------------------------------------------------------------------
// /cancel handler (WU4, REQ-6)
// ---------------------------------------------------------------------------

// cmdCancel is the /cancel built-in command handler. It cancels the in-progress
// LLM turn for the (channel, sender) pair that sent the command.
//
// If a turn is in progress: calls a.cancels.Cancel(key) → replies confirmation.
// If no turn is in progress (idempotent): replies neutral message.
//
// /cancel is registered as a closure (method on *Agent) so it can access
// a.cancels, following the same pattern as cmdCompact.
func (a *Agent) cmdCancel(cc CommandContext) error {
	key := cancelKey{ChannelID: cc.ChannelID, SenderID: cc.SenderID}
	if a.cancels.Cancel(key) {
		cc.Reply("Turn cancellation requested.")
	} else {
		cc.Reply("No turn in progress.")
	}
	return nil
}

// ---------------------------------------------------------------------------
// /cd handler (WU5, REQ-5, REQ-23)
// ---------------------------------------------------------------------------

// cmdCd implements the /cd built-in command for per-(channel, sender) shell
// working directory overrides.
//
// Reply format (Decision #7):
//   - No-arg or "~": reset to default → reply "Working directory: <defaultCwd>"
//   - Valid path set  → reply "Changed working directory to: <path>"
//   - Error           → reply "cd: <description>"; no state change
//
// Sandbox enforcement (REQ-23):
//   - Rejects any path containing ".." after filepath.Clean (traversal guard).
//   - If a.sandboxRoot is non-empty (set at construction time from the
//     deployer-configured file-tool base path), the resolved path must be
//     inside it.
func (a *Agent) cmdCd(cc CommandContext) error {
	key := cancelKey{ChannelID: cc.ChannelID, SenderID: cc.SenderID}

	arg := strings.TrimSpace(cc.Args)

	// No-arg or "~" → reset to default cwd.
	if arg == "" || arg == "~" {
		a.shellCwd.Reset(key)
		defaultCwd := a.shellCwd.DefaultCwd()
		cc.Reply("Working directory: " + defaultCwd)
		return nil
	}

	// Reject paths containing ".." before any resolution (REQ-23).
	// filepath.Clean normalises ".." traversals in-place; if the result
	// still contains ".." the path tried to escape the current dir.
	cleaned := filepath.Clean(arg)
	if strings.Contains(cleaned, "..") {
		cc.Reply("cd: path must not contain '..' traversal components")
		return nil
	}

	// Resolve symlinks for a canonical absolute path.
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		if os.IsNotExist(err) {
			cc.Reply(fmt.Sprintf("cd: %s: no such file or directory", cleaned))
		} else {
			cc.Reply(fmt.Sprintf("cd: %s: %v", cleaned, err))
		}
		return nil
	}

	// Must be a directory.
	info, err := os.Stat(resolved)
	if err != nil {
		cc.Reply(fmt.Sprintf("cd: %s: %v", resolved, err))
		return nil
	}
	if !info.IsDir() {
		cc.Reply(fmt.Sprintf("cd: %s: not a directory", resolved))
		return nil
	}

	// Sandbox check (REQ-23): if a sandbox root is configured, the resolved
	// path must be inside it.
	if sandboxRoot := a.shellCwd.SandboxRoot(); sandboxRoot != "" {
		resolvedSandbox, err := filepath.EvalSymlinks(sandboxRoot)
		if err != nil {
			resolvedSandbox = sandboxRoot
		}
		if !strings.HasPrefix(resolved, resolvedSandbox) {
			cc.Reply(fmt.Sprintf("cd: %s: outside sandbox boundary", resolved))
			return nil
		}
	}

	// All checks passed — store the override.
	if err := a.shellCwd.Set(key, resolved); err != nil {
		cc.Reply(fmt.Sprintf("cd: failed to set working directory: %v", err))
		return nil
	}

	cc.Reply("Changed working directory to: " + resolved)
	return nil
}
