//go:build windows

// Command overlay-webview2 is a spike: a small borderless always-on-top chat overlay hosted in
// WebView2 through a pure-Go binding. See experiments/README.md.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"unsafe"

	"github.com/jchv/go-webview2/pkg/edge"
	"golang.org/x/sys/windows"
)

//go:embed page.html
var page string

// Translucency modes: alpha is whole-window alpha (LWA_ALPHA), pixel is a transparent WebView2
// background with only the panel drawn, opaque is the fallback.
const (
	modeAlpha  = "alpha"
	modePixel  = "pixel"
	modeOpaque = "opaque"
)

const (
	panelW, panelH = 320, 260
	wsPopup        = 0x80000000
	wsExTopmost    = 0x00000008
	wsExToolWindow = 0x00000080
)

type overlay struct {
	mode      string
	hwnd      uintptr
	web       *edge.Chromium
	opacity   uint8
	clickThru bool
	hidden    bool
}

var (
	app      overlay
	wndProcs = windows.NewCallback(wndProc)
)

func main() {
	runtime.LockOSThread()
	mode := flag.String("mode", modeAlpha, "translucency: alpha | pixel | opaque")
	flag.Parse()
	app.mode = *mode
	app.opacity = 220
	if err := run(); err != nil {
		showError("overlay-webview2", err)
		log.Fatal(err)
	}
}

func run() error {
	uninit, note, err := coInitStatus(hresultOf(windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)))
	if err != nil {
		return err
	}
	if note != "" {
		log.Print(note)
	}
	if uninit {
		defer windows.CoUninitialize()
	}
	var inst windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &inst)
	class, _ := windows.UTF16PtrFromString("wowcOverlay")
	brush, _, _ := procGetStockObject.Call(4) // BLACK_BRUSH
	wc := wndClassEx{
		size:       uint32(unsafe.Sizeof(wndClassEx{})),
		wndProc:    wndProcs,
		instance:   uintptr(inst),
		background: brush,
		className:  class,
	}
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassEx: %w", err)
	}
	title, _ := windows.UTF16PtrFromString("WoW Companion")
	hwnd, _, err := procCreateWindowExW.Call(wsExTopmost|wsExToolWindow, uintptr(unsafe.Pointer(class)),
		uintptr(unsafe.Pointer(title)), wsPopup, 200, 200, panelW, panelH, 0, 0, uintptr(inst), 0)
	if hwnd == 0 {
		return fmt.Errorf("CreateWindowEx: %w", err)
	}
	app.hwnd = hwnd

	app.web = edge.NewChromium()
	app.web.MessageCallback = func(m string) {
		if m == "drag" {
			startDrag(hwnd)
		}
	}
	if !app.web.Embed(hwnd) {
		return fmt.Errorf("WebView2 runtime not available")
	}
	if app.mode == modePixel {
		if c2 := app.web.GetController().GetICoreWebView2Controller2(); c2 != nil {
			if err := c2.PutDefaultBackgroundColor(edge.COREWEBVIEW2_COLOR{A: 0}); err != nil {
				log.Printf("DefaultBackgroundColor: %v", err)
			}
		}
	}
	app.web.Resize()
	app.web.NavigateToString(page)
	app.applyOpacity()
	registerHotkeys(hwnd)
	showWindow(hwnd, true)
	app.web.Focus()

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func wndProc(hwnd, message, wParam, lParam uintptr) uintptr {
	switch message {
	case wmSize:
		if app.web != nil {
			app.web.Resize()
		}
	case wmMove:
		if app.web != nil {
			_ = app.web.NotifyParentWindowPositionChanged()
		}
	case wmHotkey:
		app.onHotkey(int(wParam))
	case wmDestroy:
		procPostQuitMessage.Call(0)
	default:
		r, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
		return r
	}
	return 0
}

func (o *overlay) onHotkey(id int) {
	switch id {
	case hkToggleShow:
		o.hidden = !o.hidden
		showWindow(o.hwnd, !o.hidden)
	case hkClickThrough:
		o.clickThru = !o.clickThru
		alpha := uint8(255)
		if o.mode == modeAlpha {
			alpha = o.opacity
		}
		setClickThrough(o.hwnd, o.clickThru, alpha)
	case hkOpacityUp:
		o.opacity = uint8(min(int(o.opacity)+20, 255))
		o.applyOpacity()
	case hkOpacityDown:
		o.opacity = uint8(max(int(o.opacity)-20, 40))
		o.applyOpacity()
	}
}

// applyOpacity sets whole-window alpha in alpha mode and the CSS panel alpha in pixel mode.
func (o *overlay) applyOpacity() {
	switch o.mode {
	case modeAlpha:
		setLayeredAlpha(o.hwnd, o.opacity)
	case modePixel:
		o.web.Eval("setPanelAlpha(" + strconv.FormatFloat(float64(o.opacity)/255, 'f', 2, 64) + ")")
	}
}
