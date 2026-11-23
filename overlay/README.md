# WoW Companion overlay

A small desktop app (Tauri v2) that sits beside or over the game and chats with your AI agent about the
character you are playing. The Go advisor is the brain: the app starts `wowc-advisor companion --handshake`
as a sidecar and talks to it over 127.0.0.1. There is no game logic in the app.

## Get it

Download `WoW-Companion-Setup-<version>-x64.exe` from the
[latest release](https://github.com/WoW-Companion/advisor/releases/latest); no GitHub sign-in is needed.
It installs for your user only (no admin prompt). `SHA256SUMS` on the same page lets you check it. The full
path from download to advice is the [quickstart](../docs/quickstart.md).

Every change under `overlay/`, `cmd/` or `internal/` also builds on GitHub Actions (workflow **overlay**).
Those are preview builds of unreleased work: open the **Actions** tab, pick a green run and download from
**Artifacts** (a GitHub sign-in is needed, and they are kept 14 days). `wow-companion-installer` holds the
NSIS installer; `wow-companion-exe` holds the raw `wow-companion.exe` and `wowc-advisor.exe`, which must
stay in one folder.

**Version and updates.** The last line of the settings panel (gear) and the tray tooltip read
"WoW Companion 0.2.0 · advisor 0.2.0". On start and every 12 hours the advisor makes one unauthenticated
request for the newest GitHub release; if it is newer the line adds "· v0.3.0 available", and clicking
"available" opens the release page in your browser. Nothing is downloaded or installed for you. Turn it off
with **Check for updates** in settings; a failed lookup shows nothing.

**Upgrading.** Run a newer `WoW-Companion-Setup-*.exe` over an older install: it upgrades in place, with no
"uninstall first" page (`overlay/src-tauri/windows/installer.nsi` is Tauri's NSIS template with that one
change, set through `bundle.windows.nsis.template`; Tauri's installer hooks run too late to skip the page).
Settings and window position live in your app data folder, which the installer does not touch. Installing the
same or an older version still asks.

The installer is **not code-signed**, so Windows SmartScreen shows "Windows
protected your PC": click **More info**, then **Run anyway**.

## Use it with the game

In WoW, set **Display Mode** to **Windowed (Fullscreen)** (System, Graphics). A true fullscreen game
cannot show anything above it; Windowed (Fullscreen) looks the same and lets the overlay appear on top.

The app needs ForeverScribe installed (required); the WoW Companion addon is optional, for in-game extras
(panel, tracker, tooltips, right-click "Ask about this"). Questions are asked here in the
overlay; in-game question chips copy the question for pasting. Nothing is written to the game folder. A leftover
`WoWCompanion_Answers` folder can be deleted. It finds your SavedVariables itself, and shows your characters and NEXT list without an AI
client. To ask questions it needs one: it picks the first of Claude Code, Codex, Antigravity CLI and Gemini CLI (legacy) it finds
(search places in `docs/ask.md`). Choose another in the settings panel, **AI client** (Auto, Claude, Codex,
Antigravity, Gemini CLI (legacy) or a custom command); the choice is kept and the advisor restarts with it. With none installed the
app shows a calm notice with links to each install page. The picker's tooltip names the program in use;
**Re-scan** looks again (it restarts the advisor), so installing a client while the overlay is open needs
no restart of the app. Claude Code, Codex, Antigravity CLI and Gemini CLI (legacy, API key only) are supported. Each is started headless with
the advisor's tools pre-allowed and everything else (shell, file edits, web) denied or sandboxed, so you
never approve anything in the AI client; the tools are read-only, apart from `update_digest` writing the
digest file (`docs/ask.md` has a table of what each passes). Gemini needs the advisor in its settings
(`wowc-advisor setup gemini`); Antigravity needs `wowc-advisor setup agy --write` once. The Command box shows only for **Custom command**.

### Where Windows installs are found

The advisor looks for `claude`, `codex`, `agy` and `gemini` with the extensions in `PATHEXT` plus `.cmd`, `.bat`
and `.ps1`, in this order: the process's `PATH`, the user's and the system's `PATH` from the registry
(`HKCU\Environment`, `HKLM\...\Session Manager\Environment`: an app started from the Start menu may have a
minimal `PATH`), then these folders:

| Folder | Put there by |
| --- | --- |
| `%USERPROFILE%\.local\bin` | Claude Code's native installer (`claude.exe`) |
| `%USERPROFILE%\.claude\local` | Claude Code's local install |
| `%LOCALAPPDATA%\agy\bin` | Antigravity CLI (`agy.exe`) |
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

## The panel

- **Title bar:** drag to move; the gear toggles the settings, then the mode button, **-** (hide to tray)
  and **x** (quit). A small icon shows when click-through is on.
- **Settings:** Opacity, Solid on hover, AI client (with **Re-scan**), the two shortcuts and **Text size**
  (Small, Medium, Large).
- **Character bar:** the character box, the age chip (`now`, `5m`, `2h`, `1d`: amber after 30 minutes, red
  after 2 hours; `/reload` in game refreshes it), **Clear** and an arrow that collapses the chat.
- **Next:** the header collapses the character's NEXT list. It is open until the chat has a message, unless
  you chose. Settings, Next and the chat remember their state.
- **Ask:** type a question (up to 1,000 characters) and press **Ask**. The overlay does not write the
  in-game digest; ask it to "refresh my digest", or run `wowc-advisor digest`, then `/reload`.
- **Storage:** look settings are in the app's WebView2 storage under `%LOCALAPPDATA%\app.wowcompanion.overlay`;
  the AI client choice is `agent.txt` in `%APPDATA%\app.wowcompanion.overlay`. There is no log file. A black
  or blank window means the Edge WebView2 runtime is missing (Windows 11 has it).

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

## Setup checklist

On first run, and whenever a row is red, the overlay shows a checklist (Settings › Setup reopens it):
WoW folder found, ForeverScribe installed, ForeverScribe has data, the optional WoW Companion addon
(Install/Update from the addon's latest GitHub release, SHA256-verified), display mode (read-only
look at `WTF/Config.wtf`; exclusive fullscreen cannot be drawn over) and the AI client (Test sends
one tiny prompt). The checks and the install are Go code in `internal/gamesetup`, served by the
companion at `/api/setup*`; `wowc-advisor doctor --fix-addon` does the addon install from the CLI.
Downloads happen only on a click, over HTTPS from github.com, and write only under
`Interface/AddOns` (never `WoWCompanion_Digest/Digest.lua` or `WoWCompanion_Answers`).
ForeverScribe publishes no release zip, so its row links to the project page instead.
