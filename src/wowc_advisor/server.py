"""The MCP server and its tool registry, shared by every transport."""

from __future__ import annotations

import os
from pathlib import Path

from mcp.server.mcpserver import MCPServer

from wowc_advisor import __version__
from wowc_advisor.store import DuckDBStore

DATASET_ENV = "WOWC_DATASET"


def server_info(dataset: Path | None) -> dict[str, str | None]:
    """Describe this server and the dataset it found."""
    info: dict[str, str | None] = {
        "version": __version__,
        "dataset_path": None,
        "manifest_version": None,
        "game_build": None,
        "dataset": "no dataset configured",
    }
    if dataset is None:
        return info
    info["dataset_path"] = str(dataset)
    manifest = DuckDBStore(dataset).manifest()
    if manifest is None:
        info["dataset"] = "no manifest found"
        return info
    info["dataset"] = "ok"
    info["manifest_version"] = str(manifest.get("version"))
    info["game_build"] = str(manifest.get("game_build"))
    return info


def create_server(dataset: Path | str | None = None) -> MCPServer:
    """Build the server. The dataset directory defaults to $WOWC_DATASET."""
    chosen = dataset or os.environ.get(DATASET_ENV)
    path = Path(chosen) if chosen else None
    mcp = MCPServer("wowc-advisor", version=__version__)

    @mcp.tool(name="server_info")
    def _server_info() -> dict[str, str | None]:
        """Return the advisor version, the dataset path and manifest version, and the game build."""
        return server_info(path)

    return mcp
