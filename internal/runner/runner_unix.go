//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts the child into its own process group so that
// terminateGroup / killProcessGroup can signal the whole tree in one
// call. Required because children (Flask reloader, Python subprocess
// helpers) outlive the immediate child on a bare Kill to the leader.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// terminateGroup sends SIGTERM to the process group led by cmd.
func terminateGroup(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
}

// killProcessGroup sends SIGKILL to the process group led by cmd.
func killProcessGroup(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
