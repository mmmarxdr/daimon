//go:build !windows

package tool

import (
	"os/exec"
	"syscall"
)

// setProcessGroup configures the child to live in its own process group so a
// timeout kill can reach grandchildren spawned by `sh -c "...|...&"`. Without
// this, exec.CommandContext only signals the immediate child PID; pipes,
// background jobs, and subshells survive the timeout.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup sends SIGKILL to the entire process group of cmd. Returns
// nil when there is no live process. Mirrors the shell `kill -KILL -<pgid>`
// form (the leading `-` makes Linux interpret the target as a group ID).
//
// Best-effort: if the process is in uninterruptible sleep (D-state, common on
// WSL stat'ing NTFS mounts), even SIGKILL won't return until the syscall
// finishes. The watchdog in shell.go bounds the wait so the agent never blocks.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	// Negative PID = whole group.
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
