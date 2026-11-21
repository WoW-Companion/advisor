//go:build windows

// Command overlay-gio is a spike: a small borderless always-on-top chat overlay drawn with Gio
// (pure Go, Direct3D 11, one process). See experiments/README.md.
package main

import (
	"flag"
	"image"
	"image/color"
	"log"
	"os"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

const fakeAnswer = "Your best upgrade right now is the Dawnbreaker Helm from the Sunwell dungeon: " +
	"+38 item level over your current head slot, and it drops from the last boss. " +
	"If you cannot run it, the vendor helm for 60 badges is a close second. " +
	"Reforge toward haste until you hit the 12% breakpoint, then stack crit. "

const (
	streamChars    = 20
	streamInterval = 50 * time.Millisecond
)

var gold = color.NRGBA{R: 0xd4, G: 0xaf, B: 0x37, A: 0xff}

// Translucency modes: alpha is whole-window alpha (LWA_ALPHA), pixel is a transparent clear with
// only the panel drawn, opaque is the fallback.
const (
	modeAlpha  = "alpha"
	modePixel  = "pixel"
	modeOpaque = "opaque"
)

type overlay struct {
	mode        string
	hwnd        uintptr
	opacity     uint8
	clickThru   bool
	hidden      bool
	answer      []rune
	shown       int
	list        widget.List
	input       widget.Editor
	th          *material.Theme
	hotkeyEvent chan int
}

func main() {
	mode := flag.String("mode", modeAlpha, "translucency: alpha | pixel | opaque")
	flag.Parse()
	o := &overlay{
		mode:        *mode,
		opacity:     220,
		answer:      []rune(fakeAnswer),
		hotkeyEvent: make(chan int, 8),
	}
	o.list.Axis = layout.Vertical
	o.list.ScrollToEnd = true
	o.input.SingleLine = true
	o.input.Submit = true
	o.th = material.NewTheme()
	o.th.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))
	o.th.Palette.Fg = color.NRGBA{R: 0xee, G: 0xee, B: 0xee, A: 0xff}

	go func() {
		w := new(app.Window)
		w.Option(app.Title("WoW Companion"), app.Size(320, 260), app.Decorated(false), app.TopMost(true))
		if err := o.loop(w); err != nil {
			log.Println(err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func (o *overlay) loop(w *app.Window) error {
	ticker := time.NewTicker(streamInterval)
	defer ticker.Stop()
	go func() {
		for range ticker.C {
			if o.shown < len(o.answer) {
				o.shown = min(o.shown+streamChars, len(o.answer))
				w.Invalidate()
			}
		}
	}()
	var ops op.Ops
	started := false
	for {
		select {
		case id := <-o.hotkeyEvent:
			o.onHotkey(id)
			w.Invalidate()
		default:
		}
		switch e := w.Event().(type) {
		case app.Win32ViewEvent:
			if e.Valid() && !started {
				started = true
				o.hwnd = e.HWND
				o.applyOpacity()
				go runHotkeys(func(id int) { o.hotkeyEvent <- id; w.Invalidate() })
			}
		case app.DestroyEvent:
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			o.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (o *overlay) onHotkey(id int) {
	switch id {
	case hkToggleShow:
		o.hidden = !o.hidden
		showWindow(o.hwnd, !o.hidden)
	case hkClickThrough:
		o.clickThru = !o.clickThru
		setClickThrough(o.hwnd, o.clickThru, o.windowAlpha())
	case hkOpacityUp:
		o.opacity = uint8(min(int(o.opacity)+20, 255))
		o.applyOpacity()
	case hkOpacityDown:
		o.opacity = uint8(max(int(o.opacity)-20, 40))
		o.applyOpacity()
	}
}

// windowAlpha is the whole-window alpha: only the alpha mode changes it.
func (o *overlay) windowAlpha() uint8 {
	if o.mode == modeAlpha {
		return o.opacity
	}
	return 255
}

func (o *overlay) applyOpacity() {
	if o.mode == modeAlpha {
		setLayeredAlpha(o.hwnd, o.opacity)
	}
}

func (o *overlay) layout(gtx layout.Context) layout.Dimensions {
	for {
		ev, ok := o.input.Update(gtx)
		if !ok {
			break
		}
		if _, submit := ev.(widget.SubmitEvent); submit {
			o.shown = 0
			o.input.SetText("")
		}
	}

	// Background: the clear colour is transparent for per-pixel alpha, dark otherwise.
	panel := color.NRGBA{R: 0x10, G: 0x12, B: 0x18, A: 0xf0}
	switch o.mode {
	case modePixel:
		paint.ColorOp{Color: color.NRGBA{}}.Add(gtx.Ops)
		paint.PaintOp{}.Add(gtx.Ops)
		panel.A = o.opacity
	case modeOpaque:
		panel.A = 0xff
	}
	size := gtx.Constraints.Max
	paint.FillShape(gtx.Ops, panel, clip.Rect{Max: size}.Op())
	border := gtx.Dp(1)
	paint.FillShape(gtx.Ops, gold, clip.Stroke{Path: clip.Rect{Min: image.Pt(border/2, border/2), Max: size.Sub(image.Pt(border/2, border/2))}.Path(), Width: float32(border)}.Op())

	return layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(o.title),
			layout.Rigid(layout.Spacer{Height: 6}.Layout),
			layout.Flexed(1, o.answerView),
			layout.Rigid(layout.Spacer{Height: 6}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				ed := material.Editor(o.th, &o.input, "Ask…")
				ed.HintColor = color.NRGBA{R: 0x88, G: 0x88, B: 0x88, A: 0xff}
				ed.Color = o.th.Palette.Fg
				return ed.Layout(gtx)
			}),
		)
	})
}

// title draws the heading; dragging anywhere on it moves the window.
func (o *overlay) title(gtx layout.Context) layout.Dimensions {
	lbl := material.Label(o.th, 16, "WoW Companion")
	lbl.Color = gold
	lbl.Font.Weight = 700
	macro := op.Record(gtx.Ops)
	dims := lbl.Layout(gtx)
	call := macro.Stop()
	dims.Size.X = gtx.Constraints.Max.X
	area := clip.Rect{Max: dims.Size}.Push(gtx.Ops)
	system.ActionInputOp(system.ActionMove).Add(gtx.Ops)
	area.Pop()
	call.Add(gtx.Ops)
	return dims
}

func (o *overlay) answerView(gtx layout.Context) layout.Dimensions {
	txt := string(o.answer[:o.shown])
	return material.List(o.th, &o.list).Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		lbl := material.Body2(o.th, txt)
		lbl.Color = o.th.Palette.Fg
		return lbl.Layout(gtx)
	})
}
