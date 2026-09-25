//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// inGroup starts the agent in its own process group and stops the whole
// group, so the shells, test runners and servers an agent starts stop with it.
func inGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM) }
}
