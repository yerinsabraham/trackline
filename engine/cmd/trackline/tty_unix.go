//go:build darwin || linux

package main

import (
	"bufio"
	"errors"
	"os"
	"strings"
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

// readSecret reads one line from the terminal without showing it.
func readSecret() (string, error) {
	fd := os.Stdin.Fd()
	var t syscall.Termios
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlGetTermios, uintptr(unsafe.Pointer(&t))); e != 0 {
		return "", errors.New("not a terminal")
	}
	quiet := t
	quiet.Lflag &^= syscall.ECHO
	quiet.Lflag |= syscall.ICANON | syscall.ISIG
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlSetTermios, uintptr(unsafe.Pointer(&quiet))); e != 0 {
		return "", errors.New("could not hide what is typed")
	}
	defer syscall.Syscall(syscall.SYS_IOCTL, fd, ioctlSetTermios, uintptr(unsafe.Pointer(&t)))
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	os.Stdout.WriteString("\n")
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
