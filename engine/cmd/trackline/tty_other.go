//go:build !darwin && !linux && !windows

package main

import "errors"

// isTerminal has no reliable answer here, so it fails closed: no questions,
// the safe defaults.
func isTerminal(uintptr) bool { return false }

// readSecret is not done here yet, so a password is never read where it would show.
func readSecret() (string, error) {
	return "", errors.New("setting a send password needs macOS or Linux for now")
}
