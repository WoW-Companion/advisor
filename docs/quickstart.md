# Quickstart: from download to in-game advice

One path for a new Windows player, top to bottom. Each step is one action and says what success looks
like. `<you>` is your Windows user name and `<account>` your WoW account folder; use your own.

## 1. Install the addons

Action: install [ForeverScribe](https://github.com/MattMaximo/foreverscribe) into your game's
`_forever_\Interface\AddOns` folder. Then add the three WoW Companion folders (`WoWCompanion`,
`WoWCompanion_Digest`, `WoWCompanion_Answers`) to the same folder. There is no player-friendly download
for them yet: take them from the [addon repository](https://github.com/WoW-Companion/addon) (GitHub's
**Code**, then **Download ZIP**, and copy just those three folders out of it), or build a zip with
`make package` there. Then **quit the game completely and start it again**: the game only finds a new
addon folder at startup, so `/reload` is not enough the first time.

Success: in game, `/wowc status` prints the addon version. After your first `/reload` or logout,
ForeverScribe has written `WTF\Account\<account>\SavedVariables\ForeverScribe.lua`.

## 2. Get WoW Companion

**Recommended: the overlay app.** It bundles the advisor, so there is nothing else to install.

Action: open the [latest release](https://github.com/WoW-Companion/advisor/releases/latest) of the advisor
repository (no GitHub sign-in needed) and download the asset named `WoW-Companion-Setup-<version>-x64.exe`.
(Builds from the **Actions** tab are preview builds of unreleased changes; use the release.) Run the installer; it installs for your user and asks for no
admin rights. Windows SmartScreen warns because the installer is unsigned: click **More info**, then
**Run anyway**.

Success: WoW Companion is in the Start menu and opens a small panel.

**Alternative: `wowc-advisor` alone** (no overlay; you get the browser companion instead). Download the
archive for your system from the [releases page](https://github.com/WoW-Companion/advisor/releases). Each is
named `wowc-advisor_<version>_<os>_<arch>`: `.zip` for Windows, `.tar.gz` for macOS and Linux, with `amd64`
for most PCs and Intel Macs and `arm64` for ARM PCs and Apple Silicon Macs. Each holds one static program,
the license, the third-party notices and a short readme. `SHA256SUMS` on the same page lets you check the
download.

- **Windows:** unzip it anywhere, then run `wowc-advisor.exe doctor` in that folder. The binary is
  **not signed**, because signing and notarization need paid certificates, so SmartScreen may say it
  "protected your PC". Click **More info**, then **Run anyway**.
- **macOS:** unpack it (`tar -xzf <file>`). If Gatekeeper blocks the downloaded binary (it is not
  notarized), run `xattr -d com.apple.quarantine wowc-advisor` in that folder.
- **Linux (Wine or Proton setups):** unpack it with `tar -xzf <file>`.
- **You have Go?** In PowerShell:

```
$env:GOPROXY="direct"; go install github.com/WoW-Companion/advisor/cmd/wowc-advisor@latest
```

(`GOPROXY=direct` skips the proxy cache, which can serve a stale build.) Success: `wowc-advisor version`
(`wowc-advisor.exe version` on Windows) prints `wowc-advisor` and a version. `doctor --check` also says
whether a newer release exists; it never downloads or replaces anything. A `go install` binary lands in
`%USERPROFILE%\go\bin`, which may not be on PATH; run it by full path if so.

## 3. Install an AI client

Answers need one AI client. Advice lists, the NEXT list and the panel work without it.

Action: install Claude Code with its official installer, in PowerShell:
`irm https://claude.ai/install.ps1 | iex`. Then run `claude` once in a terminal and sign in. It must be
signed in once before it can answer headlessly.

The installer puts `claude.exe` in `%USERPROFILE%\.local\bin`, which is often not on PATH, so `claude` may
not work in a new terminal. You do not need to fix that for the overlay: it looks there (and in the other
usual places) itself. See [overlay/README.md](../overlay/README.md).

Alternatives: [Codex](https://github.com/openai/codex) or
[Gemini CLI](https://github.com/google-gemini/gemini-cli), each from its own install page; sign in once
too. Gemini also needs `wowc-advisor setup gemini --write`.

Success: `claude` answers in a terminal, or the overlay (next step) shows the client's name.

## 4. Set up the overlay

Open WoW Companion from the Start menu.

- **Windowed (Fullscreen).** In WoW, System, Graphics, set **Display Mode** to **Windowed (Fullscreen)**.
  The overlay only draws over the game in that mode; in true Fullscreen it is hidden behind the game.
- **Modes.** The pin/window button in the title bar switches **Overlay** (always on top, no taskbar entry)
  and **Window** (a normal window). The settings (the gear) hold **Opacity** (0% to 100%) and
  **Solid on hover**. Opacity fades only the panel background, never text or controls; with Solid on
  hover the panel turns mostly opaque while the mouse is over it.
- **Click-through.** `Ctrl+Shift+T` lets mouse clicks pass through the panel to the game (a small icon
  shows it is on). Press it again, or use the tray icon's **Click-through**, to get the panel back. It is
  always off when the app starts.
- **Shortcuts and tray.** `Ctrl+Shift+W` shows or hides the panel. Change either shortcut in the settings:
  click its box and press the new keys. The tray icon has Show/Hide, Overlay mode, Click-through and
  Quit. The title bar's **-** hides to the tray; **x** quits. If the panel is invisible at 0% opacity,
  use `Ctrl+Shift+W` or the tray.
- **AI client.** The settings' **AI client** box says **Auto** and names what it found. Pick Claude,
  Codex, Gemini or a custom command instead if you like. After installing a client while the overlay is
  open, press **Re-scan**; no restart of the app is needed.
- **Text size.** Settings, **Text size**: Small, Medium or Large.
- **Collapsing.** The gear toggles the settings, the **Next** header collapses the NEXT list and the arrow
  by **Clear** collapses the chat. The panels remember it. NEXT opens by itself until you ask something,
  unless you chose.
- **The age chip.** Next to the character box, a chip shows how old the game data is: `now`, `5m`, `2h`,
  `1d`. It turns amber after 30 minutes and red after 2 hours; `/reload` in game refreshes it.
- **The game is never touched.** The app does not inject into, read or send input to WoW. It is an ordinary
  window that talks to the advisor over 127.0.0.1.

Success: the panel shows your most recently played character, its NEXT list and an age chip. Settings
live in the app's own storage, not a file you edit; see the FAQ.

## 5. Put the digest in the game panel

With the overlay running, the advice refreshes itself; `/reload` to see it.

The overlay's advisor rewrites the in-game digest a couple of seconds after ForeverScribe saves, and
skips the write when nothing changed. The game only saves and loads addon files on `/reload`, so the
sequence is: your `/reload` saves, the overlay writes within a couple of seconds, and your next
`/reload` loads it and shows it in `/wowc`. One `/reload` both saves and loads, but the load happens
before the overlay's write, so the new advice shows on the following one. Switch it off with "Keep
in-game advice up to date" in the overlay's settings; it shows "addon not found" and does nothing
until the `WoWCompanion_Digest` addon folder exists. The age chip's tooltip says when the in-game
advice was last updated.

Without the overlay, run `wowc-advisor digest` in a terminal (or `wowc-advisor companion --auto-digest`
to get the same automatic writes), then `/reload` in game and `/wowc`. You can also type "refresh my
digest" to a connected AI client.

Success: the digest file is written (the command says it wrote the `WoWCompanion_Digest` addon), and the panel opens
with your next quests, upgrades and materials. Every character you have played is kept, so one digest
covers them all.

## 6. Ask

Pick the way that suits you:

- **The overlay's Ask box (preferred).** Type a question and press **Ask**. The answer streams in beside
  the game; no `/reload` needed. The age chip tells you how fresh the data is.
- **The browser companion**, `wowc-advisor companion --agent claude`, is the same chat in a browser tab
  (see below). Use it without the overlay.
- **In game, `/ask <question>`.** Slowest: `/reload` to send it out, run `wowc-advisor ask --agent claude`
  (or `--watch` to keep polling), then `/reload` again for the answer. `claude`, `codex` and `gemini` are
  the agent presets; see [ask.md](ask.md).

The advisor's tools are pre-allowed for these runs and read-only (apart from `update_digest`, which writes
the digest file); nothing else is. The presets pass the permission flags themselves, so you never approve
anything in the AI client. Advice reflects the last ForeverScribe save, so `/reload` before asking if
you just played.

To connect an AI client you use yourself (Claude Desktop, Claude Code, Codex, Gemini) to the advisor, run
`wowc-advisor setup claude-desktop --write` (or `claude-code`, `codex`, `gemini`); without `--write` it
only prints the entry. Check everything with `wowc-advisor doctor`. Details: [clients.md](clients.md).

## The browser companion (no overlay)

Command: `wowc-advisor companion --agent claude`

Success: your browser opens a chat page (URL `http://127.0.0.1:4317/?token=...`). Pick the character
(the most recently captured is the default), type a question, and the answer streams in as the agent
writes it. The "data from game" badge shows when ForeverScribe last saved; it updates the moment you
`/reload` or log out. Press Ctrl-C to stop.

Why the in-game Ask box can't do this: the WoW client gives addons no file access at runtime.
SavedVariables are written only on `/reload` or logout, and addon files are read only at login or
`/reload`, so an in-game question needs one `/reload` to go out and another to bring the answer back.
The companion sits beside the game instead of inside it, and changes nothing in the client.

- **Second monitor:** drag the browser window over. **Over the game:** run WoW in Windowed (Fullscreen)
  and put the window on top or beside it; the overlay app does that for you.
- **Phone or another PC:** `wowc-advisor companion --lan` listens on all interfaces and prints a LAN
  URL with the token. Anyone with that URL can ask your AI agent, so use it on a network you trust.
  `--port N` picks the first port to try (default 4317; the next free one is used), `--no-open`
  skips opening the browser.
- **Questions asked in game** show up in the page marked "asked in game", and the companion still writes
  Answers.lua for them, like `ask --watch`, so `/reload` in game shows them too. Do not run
  `ask --watch` on the same file at the same time.
- **Streaming:** Claude (`--agent claude`) streams text as it arrives. Codex and Gemini answers appear
  once finished, marked "not streamed".
- **Context:** the last 6 turns about the same character (at most 4,000 characters, answers cut to 600)
  are sent with each question. Questions are limited to 1,000 characters, and one answer runs at a time.
- **Safety:** it binds to 127.0.0.1, checks the `Host` and `Origin` of every request, and needs the random
  token printed at start (kept in a cookie). The agent runs exactly as `ask` runs it.

## What the advisor can answer

| Tool | Answers |
| ---- | ------- |
| `server_info` | Which version, dataset build and game build are running, and whether the capture was found. |
| `list_characters` | Which characters were captured, and when. |
| `character_brief` | Level, class, gear, money and quests of one character. |
| `whats_next` | The ranked next steps: turn-ins, talents, open quests. |
| `upgrade_finder` | Better gear per slot and where it drops. |
| `profession_plan` | How to level each profession. |
| `talent_plan` | Where the next talent points go. |
| `dungeon_plan` | Which dungeons fit your level and drop an upgrade. |
| `farm_spots` | Where to farm an item. |
| `quest_chain` | Where a quest sits in its quest line. |
| `inventory_review` | What to keep, sell or bank from your bags and bank. |
| `update_digest` | Writes the in-game digest the panel shows. |
| `doctor` | Checks the whole setup and says how to fix each failure. |

## FAQ and troubleshooting

**"Auto (none found)" in the AI client box.** No client was found. Install one (step 3), sign in once,
then press **Re-scan**.

**The AI says the advisor's tools are not allowed.** Your build is outdated: the permission flags were
added later. Install a newer overlay (step 2) or rebuild with `@main`.

**The data is stale, or the age chip is amber or red.** The advice reflects the last ForeverScribe save.
Type `/reload` in game (or log out) and the chip returns to `now`. The in-game panel refreshes itself
while the overlay runs (`/reload` twice, see step 5); without the overlay run `wowc-advisor digest`
then `/reload`. The panel asks for a refresh after 12 hours.

**The overlay does not show over the game.** Set WoW's Display Mode to **Windowed (Fullscreen)**
(System, Graphics). In true Fullscreen nothing can draw above the game. Also check the panel is not
hidden (`Ctrl+Shift+W`) or at 0% opacity with click-through on.

**Windows SmartScreen says "Windows protected your PC".** The installer is not code-signed. Click
**More info**, then **Run anyway**.

**The addon does not appear in game.** A newly added addon folder needs a full game restart; `/reload`
does not find it. Check that all three folders sit in `_forever_\Interface\AddOns`.

**A digest for another character.** `/reload` on that character so ForeverScribe captures it, then run
`wowc-advisor digest` (every captured character is kept in one digest) and `/reload`. In the overlay pick
the character in the box at the top.

**The overlay is black or blank.** It needs the Microsoft Edge WebView2 runtime. Windows 11 has it;
otherwise install it from
[Microsoft](https://developer.microsoft.com/microsoft-edge/webview2/#download-section).

**Where are settings and logs?** The overlay keeps its look settings (opacity, text size, shortcuts,
collapsed panes) in its WebView2 storage under `%LOCALAPPDATA%\app.wowcompanion.overlay`, and the AI
client choice in `%APPDATA%\app.wowcompanion.overlay\agent.txt`. It writes no log file: if the advisor
stops, the red bar names its last error; press **Reconnect**. Run `wowc-advisor doctor` for the setup.

**`go install ...@latest` gives an old build.** Use `$env:GOPROXY="direct"` and `@main` (step 2).

**`claude` is not recognized in a terminal.** `%USERPROFILE%\.local\bin` is not on PATH. The overlay
finds it anyway; to use it in a terminal add that folder to your user PATH.

**The realm shows blank.** The game client sometimes captures an empty realm. The advisor keeps it
blank and finds the character by name; nothing is wrong. See [foreverscribe-format.md](foreverscribe-format.md).

**"Bank not captured".** Open your bank once in game, then `/reload`. Until then only the bags are reviewed.

**Quests say "level unknown".** Neither the capture nor the dataset has that quest's level. The advisor
lists those quests last and never guesses. See [advice.md](advice.md).

**Where does it look for `ForeverScribe.lua`?** With no `--savedvariables` and no `WOWC_SAVEDVARIABLES`,
the advisor searches `C:\Program Files (x86)\World of Warcraft`, `C:\Program Files\World of Warcraft`, the same
two folders on every other drive, and (on macOS) `/Applications/World of Warcraft`, for
`_forever_\WTF\Account\*\SavedVariables\ForeverScribe.lua` (then `_classic_era_` and `_retail_`). It prints
one line naming the file it picked; with several accounts it takes the most recently modified one and lists
the others. To use a different file, pass `--savedvariables` or run
`setx WOWC_SAVEDVARIABLES "<path to ForeverScribe.lua>"` (then open a new terminal). An explicit path always wins.

**Windows paths.** In JSON config files use forward slashes or doubled backslashes
(`C:/Users/<you>/wowc-advisor.exe`). In PowerShell or cmd, quote any path with spaces, such as
`Program Files (x86)`. The capture is at
`World of Warcraft\_forever_\WTF\Account\<account>\SavedVariables\ForeverScribe.lua`; use the file that
exists on your PC. More in [try-it.md](try-it.md).
