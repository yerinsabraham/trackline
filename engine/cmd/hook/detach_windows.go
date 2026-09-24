//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// detach starts the sender without a console and outside the hook's process
// group, so it outlives the hook and opens no window.
func detach(cmd *exec.Cmd) {
	const detachedProcess = 0x00000008
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: detachedProcess | syscall.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
}
