//go:build !darwin && !windows

package main

// leftMouseButtonDown has no MyGo API on this platform. Linux falls back to
// the timed check in the window callback; Linux is not part of the current
// release matrix.
func leftMouseButtonDown() (down, ok bool) { return false, false }
