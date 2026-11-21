# Try it: Claude Desktop on Windows

This sets up the advisor so Claude can answer questions about your own character.
Sources read on 2026-11-20 are cited per step. A step marked **unverified** was not confirmed
against current documentation, and none of it was run in a real Claude Desktop yet.

## 1. Install the ForeverScribe addon

Install [ForeverScribe](https://github.com/MattMaximo/foreverscribe) into your game's `AddOns`
folder, log in with your character, then `/reload` (or log out). The addon writes your character
data when you do. Addon installation steps are **unverified** here; follow the addon's own README.

## 2. Find your SavedVariables file

The Forever game folder is `World of Warcraft\_forever_`. The file is:

```
World of Warcraft\_forever_\WTF\Account\<ACCOUNT>\SavedVariables\ForeverScribe.lua
```

The `_forever_` folder name is from the project brief; the `WTF\Account\...\SavedVariables` layout
is the standard WoW layout and is **unverified** for Forever. Use the file that actually exists.

## 3. Install the advisor

Download `wowc-advisor-windows-amd64.exe` (or `-arm64.exe` on an ARM PC) from the
[releases page](https://github.com/WoW-Companion/advisor/releases), and save it, for example, as
`C:\Users\you\wowc-advisor.exe`. It is one static file: nothing else to install. Developers can
build it with `make build` or `go install github.com/WoW-Companion/advisor/cmd/wowc-advisor@latest`.
Whether a release is published yet is **unverified**.

## 4. Add the server to Claude Desktop

Source: <https://modelcontextprotocol.io/quickstart/user> (also [clients.md](clients.md)).
Edit `%APPDATA%\Claude\claude_desktop_config.json` and add this, using forward slashes or doubled
backslashes in the paths, then fully restart Claude Desktop:

```json
{
  "mcpServers": {
    "wowc-advisor": {
      "command": "C:/Users/you/wowc-advisor.exe",
      "args": ["serve", "--stdio"],
      "env": {
        "WOWC_SAVEDVARIABLES": "C:/Program Files (x86)/World of Warcraft/_forever_/WTF/Account/ACCOUNT/SavedVariables/ForeverScribe.lua"
      }
    }
  }
}
```

Ask Claude to call `server_info`: it reports whether the file was found, how many characters it
holds and the game client version and build that wrote the capture. Use the full path to the `.exe` in `command`. Windows may ask you to confirm running a downloaded
program the first time (the binary is not code-signed).

### The game data (automatic)

The advisor needs the published game dataset for your game build. You do nothing: on the first
`serve` it reads the client build from your capture, downloads the matching release from
<https://github.com/WoW-Companion/data/releases> (a `dataset-<build>` release), checks every file's
sha256 against the release manifest and keeps it in a per-user cache
(`%LOCALAPPDATA%\wowc\dataset\<build>` on Windows, `~/Library/Caches/wowc/dataset/<build>` on macOS,
`$XDG_CACHE_HOME/wowc/dataset/<build>` or `~/.cache/wowc/dataset/<build>` on Linux). A download that
fails a check or stops half way is thrown away and the cache is untouched. Once cached it works
offline. `server_info` reports the dataset build and warns when it differs from your client's build.

- `wowc-advisor dataset status` lists the cached builds and whether each still verifies.
- `wowc-advisor doctor --savedvariables /path/to/ForeverScribe.lua` checks the whole setup (the capture and its age, the three addons and their versions, the dataset cache and its build, the digest and answers files, and which client configs mention wowc-advisor) and prints a fix for each failure. Your AI client can run the same check with the `doctor` tool.
- `wowc-advisor dataset update` fetches the capture's build, or the newest release without a capture.
  Use `--build 71037` to name one.
- `serve --dataset DIR` (or `WOWC_DATASET`) uses your own directory instead; `--cache DIR` moves the cache.

Whether a release is published yet is **unverified**: the first real download is not yet tested.

## 5. Install the skill

Source: <https://support.claude.com/en/articles/12512180-using-skills-in-claude>

1. Turn on **Settings > Capabilities > Code execution and file creation** (skills need it).
2. Make a ZIP of the `clients/claude/wow-companion` folder (the folder itself at the top of the ZIP).
3. In Claude go to **Customize > Skills**, click **+**, **+ Create skill**, **Upload a skill**,
   and choose the ZIP. Switch the skill on.

The folder name must match the skill name (`wow-companion`). The article does not state the ZIP
size limit; whether Claude Desktop shows the same menu as claude.ai is **unverified**.

## 6. Three questions to try

1. "Which of my characters did the advisor find, and when was each captured?"
2. "Give me a brief of my character. What am I wearing and which slots are empty?"
3. "What quests am I on, and which are ready to hand in?"

Each answer should name the "ForeverScribe capture at ..." time, and say plainly if anything was
not captured or the capture is more than an hour old (then `/reload` in game and ask again).
