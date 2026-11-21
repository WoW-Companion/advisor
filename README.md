# WoW Companion advisor

An MCP server that gives AI clients deterministic, character-aware World of Warcraft advice
backed by a versioned Parquet game dataset. Tools today: `server_info`, `list_characters` and
`character_brief` (a brief of your captured character). A Claude skill and a Windows setup guide
are in [clients/](clients/) and [docs/try-it.md](docs/try-it.md).

**New here? Start with the [quickstart](docs/quickstart.md):** one page from download to in-game advice.

> WoW Companion is a fan project. It is not affiliated with or endorsed by Blizzard Entertainment. World of Warcraft and Warcraft are trademarks of Blizzard Entertainment, Inc.

## The three pieces

- **Addon** ([WoW-Companion/addon](https://github.com/WoW-Companion/addon), with
  [ForeverScribe](https://github.com/MattMaximo/foreverscribe)): the in-game panel and the `/ask` box. It
  only reads game state and shows the digest.
- **Advisor** (this repo, `wowc-advisor`): the brain. An MCP server and command line that read your
  ForeverScribe capture and a game dataset, answer with ranked advice, and write the digest.
  `wowc-advisor companion` is a browser chat beside the game.
- **Overlay** ([overlay/](overlay/README.md)): a small Windows app that bundles the advisor, sits over the
  game (Windowed (Fullscreen) only), finds your AI client and chats about the character you are playing.

Setup for all three, step by step, is the [quickstart](docs/quickstart.md). Developers can run the advisor
alone: `wowc-advisor serve --stdio` (default) or `serve --http --port 8000` (streamable HTTP at `/mcp`).
Binaries are on the [releases page](https://github.com/WoW-Companion/advisor/releases) once published, or
`$env:GOPROXY="direct"; go install github.com/WoW-Companion/advisor/cmd/wowc-advisor@main`.

Point the server at a dataset directory (Parquet tables plus `manifest.json`) with
`--dataset DIR` or `WOWC_DATASET`. Without one, `server_info` reports "no dataset configured".

Client configuration for Claude Desktop, Claude Code, Codex and Gemini CLI is in
[docs/clients.md](docs/clients.md).

## Character data

Character-aware advice reads the file the [ForeverScribe](https://github.com/MattMaximo/foreverscribe)
addon (MIT) writes when you `/reload` or log out:
`WTF/Account/<account>/SavedVariables/ForeverScribe.lua` inside your game folder (on Windows,
`World of Warcraft\_forever_\WTF\Account\<ACCOUNT>\SavedVariables\ForeverScribe.lua`). The advisor finds it itself on a standard install; otherwise point it
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
