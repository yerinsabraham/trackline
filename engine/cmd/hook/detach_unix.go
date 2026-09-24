//go:build !windows

package main

import (
	"os/exec"
	"syscall"
)

// detach puts the sender in its own session, so it outlives the hook and is
// not caught by a signal the host sends to the hook's process group.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
