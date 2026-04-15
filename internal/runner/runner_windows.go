//go:build windows

package runner

import "os/exec"

// Windows has no POSIX process groups and no SIGTERM/SIGKILL split. We
// fall back to exec.Cmd.Process.Kill() which maps to TerminateProcess
// — abrupt but fine for the runner's use case (we are killing an
// agent that is either a short-lived oneshot or a long-running HTTP
// server we explicitly want torn down).
func setProcessGroup(cmd *exec.Cmd) {
	// no-op
}

func terminateGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

func killProcessGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
