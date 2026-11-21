//go:build windows

package main

import (
	"errors"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                         = windows.NewLazySystemDLL("user32.dll")
	gdi32                          = windows.NewLazySystemDLL("gdi32.dll")
	procGetStockObject             = gdi32.NewProc("GetStockObject")
	procRegisterClassExW           = user32.NewProc("RegisterClassExW")
	procCreateWindowExW            = user32.NewProc("CreateWindowExW")
	procDefWindowProcW             = user32.NewProc("DefWindowProcW")
	procGetMessageW                = user32.NewProc("GetMessageW")
	procTranslateMessage           = user32.NewProc("TranslateMessage")
	procDispatchMessageW           = user32.NewProc("DispatchMessageW")
	procPostQuitMessage            = user32.NewProc("PostQuitMessage")
	procRegisterHotKey             = user32.NewProc("RegisterHotKey")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procGetWindowLongW             = user32.NewProc("GetWindowLongW")
	procSetWindowLongW             = user32.NewProc("SetWindowLongW")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procReleaseCapture             = user32.NewProc("ReleaseCapture")
	procSendMessageW               = user32.NewProc("SendMessageW")
	procMessageBoxW                = user32.NewProc("MessageBoxW")
)

var gwlExStyle = int32(-20)

const (
	modControl   = 0x0002
	modShift     = 0x0004
	modNoRepeat  = 0x4000
	wmDestroy    = 0x0002
	wmMove       = 0x0003
	wmSize       = 0x0005
	wmHotkey     = 0x0312
	wmNCLButton  = 0x00A1
	htCaption    = 2
	wsExLayered  = 0x00080000
	wsExTranspar = 0x00000020
	lwaAlpha     = 0x2
	swHide       = 0
	swShow       = 5
	vkUp         = 0x26
	vkDown       = 0x28
)

// Hotkey ids.
const (
	hkToggleShow = iota + 1
	hkClickThrough
	hkOpacityUp
	hkOpacityDown
)

type wndClassEx struct {
	size       uint32
	style      uint32
	wndProc    uintptr
	clsExtra   int32
	wndExtra   int32
	instance   uintptr
	icon       uintptr
	cursor     uintptr
	background uintptr
	menuName   *uint16
	className  *uint16
	iconSm     uintptr
}

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	ptX     int32
	ptY     int32
}

func registerHotkeys(hwnd uintptr) {
	keys := map[int]uintptr{hkToggleShow: 'W', hkClickThrough: 'T', hkOpacityUp: vkUp, hkOpacityDown: vkDown}
	for id, vk := range keys {
		procRegisterHotKey.Call(hwnd, uintptr(id), modControl|modShift|modNoRepeat, vk)
	}
}

func showWindow(hwnd uintptr, show bool) {
	cmd := uintptr(swHide)
	if show {
		cmd = swShow
	}
	procShowWindow.Call(hwnd, cmd)
}

// startDrag hands the mouse to the window manager as if the caption had been clicked.
func startDrag(hwnd uintptr) {
	procReleaseCapture.Call()
	procSendMessageW.Call(hwnd, wmNCLButton, htCaption, 0)
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

// hresultOf recovers the HRESULT from the error x/sys/windows builds for a nonzero return.
func hresultOf(err error) int32 {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return int32(uint32(errno))
	}
	return 0
}

// showError reports a startup failure in a message box, because a -H windowsgui binary has no
// console to print to.
func showError(title string, err error) {
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(err.Error())
	procMessageBoxW.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10) // MB_ICONERROR
}
