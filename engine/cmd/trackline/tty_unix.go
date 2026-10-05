//go:build darwin || linux

package main

import (
	"syscall"
	"unsafe"
)

// isTerminal asks the terminal driver itself, as golang.org/x/term does:
// /dev/null, /dev/zero and pipes all refuse the request, so it fails closed.
func isTerminal(fd uintptr) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlGetTermios, uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
