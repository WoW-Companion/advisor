# WoW Companion advisor

An MCP server that gives AI clients deterministic, character-aware World of Warcraft advice
backed by a versioned Parquet game dataset. Tools today: `server_info`, `list_characters` and
`character_brief` (a brief of your captured character). A Claude skill and a Windows setup guide
are in [clients/](clients/) and [docs/try-it.md](docs/try-it.md).

**New here? Start with the [quickstart](docs/quickstart.md):** one page from download to in-game advice.

> WoW Companion is a fan project. It is not affiliated with or endorsed by Blizzard Entertainment. World of Warcraft and Warcraft are trademarks of Blizzard Entertainment, Inc.

## Quick start

Download the binary for your platform (Windows, macOS or Linux; amd64 or arm64) from the
[releases page](https://github.com/WoW-Companion/advisor/releases). It is one static file with no
dependencies.

```bash
wowc-advisor serve --stdio                 # default transport
wowc-advisor serve --http --port 8000      # streamable HTTP at /mcp
```

Developers can install it with Go instead:
`go install github.com/WoW-Companion/advisor/cmd/wowc-advisor@latest`.

Point the server at a dataset directory (Parquet tables plus `manifest.json`) with
`--dataset DIR` or `WOWC_DATASET`. Without one, `server_info` reports "no dataset configured".

Client configuration for Claude Desktop, Claude Code, Codex and Gemini CLI is in
[docs/clients.md](docs/clients.md).

## Character data

Character-aware advice reads the file the [ForeverScribe](https://github.com/MattMaximo/foreverscribe)
addon (MIT) writes when you `/reload` or log out:
`WTF/Account/<account>/SavedVariables/ForeverScribe.lua` inside your game folder (on Windows,
`World of Warcraft\_forever_\WTF\Account\<ACCOUNT>\SavedVariables\ForeverScribe.lua`). Point the server
at it with `--savedvariables FILE` or `WOWC_SAVEDVARIABLES`; `server_info` then reports how many
characters it found, when they were captured and which game client version and build wrote them. The file is only read, on your machine, by a
parser that never executes it; it is never uploaded or written to. The format is documented in
[docs/foreverscribe-format.md](docs/foreverscribe-format.md).

## Development

```bash
make verify   # gofmt -l, go vet, go test
make build    # static binaries for six platforms in dist/ (git-ignored)
make bench    # DataStore benchmarks: point lookups and a range filter on a 32,000 row table
```

Go only; CGO is disabled for every build. Layout: `cmd/wowc-advisor` (the command) and `internal/`
(`lua` parser, `character` model, `brief`, `server` tools, `store` Parquet DataStore).

## License

MIT, see [LICENSE](LICENSE).
