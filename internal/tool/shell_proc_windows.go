//go:build windows

package tool

import "os/exec"

// setProcessGroup is a no-op on Windows — process groups work differently and
// exec.CommandContext + Job Objects (used internally by the runtime) handle
// child lifecycle reasonably for the daimon use case.
func setProcessGroup(_ *exec.Cmd) {}

// killProcessGroup falls back to killing the immediate process. CommandContext
// already does this when the context is cancelled; this helper exists so the
// caller can be platform-agnostic.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
