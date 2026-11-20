# WoW Companion advisor

An MCP server that gives AI clients deterministic, character-aware World of Warcraft advice
backed by a versioned Parquet game dataset. Tools today: `server_info`, `list_characters` and
`character_brief` (a brief of your captured character). A Claude skill and a Windows setup guide
are in [skills/](skills/) and [docs/try-it.md](docs/try-it.md).

> WoW Companion is a fan project. It is not affiliated with or endorsed by Blizzard Entertainment. World of Warcraft and Warcraft are trademarks of Blizzard Entertainment, Inc.

## Quick start

Requires [uv](https://docs.astral.sh/uv/) and Python 3.12+.

```bash
uv sync
uv run wowc-advisor serve --stdio                 # default transport
uv run wowc-advisor serve --http --port 8000      # streamable HTTP at /mcp
```

Point the server at a dataset directory (Parquet tables plus `manifest.json`) with
`--dataset DIR` or `WOWC_DATASET`. Without one, `server_info` reports "no dataset configured".

Client configuration for Claude Desktop, Claude Code, Codex and Gemini CLI is in
[docs/clients.md](docs/clients.md).

## Character data

Character-aware advice reads the file the [ForeverScribe](https://github.com/MattMaximo/foreverscribe)
addon (MIT) writes when you `/reload` or log out:
`WTF/Account/<account>/SavedVariables/ForeverScribe.lua` inside your game folder. Point the server
at it with `--savedvariables FILE` or `WOWC_SAVEDVARIABLES`; `server_info` then reports how many
characters it found and when they were captured. The file is only read, on your machine, by a
parser that never executes it; it is never uploaded or written to. The format is documented in
[docs/foreverscribe-format.md](docs/foreverscribe-format.md).

## Development

```bash
make verify   # ruff check, ruff format --check, pytest
```

## License

MIT, see [LICENSE](LICENSE).
