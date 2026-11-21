# /ask: answering questions from the game

**The overlay answers these for you.** A question queued in game (right-click "Ask about this", or `/ask`)
lands in the addon's SavedVariables (`WoWCompanion.lua`, next to `ForeverScribe.lua`). With the overlay (or
`wowc-advisor companion`) running, it shows up in the overlay chat, marked "asked in game", on the next
`/reload`, and is answered there, streaming, once each. Nothing is written to the game folder.

**Deprecated: `--answers-file`.** `wowc-advisor ask` is for anyone still on an older addon that has the
`WoWCompanion_Answers` slot. It runs a headless agent CLI per unanswered question and writes
`Interface/AddOns/WoWCompanion_Answers/Answers.lua` (schema 1) only when you pass `--answers-file`;
`companion --answers-file` does the same alongside the overlay chat. Without the flag, `ask` stops and
points at the overlay. The folder can be deleted if you do not use it.

```
wowc-advisor ask --answers-file --agent claude --savedvariables /path/to/ForeverScribe.lua
wowc-advisor ask --answers-file --agent claude --watch     # poll the file; remind to /reload after each pass
```

## The agent

`--agent` takes a preset or a command; without it, `"agent"` is read from
`<user config dir>/wowc-advisor/ask.json` (`--config` to move it): `{"agent": "codex"}`.

| Preset | Runs |
| ------ | ---- |
| `claude` | `claude -p <prompt> --mcp-config <file> --strict-mcp-config --allowedTools mcp__wowc-advisor --disallowedTools Bash Edit Write NotebookEdit WebFetch WebSearch Read Glob Grep` |
| `codex` | `codex exec --skip-git-repo-check -s read-only -c 'mcp_servers.wowc-advisor={ command, args, env, default_tools_approval_mode = "approve" }' <prompt>` |
| `gemini` | `gemini --allowed-mcp-server-names wowc-advisor --policy <file> -p <prompt>` |

### What each preset allows

A headless run cannot ask permission, so a tool that is not pre-approved is refused. Each preset
pre-approves the advisor's tools and nothing else. They are read-only, except `update_digest`, which
writes the digest file.

| Preset | Advisor tools | Everything else | The player's other MCP servers |
| ------ | ------------- | --------------- | ------------------------------ |
| `claude` | allowed (`mcp__wowc-advisor`, the whole server) | denied by name: shell, edit, write, web, file read and search | not loaded (`--strict-mcp-config`) |
| `codex` | server defined on the command line (nothing is written to `~/.codex`), its tools set to `approve` | read-only sandbox; no approval bypass | still loaded from the player's Codex config |
| `gemini` | `--policy` file (generated beside the MCP config) allows server `wowc-advisor` | the same file denies shell, file writes and web fetch/search; no `--yolo` | only `wowc-advisor` is enabled (`--allowed-mcp-server-names`) |

Gemini does not take an MCP server on the command line: the server must already be in
`~/.gemini/settings.json` (`wowc-advisor setup gemini`, see `docs/clients.md`), and the preset only
approves it. Codex's `exec` mode has had cases of cancelling MCP calls it should have approved; if
Codex answers that the tools were not available, update Codex or use `--agent claude`.

The prompt tells the agent to use the tools for anything about the character, never to tell the player
to change permissions or settings, and, if the tools are refused or missing, to answer "The advisor's
tools weren't available to the AI client (setup problem)."

### The config file

| System | Path |
| ------ | ---- |
| Windows | `%AppData%\wowc-advisor\ask.json` |
| macOS | `~/Library/Application Support/wowc-advisor/ask.json` |
| Linux | `$XDG_CONFIG_HOME/wowc-advisor/ask.json`, else `~/.config/wowc-advisor/ask.json` |

`wowc-advisor config path` prints it; `wowc-advisor config set agent <preset|command|auto>` writes the
`"agent"` field and keeps the file's other keys (an empty value removes it).

### Auto-detect (companion and the overlay)

`wowc-advisor companion` starts with no agent at all: the character list, game-data badge, NEXT strip and
digest work, and asking answers "No AI client set up yet…". When no `--agent` or config value is set (or the
value is `auto`) it looks for `claude`, `codex` and `gemini`, in that order, and uses the first found. For
each name it checks every directory of `PATH` first, then these (`.exe`, `.cmd`, `.bat` and `.com` on Windows):

| System | Extra directories |
| ------ | ----------------- |
| Windows | `%USERPROFILE%\.local\bin`, `%USERPROFILE%\.claude\local`, `%APPDATA%\npm`, `%LOCALAPPDATA%\Programs\claude`, `%USERPROFILE%\scoop\shims`, `%ProgramFiles%\nodejs` |
| macOS, Linux | `~/.local/bin`, `~/.claude/local`, `~/.npm-global/bin`, `/opt/homebrew/bin`, `/usr/local/bin` |

An explicit `--agent` or config value always wins. `companion --handshake` reports the result in its JSON
line: `"agent":"claude","agent_source":"auto"` (`agent_source` is `flag`, `config`, `auto` or `none`).
`ask` and `ask --watch` do not auto-detect; they still need an agent.

A custom command gets the prompt as its last argument, or wherever `{prompt}` / `{mcp_config}` appear.
The agent's environment also has `WOWC_QUESTION`, `WOWC_CHARACTER` and `WOWC_MCP_CONFIG`. The MCP
config starts `wowc-advisor serve --stdio` on your capture; claude and codex are handed it, gemini needs the
same server in its own settings (see `docs/clients.md`). The capture file itself is never sent; the agent
gets the question and the character key, and reads facts through the tools.

## Limits

stdout is the answer, cut to 1,500 characters (at most 64 KB is read). Each run has a timeout
(`--timeout`, default 2m) and a pass answers at most `--max` questions (default 5); the rest wait for
the next pass. A failure or timeout becomes a short "Could not answer: ..." answer, and the file stays
valid. If the output names one of the advisor's tools, it is listed on the answer's sources line.
Answers already in the file are kept while their question is still queued. The file is checked against
the slot rule (one data-only assignment) before an atomic replace.
