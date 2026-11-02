package main

import (
	"errors"
	"syscall"
)

// isTerminal is true only for a console, which is where a person can answer.
func isTerminal(fd uintptr) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(fd), &mode) == nil
}

// readSecret is not done here yet, so a password is never read where it would show.
func readSecret() (string, error) {
	return "", errors.New("setting a send password needs macOS or Linux for now")
}
