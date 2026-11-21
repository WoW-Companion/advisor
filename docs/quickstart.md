# Quickstart: from download to in-game advice

One path, eight steps. Each step is one command or one click, and says what success looks like.
`<you>` is your Windows user name and `<account>` your WoW account folder; use your own.

## 1. Install ForeverScribe and the WoW Companion addon

Click: install [ForeverScribe](https://github.com/MattMaximo/foreverscribe) into your game's
`Interface\AddOns` folder. Then get the WoW Companion addon zip (a release, or `make package` in the
addon repo, which writes `dist/WoWCompanion-<version>.zip`) and unzip it into the same folder.

Success: `Interface\AddOns` holds `WoWCompanion`, `WoWCompanion_Digest` and `WoWCompanion_Answers`
side by side, next to the ForeverScribe folder.

## 2. `/reload` in game

Command: type `/reload` in the game chat.

Success: `/wowc status` prints the addon version, and ForeverScribe has written
`WTF\Account\<account>\SavedVariables\ForeverScribe.lua`.

## 3. Get `wowc-advisor`

Click: download `wowc-advisor-windows-amd64.exe` (`-arm64.exe` on an ARM PC) from the
[releases page](https://github.com/WoW-Companion/advisor/releases) and save it, for example, as
`C:\Users\<you>\wowc-advisor.exe`. It is one static file. Windows may ask you to confirm running it.

Success: `C:\Users\<you>\wowc-advisor.exe version` prints `wowc-advisor` and a version.

## 4. Connect your AI client

On a standard install you can skip the `--savedvariables` path here: the advisor finds
`ForeverScribe.lua` itself (see the FAQ). The client entry still has to be written once.

Command (Claude Desktop shown; the others are `claude-code`, `codex` and `gemini`):

```
wowc-advisor setup claude-desktop --savedvariables "C:\Program Files (x86)\World of Warcraft\_forever_\WTF\Account\<account>\SavedVariables\ForeverScribe.lua" --write
```

Without `--write` it only prints the entry and the file it belongs in. Then fully restart the client.
Details per client are in [clients.md](clients.md).

Success: the command names the config file it wrote and keeps a `.bak` copy of the old one.

## 5. Check the setup

Command: `wowc-advisor doctor` (add `--savedvariables "<the path>"` only if your WoW is not in a
standard folder)

Success: every check passes, and the SavedVariables line says `(auto-detected)` or where else the path came from. A failing check prints the fix for it. The game dataset is downloaded
on first use and cached; the check says if it is missing.

## 6. Ask your AI client

Ask: "what should I do next?" and then "refresh my digest".

Success: the first answer names the "ForeverScribe capture at ..." time and ranked steps; the second
says it wrote the digest into the `WoWCompanion_Digest` addon.

## 7. `/reload` to see the panel

Command: `/reload`, then `/wowc` in game.

Success: the panel opens and shows your next quests, upgrades, materials and notes, with the digest's age.

## 8. Ask from inside the game

Command: type `/ask <question>` in game (or use the Ask box in the panel). `/reload` once to send it
out, run `wowc-advisor ask --agent claude` (or `--watch` to keep
polling), then `/reload` again.

Success: the answer appears under your question in the panel. `claude`, `codex` and `gemini` are the
agent presets; see [ask.md](ask.md).

## Live answers beside the game

Command: `wowc-advisor companion --agent claude`

Success: your browser opens a chat page (URL `http://127.0.0.1:4317/?token=...`). Pick the character
(the most recently captured is the default), type a question, and the answer streams in as the agent
writes it. The "data from game" badge shows when ForeverScribe last saved; it updates the moment you
`/reload` or log out. Press Ctrl-C to stop.

Why the in-game Ask box can't do this: the WoW client gives addons no file access at runtime.
SavedVariables are written only on `/reload` or logout, and addon files are read only at login or
`/reload`, so an in-game question needs one `/reload` to go out and another to bring the answer back.
The companion sits beside the game instead of inside it, and changes nothing in the client.

- **Second monitor:** drag the browser window over. **Over the game:** run WoW in windowed fullscreen
  and put the window on top or beside it.
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

## FAQ

**The realm shows blank.** The game client sometimes captures an empty realm. The advisor keeps it
blank and finds the character by name; nothing is wrong. See [foreverscribe-format.md](foreverscribe-format.md).

**"Bank not captured".** Open your bank once in game, then `/reload`. Until then only the bags are reviewed.

**Quests say "level unknown".** Neither the capture nor the dataset has that quest's level. The advisor
lists those quests last and never guesses. See [advice.md](advice.md).

**Switching characters.** `/reload` on the new one, then ask for a digest update; every character you've
played is kept.

**The digest is stale.** The panel shows its age and asks for a refresh after 12 hours. Ask your AI
client to "refresh my digest" (or run `wowc-advisor digest`), then `/reload`. If the capture itself is
old, `/reload` in game first so ForeverScribe writes a new one.

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
