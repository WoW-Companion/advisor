# WoW Companion overlay

A small desktop app (Tauri v2) that sits beside or over the game and chats with your AI agent about the
character you are playing. The Go advisor is the brain: the app starts `wowc-advisor companion --handshake`
as a sidecar and talks to it over 127.0.0.1. There is no game logic in the app.

## Get it

Every change under `overlay/`, `cmd/` or `internal/` builds on GitHub Actions (workflow **overlay**).
Open the repository's **Actions** tab, pick a run of **overlay**, and download from **Artifacts**:

- `wow-companion-installer`: the NSIS installer (`WoW Companion_<version>_x64-setup.exe`). Installs for
  your user only; no admin prompt.
- `wow-companion-exe`: the raw `wow-companion.exe` and `wowc-advisor.exe`; keep them in one folder.

Artifacts are kept 14 days. The installer is **not code-signed**, so Windows SmartScreen shows "Windows
protected your PC": click **More info**, then **Run anyway**.

## Use it with the game

In WoW, set **Display Mode** to **Windowed (Fullscreen)** (System, Graphics). A true fullscreen game
cannot show anything above it; Windowed (Fullscreen) looks the same and lets the overlay appear on top.

The app needs the same setup as the rest of the advisor: ForeverScribe and the WoW Companion addon
installed. It finds your SavedVariables itself, and shows your characters and NEXT list without an AI
client. To ask questions it needs one: it picks the first of Claude Code, Codex and Gemini CLI it finds
(search places in `docs/ask.md`). Choose another in the settings panel, **AI client** (Auto, Claude, Codex,
Gemini or a custom command); the choice is kept and the advisor restarts with it. With none installed the
app shows a calm notice with links to each install page. The picker's tooltip names the program in use;
**Re-scan** looks again (it restarts the advisor), so installing a client while the overlay is open needs
no restart of the app. The Command box shows only for **Custom command**.

### Where Windows installs are found

The advisor looks for `claude`, `codex` and `gemini` with the extensions in `PATHEXT` plus `.cmd`, `.bat`
and `.ps1`, in this order: the process's `PATH`, the user's and the system's `PATH` from the registry
(`HKCU\Environment`, `HKLM\...\Session Manager\Environment`: an app started from the Start menu may have a
minimal `PATH`), then these folders:

| Folder | Put there by |
| --- | --- |
| `%USERPROFILE%\.local\bin` | Claude Code's native installer (`claude.exe`) |
| `%USERPROFILE%\.claude\local` | Claude Code's local install |
| `%APPDATA%\npm` | `npm install -g` shims (`claude.cmd`, `codex.cmd`, `gemini.cmd`) |
| `%LOCALAPPDATA%\Microsoft\WinGet\Links` | WinGet (`winget install Anthropic.ClaudeCode`) |
| `%USERPROFILE%\scoop\shims` | Scoop |
| `%ProgramFiles%\nodejs` | a system-wide npm prefix |

A `.cmd` or `.bat` agent runs through `cmd.exe /d /s /c` and a `.ps1` through `powershell.exe -File`, with every
argument quoted. For `.cmd` the prompt's line breaks become spaces, `"` becomes `'` and `%` becomes `％`, because
`cmd` would otherwise end the command or expand `%VAR%` in it.

## Modes

| Mode | What it does |
| ---- | ------------ |
| **Overlay** | Always on top, no taskbar entry, opacity slider (0% to 100%, in the settings panel; it fades only the panel background, never the text or controls) with an optional "Solid on hover" and optional click-through. |
| **Window** | A normal window with a taskbar entry. |

Switch with the pin / window icon button in the title bar (tooltip names the mode), or the tray menu. The window is see-through
outside the rounded panel. Position, size, mode and opacity are remembered.

## Shortcuts and click-through

| Shortcut | Does |
| -------- | ---- |
| `Ctrl+Shift+W` | Show or hide the window |
| `Ctrl+Shift+T` | Click-through on or off (Overlay mode) |

Both are configurable in the settings panel (the gear): click a box and press the new keys. If another
program already holds a key, the app shows a notice and keeps running.

**Click-through** lets mouse clicks pass through the panel to the game. Turn it off again with
`Ctrl+Shift+T` or the tray menu (**Click-through**); it is always off when the app starts. The tray icon
also has Show/Hide, Overlay mode and Quit. The title bar's **-** hides to the tray; **x** quits.

## Never touches the game

The app does not inject into, read the memory of, hook or send input to the game process. It is an
ordinary window plus one local web connection to the advisor. Global shortcuts go through Tauri's
`global-shortcut` plugin only. It has no telemetry and no updater.

## Build it yourself

Needs Go, Rust and the Tauri CLI (`cargo install tauri-cli --version "=2.12.6" --locked`). From the repo root:

```
# the sidecar, named for the target triple (shown for Windows)
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o overlay/src-tauri/binaries/wowc-advisor-x86_64-pc-windows-msvc.exe ./cmd/wowc-advisor
cd overlay/src-tauri && cargo tauri build --bundles nsis
```

`cargo check` also needs the sidecar file for your own host triple (`rustc -vV` names it) in
`src-tauri/binaries/`, because the build script checks it exists.

## CI

`.github/workflows/overlay.yml` builds the installer on Windows for every PR that touches the overlay,
the advisor or the workflow, and on pushes to `main`. A newer commit cancels an older PR build. Rust
dependencies are cached with `Swatinem/rust-cache`; only `main` saves that cache (GitHub never lets one
PR branch read another's), so PRs restore main's. The Tauri CLI has its own cache entry keyed on its
pinned version; bump the version in the workflow when the `tauri` crate's minor version moves.

## Layout

- `src-tauri/`: the Rust shell (window, tray, shortcuts, sidecar), `tauri.conf.json`, capabilities, icons.
- `ui/`: static HTML, CSS and plain JS; no npm build. Fonts are bundled under `ui/fonts/`: Cinzel and
  Inter, both SIL Open Font License (licenses alongside).
- `tools/titlebar-check/`, `tools/status-check/`: headless-Chrome checks of the title bar and of the banners,
  AI client picker and Command box (`run.sh` in each; they need Chrome and python3).
- `tools/genicon/`: draws the emblem and writes `src-tauri/icons/` (`go run ./tools/genicon src-tauri/icons` from `overlay/`).
