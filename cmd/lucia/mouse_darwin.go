//go:build darwin

package main

import (
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/ebitengine/purego"
)

// leftMouseButtonDown reports whether the primary mouse button is held. The
// second result is false when the native state is unavailable; callers then
// fall back to a timed check.
//
// MyGo exposes no drag-end event, so this asks AppKit directly. A platform
// helper here keeps the window on-screen after a drag without guessing from
// move events, which cannot distinguish a held button from a released one.
func leftMouseButtonDown() (down, ok bool) {
	mouseButton.once.Do(mouseButton.load)
	if mouseButton.msgSend == 0 || mouseButton.eventClass == 0 || mouseButton.selector == 0 {
		return false, false
	}
	r, _, _ := purego.SyscallN(mouseButton.msgSend, mouseButton.eventClass, mouseButton.selector)
	return r&1 != 0, true
}

var mouseButton mouseButtonState

type mouseButtonState struct {
	once       sync.Once
	msgSend    uintptr
	eventClass uintptr
	selector   uintptr
}

func (m *mouseButtonState) load() {
	lib, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_NOW)
	if err != nil {
		return
	}
	msgSend, err := purego.Dlsym(lib, "objc_msgSend")
	if err != nil {
		return
	}
	getClass, err := purego.Dlsym(lib, "objc_getClass")
	if err != nil {
		return
	}
	registerName, err := purego.Dlsym(lib, "sel_registerName")
	if err != nil {
		return
	}
	className, err := syscall.BytePtrFromString("NSEvent")
	if err != nil {
		return
	}
	selectorName, err := syscall.BytePtrFromString("pressedMouseButtons")
	if err != nil {
		return
	}
	class, _, _ := purego.SyscallN(getClass, uintptr(unsafe.Pointer(className)))
	selector, _, _ := purego.SyscallN(registerName, uintptr(unsafe.Pointer(selectorName)))
	runtime.KeepAlive(className)
	runtime.KeepAlive(selectorName)
	m.msgSend, m.eventClass, m.selector = msgSend, class, selector
}
