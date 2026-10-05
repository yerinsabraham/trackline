package main

import "syscall"

// isTerminal is true only for a console, which is where a person can answer.
func isTerminal(fd uintptr) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(fd), &mode) == nil
}
