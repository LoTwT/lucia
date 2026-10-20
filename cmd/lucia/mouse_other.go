//go:build !darwin

package main

// leftMouseButtonDown has no cross-platform MyGo API yet. Windows and Linux
// fall back to the timed check in the window callback; their drag behavior is
// validated in the platform follow-up.
func leftMouseButtonDown() (down, ok bool) { return false, false }
