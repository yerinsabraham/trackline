//go:build !unix

package main

import "os/exec"

// inGroup: without process groups, stopping the agent stops the agent alone.
func inGroup(cmd *exec.Cmd) {}
