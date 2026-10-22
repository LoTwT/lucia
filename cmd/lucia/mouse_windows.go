//go:build windows

package main

import "syscall"

const vkLeftButton = 0x01

var getAsyncKeyState = syscall.NewLazyDLL("user32.dll").NewProc("GetAsyncKeyState")

// leftMouseButtonDown asks Win32 for the primary button state. MyGo exposes
// no drag-end event, so this is what lets the native drag finish before the
// pet is clamped back inside the work area.
func leftMouseButtonDown() (down, ok bool) {
	if err := getAsyncKeyState.Find(); err != nil {
		return false, false
	}
	r, _, _ := getAsyncKeyState.Call(vkLeftButton)
	return r&0x8000 != 0, true
}
