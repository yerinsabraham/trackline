//go:build !darwin && !linux && !windows

package main

// isTerminal has no reliable answer here, so it fails closed: no questions,
// the safe defaults.
func isTerminal(uintptr) bool { return false }
