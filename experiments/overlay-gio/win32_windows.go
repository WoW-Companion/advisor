//go:build windows

package main

import (
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	procRegisterHotKey             = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey           = user32.NewProc("UnregisterHotKey")
	procGetMessageW                = user32.NewProc("GetMessageW")
	procMessageBoxW                = user32.NewProc("MessageBoxW")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procGetWindowLongW             = user32.NewProc("GetWindowLongW")
	procSetWindowLongW             = user32.NewProc("SetWindowLongW")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
)

var gwlExStyle = int32(-20)

const (
	modControl   = 0x0002
	modShift     = 0x0004
	modNoRepeat  = 0x4000
	wmHotkey     = 0x0312
	wsExLayered  = 0x00080000
	wsExTranspar = 0x00000020
	lwaAlpha     = 0x2
	swHide       = 0
	swShowNA     = 8
	vkUp         = 0x26
	vkDown       = 0x28
)

// Hotkey ids, also the order of hotkeys.
const (
	hkToggleShow = iota + 1
	hkClickThrough
	hkOpacityUp
	hkOpacityDown
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

// runHotkeys registers the global hotkeys on its own OS thread and calls on for each press. It
// blocks, so run it in a goroutine.
func runHotkeys(on func(id int)) {
	runtime.LockOSThread()
	keys := map[int]uintptr{hkToggleShow: 'W', hkClickThrough: 'T', hkOpacityUp: vkUp, hkOpacityDown: vkDown}
	for id, vk := range keys {
		procRegisterHotKey.Call(0, uintptr(id), modControl|modShift|modNoRepeat, vk)
		defer procUnregisterHotKey.Call(0, uintptr(id))
	}
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		if m.message == wmHotkey {
			on(int(m.wParam))
		}
	}
}

func showWindow(hwnd uintptr, show bool) {
	cmd := uintptr(swHide)
	if show {
		cmd = swShowNA
	}
	procShowWindow.Call(hwnd, cmd)
}

func exStyle(hwnd uintptr) uint32 {
	r, _, _ := procGetWindowLongW.Call(hwnd, uintptr(gwlExStyle))
	return uint32(r)
}

func setExStyle(hwnd uintptr, v uint32) {
	procSetWindowLongW.Call(hwnd, uintptr(gwlExStyle), uintptr(v))
}

// setLayeredAlpha makes the window layered and applies whole-window alpha (LWA_ALPHA).
func setLayeredAlpha(hwnd uintptr, alpha uint8) {
	setExStyle(hwnd, exStyle(hwnd)|wsExLayered)
	procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(alpha), lwaAlpha)
}

// setClickThrough toggles WS_EX_TRANSPARENT, which needs WS_EX_LAYERED to take effect.
func setClickThrough(hwnd uintptr, on bool, alpha uint8) {
	setLayeredAlpha(hwnd, alpha)
	s := exStyle(hwnd)
	if on {
		s |= wsExTranspar
	} else {
		s &^= wsExTranspar
	}
	setExStyle(hwnd, s)
}

// showError reports a startup failure in a message box, because a -H windowsgui binary has no
// console to print to.
func showError(title string, err error) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(err.Error())
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10) // MB_ICONERROR
}
