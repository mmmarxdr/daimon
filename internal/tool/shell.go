package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"os/user"
	"strconv"
	"strings"
	"time"

	"daimon/internal/config"
)

type ShellTool struct {
	config config.ShellToolConfig
}

func NewShellTool(cfg config.ShellToolConfig) *ShellTool {
	return &ShellTool{config: cfg}
}

func (t *ShellTool) Name() string {
	return "shell_exec"
}

func (t *ShellTool) Description() string {
	desc := "Execute a shell command on the host system. Only whitelisted commands are allowed unless allow_all is true in config."
	if t.config.WorkingDir != "" {
		desc += fmt.Sprintf(" Working directory: %s.", t.config.WorkingDir)
	}
	return desc
}

func (t *ShellTool) Schema() json.RawMessage {
	return json.RawMessage(`{
  "type": "object",
  "properties": {
    "command": { "type": "string", "description": "The command to execute (e.g., 'ls -la /tmp')" }
  },
  "required": ["command"]
}`)
}

type shellParams struct {
	Command string `json:"command"`
}

// hardKillGrace is how long we wait after SIGKILL'ing the process group before
// returning to the caller anyway. Bounds the worst case for processes stuck in
// uninterruptible sleep (D-state) — common when reading from network filesystems
// or NTFS mounts on WSL.
const hardKillGrace = 500 * time.Millisecond

func (t *ShellTool) Execute(ctx context.Context, params json.RawMessage) (ToolResult, error) {
	var input shellParams
	if err := json.Unmarshal(params, &input); err != nil {
		return ToolResult{}, fmt.Errorf("parsing params: %w", err)
	}

	cmdStr := strings.TrimSpace(input.Command)
	if cmdStr == "" {
		return ToolResult{IsError: true, Content: "command cannot be empty"}, nil
	}

	parts := strings.Fields(cmdStr)
	baseCmd := parts[0]

	if !t.config.AllowAll {
		allowed := false
		for _, ac := range t.config.AllowedCommands {
			if ac == baseCmd {
				allowed = true
				break
			}
		}
		if !allowed {
			return ToolResult{IsError: true, Content: fmt.Sprintf("Command '%s' is not in the allowed list", baseCmd)}, nil
		}
	}

	cmd := exec.Command("sh", "-c", cmdStr) //nolint:gosec // shell exec is the contract
	setProcessGroup(cmd)
	if t.config.WorkingDir != "" {
		wd := t.config.WorkingDir
		if strings.HasPrefix(wd, "~") {
			if usr, err := user.Current(); err == nil {
				wd = strings.Replace(wd, "~", usr.HomeDir, 1)
			}
		}
		cmd.Dir = wd
	}

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if err := cmd.Start(); err != nil {
		return ToolResult{
			IsError: true,
			Content: fmt.Sprintf("Failed to start command: %v", err),
			Meta:    map[string]string{"command": cmdStr, "exit_code": "-1"},
		}, nil
	}

	waitErr := waitWithDeadline(ctx, cmd)

	out := buf.Bytes()
	const maxLen = 64 * 1024
	outStr := string(out)
	if len(outStr) > maxLen {
		originalLen := len(outStr)
		outStr = outStr[:maxLen] + fmt.Sprintf("\n...(output truncated — showing first %d of %d bytes)", maxLen, originalLen)
	}

	if waitErr != nil {
		if errors.Is(waitErr, context.DeadlineExceeded) || errors.Is(waitErr, context.Canceled) {
			return ToolResult{
				IsError: true,
				Content: "Tool timed out (process group killed)",
				Meta:    map[string]string{"command": cmdStr, "exit_code": "-1"},
			}, nil
		}
		exitCode := "-1"
		if cmd.ProcessState != nil {
			exitCode = strconv.Itoa(cmd.ProcessState.ExitCode())
		}
		return ToolResult{
			IsError: true,
			Content: fmt.Sprintf("Command failed: %v\nOutput: %s", waitErr, outStr),
			Meta:    map[string]string{"command": cmdStr, "exit_code": exitCode},
		}, nil
	}

	if len(outStr) == 0 {
		outStr = "(command successful, no output)"
	}

	return ToolResult{Content: outStr, Meta: map[string]string{"command": cmdStr, "exit_code": "0"}}, nil
}

// waitWithDeadline runs cmd.Wait in a goroutine and races it against ctx.Done.
// On context cancellation it SIGKILLs the entire process group via
// killProcessGroup and gives Wait up to hardKillGrace to drain pipes; if Wait
// stays blocked past that (D-state child), the function returns ctx.Err()
// anyway so the agent loop is never held hostage by a stuck child.
func waitWithDeadline(ctx context.Context, cmd *exec.Cmd) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if err := killProcessGroup(cmd); err != nil {
			slog.Debug("shell: kill process group (best-effort)", "error", err)
		}
		select {
		case <-done:
			// Process exited cleanly after the kill; surface the timeout cause anyway.
		case <-time.After(hardKillGrace):
			// Child stuck in uninterruptible sleep. Stop waiting.
			slog.Warn("shell: process did not exit within grace period after SIGKILL — likely D-state",
				"grace", hardKillGrace, "pid", cmd.Process.Pid)
		}
		return ctx.Err()
	}
}
