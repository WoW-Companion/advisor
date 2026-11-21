# /ask: answering questions from the game

`/ask <question>` in game queues the question in the addon's SavedVariables (`WoWCompanion.lua`,
next to `ForeverScribe.lua`). `wowc-advisor ask` reads the unanswered ones, runs a headless agent
CLI for each, and writes `Interface/AddOns/WoWCompanion_Answers/Answers.lua` (schema 1, see
`docs/ask-format.md` in the addon repo). `/reload` in game shows the answers.

```
wowc-advisor ask --agent claude --savedvariables /path/to/ForeverScribe.lua
wowc-advisor ask --agent claude --watch     # poll the file; remind to /reload after each pass
```

## The agent

`--agent` takes a preset or a command; without it, `"agent"` is read from
`<user config dir>/wowc-advisor/ask.json` (`--config` to move it): `{"agent": "codex"}`.

| Preset | Runs |
| ------ | ---- |
| `claude` | `claude -p <prompt> --mcp-config <file>` |
| `codex` | `codex exec <prompt>` |
| `gemini` | `gemini -p <prompt>` |

A custom command gets the prompt as its last argument, or wherever `{prompt}` / `{mcp_config}` appear.
The agent's environment also has `WOWC_QUESTION`, `WOWC_CHARACTER` and `WOWC_MCP_CONFIG`. The MCP
config starts `wowc-advisor serve --stdio` on your capture: codex and gemini need the same server
added in their own settings (see `docs/clients.md`). The capture file itself is never sent; the agent
gets the question and the character key, and reads facts through the tools.

## Limits

stdout is the answer, cut to 1,500 characters (at most 64 KB is read). Each run has a timeout
(`--timeout`, default 2m) and a pass answers at most `--max` questions (default 5); the rest wait for
the next pass. A failure or timeout becomes a short "Could not answer: ..." answer, and the file stays
valid. If the output names one of the advisor's tools, it is listed on the answer's sources line.
Answers already in the file are kept while their question is still queued. The file is checked against
the slot rule (one data-only assignment) before an atomic replace.
