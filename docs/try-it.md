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

Install [uv](https://docs.astral.sh/uv/) and Python 3.12+, then clone this repository, for
example to `C:\Users\you\advisor`, and run `uv sync` there. (Windows install commands for uv:
see its docs; **unverified** here.)

## 4. Add the server to Claude Desktop

Source: <https://modelcontextprotocol.io/quickstart/user> (also [clients.md](clients.md)).
Edit `%APPDATA%\Claude\claude_desktop_config.json` and add this, using forward slashes or doubled
backslashes in the paths, then fully restart Claude Desktop:

```json
{
  "mcpServers": {
    "wowc-advisor": {
      "command": "uv",
      "args": ["run", "--directory", "C:/Users/you/advisor", "wowc-advisor", "serve", "--stdio"],
      "env": {
        "WOWC_SAVEDVARIABLES": "C:/Program Files (x86)/World of Warcraft/_forever_/WTF/Account/ACCOUNT/SavedVariables/ForeverScribe.lua"
      }
    }
  }
}
```

Ask Claude to call `server_info`: it reports whether the file was found and how many characters it
holds. Whether Claude Desktop finds `uv` on your Windows PATH is **unverified**; if it does not,
put the full path to `uv.exe` in `command`.

## 5. Install the skill

Source: <https://support.claude.com/en/articles/12512180-using-skills-in-claude>

1. Turn on **Settings > Capabilities > Code execution and file creation** (skills need it).
2. Make a ZIP of the `skills/claude/wow-companion` folder (the folder itself at the top of the ZIP).
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
