# Client setup

The advisor is one MCP server that runs locally over stdio (the default) or streamable HTTP. Every
client below needs two things: the server entry, and the instruction file that tells the model how
to use the tools.

Fastest route: `wowc-advisor setup <client> --savedvariables /path/to/ForeverScribe.lua` prints the
entry for your OS and the file it belongs in; add `--write` to merge it into that file. `--write`
keeps a `.bak` copy first, keeps every other server and setting, and refuses a file it cannot parse
(or a Codex file that already has a `wowc-advisor` table). `<client>` is `claude-desktop`,
`claude-code`, `codex` or `gemini`; `--binary` and `--os` override the detected values.

In the snippets below, `/path/to/wowc-advisor` is the binary you downloaded (on Windows
`wowc-advisor.exe`, written with doubled backslashes in JSON) and `WOWC_SAVEDVARIABLES` is your
`ForeverScribe.lua`. Optionally set `WOWC_DATASET` to a dataset directory. For HTTP, start
`wowc-advisor serve --http --port 8000` and use `http://127.0.0.1:8000/mcp`.

The instruction text is one source, `clients/body.md`; the files under `clients/` are generated
from it and a test fails when they drift.

Each section was checked against the cited pages on 2026-11-21. How each was exercised is in the
last section.

## Claude Desktop

Docs: <https://modelcontextprotocol.io/quickstart/user>

Config file `claude_desktop_config.json` (menu: Settings, Developer, Edit Config):

| OS      | Path |
| ------- | ---- |
| Windows | `%APPDATA%\Claude\claude_desktop_config.json` |
| macOS   | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Linux   | `~/.config/Claude/claude_desktop_config.json` |

```json
{
  "mcpServers": {
    "wowc-advisor": {
      "command": "/path/to/wowc-advisor",
      "args": ["serve", "--stdio"],
      "env": { "WOWC_SAVEDVARIABLES": "/path/to/ForeverScribe.lua" }
    }
  }
}
```

Instructions: zip the folder `clients/claude/wow-companion` (the folder itself at the top of the
ZIP) and upload it under Customize, Skills on claude.ai
(<https://code.claude.com/docs/en/skills>; uploads accept only the frontmatter fields name,
description, license, compatibility, metadata, allowed-tools, which this file uses).

Check it works: fully quit and restart Claude Desktop, then ask "run server_info on wowc-advisor".

## Claude Code

Docs: <https://code.claude.com/docs/en/mcp>, <https://code.claude.com/docs/en/skills>

Config: user and local scope live in `~/.claude.json` on every OS; project scope is `.mcp.json` in
the project root. `wowc-advisor setup claude-code --write` writes the project `.mcp.json` in the
current directory (Claude Code asks you to approve project servers the first time).

```bash
claude mcp add --scope user --env WOWC_SAVEDVARIABLES=/path/to/ForeverScribe.lua --transport stdio \
  wowc-advisor -- /path/to/wowc-advisor serve --stdio
```

or the same entry as the Claude Desktop JSON above, saved as `.mcp.json`.

Instructions: copy `clients/claude/wow-companion` to `~/.claude/skills/wow-companion` (all
projects; on Windows `%USERPROFILE%\.claude\skills\wow-companion`) or `.claude/skills/wow-companion`
in a repository.

Check it works: `claude mcp list` shows `wowc-advisor ... Connected` (a project entry shows
"Pending approval" until you approve it in `claude`).

## OpenAI Codex CLI

Docs: <https://learn.chatgpt.com/docs/extend/mcp?surface=cli> (MCP),
<https://learn.chatgpt.com/docs/agent-configuration/agents-md> (instructions)

Config file `config.toml` in the Codex home, which is `~/.codex` (Windows
`%USERPROFILE%\.codex`) unless `CODEX_HOME` is set.

```toml
[mcp_servers.wowc-advisor]
command = "/path/to/wowc-advisor"
args = ["serve", "--stdio"]

[mcp_servers.wowc-advisor.env]
WOWC_SAVEDVARIABLES = "/path/to/ForeverScribe.lua"
```

or `codex mcp add wowc-advisor --env WOWC_SAVEDVARIABLES=/path/to/ForeverScribe.lua -- /path/to/wowc-advisor serve --stdio`.

Instructions: copy `clients/codex/AGENTS.md` to `~/.codex/AGENTS.md` (all projects), or into a
project root. Codex reads one file per directory, and `AGENTS.override.md` wins if present.

Check it works: `codex mcp get wowc-advisor` shows `enabled: true`; in a session, ask it to call
`server_info`.

## Gemini CLI

Docs: <https://geminicli.com/docs/tools/mcp-server/>, <https://geminicli.com/docs/cli/gemini-md/>

Config file `settings.json`: `~/.gemini/settings.json` (user; Windows
`%USERPROFILE%\.gemini\settings.json`) or `.gemini/settings.json` in a project.

```json
{
  "mcpServers": {
    "wowc-advisor": {
      "command": "/path/to/wowc-advisor",
      "args": ["serve", "--stdio"],
      "env": { "WOWC_SAVEDVARIABLES": "/path/to/ForeverScribe.lua" }
    }
  }
}
```

For HTTP use `"httpUrl": "http://127.0.0.1:8000/mcp"`. The docs advise hyphens, not underscores,
in server names. Gemini CLI ignores MCP servers in folders you have not trusted.

Instructions: copy `clients/gemini/GEMINI.md` to `~/.gemini/GEMINI.md` (all projects) or into a
project root.

Check it works: `gemini mcp list` shows `wowc-advisor ... Connected`.

## How this was verified

`make verify` runs, with no network: golden tests of every snippet, a test that `--write` keeps
other entries, and a test that `serve --stdio` answers `initialize` and `tools/list`, which is the
contract all four clients rely on. Beyond that, on a host with the CLI installed, the entry
`setup --write` produced was loaded by the real client; see the ADV-023 report for what each
session showed. The Claude Desktop app was not run, so its entry is "matches the docs".
