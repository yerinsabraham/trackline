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

// endGroup stops whatever the agent left running when it finished by itself.
// Nothing signals a group on a normal exit, and a process left behind can
// rewrite the project's settings after the job's checks have passed (shown in
// the review of 2026-10-31). A process that left the group with setsid gets
// past this, which is why it narrows the gap rather than closing it.
func endGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
