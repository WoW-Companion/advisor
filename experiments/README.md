# Overlay spike: Gio versus WebView2

Two throwaway Windows prototypes of the same small in-game overlay, to decide which toolkit the
real overlay uses. They are separate Go modules, so they are not part of `make verify`,
`make build` or the release archives, and they add nothing to the advisor's `go.mod`.

Both show a borderless, always-on-top 320×260 panel (dark, translucent, thin gold border, title
"WoW Companion"), a scrolling text area that streams a fixed fake answer (20 characters every
50 ms, restarting on Enter) and a one-line input. Drag the title to move it. Close it with Alt+F4 while it has focus. A startup failure also
shows in a message box, since the binaries are `-H windowsgui`.

| Hotkey | Action |
| --- | --- |
| Ctrl+Shift+W | show / hide |
| Ctrl+Shift+T | click-through on / off (`WS_EX_TRANSPARENT`) |
| Ctrl+Shift+Up / Down | opacity up / down |

Hotkeys use `RegisterHotKey` only. No hooks, no access to any other process.

## Install (Windows)

cmd:

```
set GOPROXY=direct
go install github.com/WoW-Companion/advisor/experiments/overlay-gio@main
go install github.com/WoW-Companion/advisor/experiments/overlay-webview2@main
```

PowerShell:

```
$env:GOPROXY = "direct"
go install github.com/WoW-Companion/advisor/experiments/overlay-gio@main
go install github.com/WoW-Companion/advisor/experiments/overlay-webview2@main
```

`overlay-webview2` needs the Evergreen WebView2 runtime (present on current Windows 10/11).
Binaries land in `%GOPATH%\bin`. Building from a checkout: `cd experiments/overlay-gio` then
`set CGO_ENABLED=0` (PowerShell: `$env:CGO_ENABLED = "0"`) and `go build -ldflags "-s -w -H windowsgui"`.

## Translucency modes

Run each program three times with `-mode alpha` (default), `-mode pixel` and `-mode opaque`:

- `alpha`: whole-window alpha, `SetLayeredWindowAttributes(LWA_ALPHA)`; the opacity hotkeys change it.
- `pixel`: transparent background, only the panel drawn (Gio: transparent clear colour; WebView2:
  `DefaultBackgroundColor` alpha 0). The opacity hotkeys change the panel's own alpha.
- `opaque`: fallback, an opaque dark panel.

## Operator checklist

For each program and mode:

1. Memory: Task Manager, Details tab, sum **Memory (private working set)** over all of the program's
   processes (Gio is one; WebView2 is the exe plus every `msedgewebview2.exe` it started, so use
   the Processes tab tree). Note the total after 1 minute idle.
2. CPU while idle (panel shown, answer finished) and while streaming (press Enter in the input).
3. How it looks over WoW in Windowed (Fullscreen): is the panel visible, translucent, and does the
   background show through (`pixel`) or a black/white box (doesn't work)?
4. Do input and drag feel right: typing, Enter restarts the stream, drag by the title, WoW keeps
   keyboard focus when click-through is on, hotkeys work while WoW has focus.
5. Does the `alpha` mode dim the whole panel including text, and do the opacity hotkeys work?
