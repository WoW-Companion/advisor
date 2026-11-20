# WoW Companion advisor

An MCP server that gives AI clients deterministic, character-aware World of Warcraft advice
backed by a versioned Parquet game dataset. This skeleton ships one tool, `server_info`, and the
data-access layer the advice tools will build on.

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

## Development

```bash
make verify   # ruff check, ruff format --check, pytest
```

## License

MIT, see [LICENSE](LICENSE).
