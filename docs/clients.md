# Client setup

The server runs locally over stdio (the default) or streamable HTTP. In every entry below,
replace `/path/to/advisor` with your checkout of this repository, and optionally set
`WOWC_DATASET` to a dataset directory (see `server_info` for what the server found).

The stdio command is `uv run --directory /path/to/advisor wowc-advisor serve --stdio`.
For HTTP, start `uv run wowc-advisor serve --http --port 8000`; the endpoint is
`http://127.0.0.1:8000/mcp`.

Each section was checked against the cited page on 2026-11-20. Nothing here was run inside the
client itself, so treat the entries as "matches the docs", not "tested in the client".

## Claude Desktop

Source: <https://modelcontextprotocol.io/quickstart/user>

Edit `claude_desktop_config.json` (macOS: `~/Library/Application Support/Claude/`, Windows:
`%APPDATA%\Claude\`, Linux: `~/.config/Claude/`), then restart Claude Desktop:

```json
{
  "mcpServers": {
    "wowc-advisor": {
      "command": "uv",
      "args": ["run", "--directory", "/path/to/advisor", "wowc-advisor", "serve", "--stdio"],
      "env": { "WOWC_DATASET": "/path/to/dataset" }
    }
  }
}
```

## Claude Code

Source: <https://code.claude.com/docs/en/mcp>

```bash
claude mcp add --env WOWC_DATASET=/path/to/dataset --transport stdio wowc-advisor \
  -- uv run --directory /path/to/advisor wowc-advisor serve --stdio

# or against a running HTTP server
claude mcp add --transport http wowc-advisor http://127.0.0.1:8000/mcp
```

Check the connection with `/mcp` inside Claude Code; `claude mcp add` does not validate it.

## Codex

Source: <https://learn.chatgpt.com/docs/extend/mcp?surface=cli>
(the older `developers.openai.com/codex/mcp` address redirects there)

```bash
codex mcp add wowc-advisor --env WOWC_DATASET=/path/to/dataset \
  -- uv run --directory /path/to/advisor wowc-advisor serve --stdio
```

Or in `~/.codex/config.toml`:

```toml
[mcp_servers.wowc-advisor]
command = "uv"
args = ["run", "--directory", "/path/to/advisor", "wowc-advisor", "serve", "--stdio"]

[mcp_servers.wowc-advisor.env]
WOWC_DATASET = "/path/to/dataset"
```

For HTTP use `url = "http://127.0.0.1:8000/mcp"` in place of `command`/`args`.

## Gemini CLI

Source: <https://geminicli.com/docs/tools/mcp-server/>

In `~/.gemini/settings.json` (user) or `.gemini/settings.json` (project):

```json
{
  "mcpServers": {
    "wowc-advisor": {
      "command": "uv",
      "args": ["run", "--directory", "/path/to/advisor", "wowc-advisor", "serve", "--stdio"],
      "env": { "WOWC_DATASET": "/path/to/dataset" }
    }
  }
}
```

For HTTP, use `"httpUrl": "http://127.0.0.1:8000/mcp"` instead. The docs advise hyphens, not
underscores, in server names.
