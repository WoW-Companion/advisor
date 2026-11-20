"""The MCP server and its tool registry, shared by every transport."""

from __future__ import annotations

import os
from datetime import UTC, datetime
from pathlib import Path

from mcp.server.mcpserver import MCPServer
from mcp.server.mcpserver.exceptions import ToolError

from wowc_advisor import __version__
from wowc_advisor.brief import CharacterLookupError, choose_character
from wowc_advisor.brief import character_brief as build_brief
from wowc_advisor.brief import list_characters as build_list
from wowc_advisor.character import LuaParseError, load_savedvariables
from wowc_advisor.store import DuckDBStore

DATASET_ENV = "WOWC_DATASET"
SAVEDVARIABLES_ENV = "WOWC_SAVEDVARIABLES"


def _character_info(path: Path | None) -> dict[str, str | int | None]:
    """Describe the configured ForeverScribe file: character count and capture time."""
    info: dict[str, str | int | None] = {
        "savedvariables_path": None,
        "characters": None,
        "characters_captured_at": None,
        "savedvariables": "no SavedVariables file configured",
    }
    if path is None:
        return info
    info["savedvariables_path"] = str(path)
    try:
        loaded = load_savedvariables(path)
    except FileNotFoundError:
        info["savedvariables"] = "file not found"
    except (OSError, UnicodeDecodeError) as err:
        info["savedvariables"] = f"unreadable: {err.__class__.__name__}"
    except LuaParseError as err:
        info["savedvariables"] = f"could not parse: {err}"
    else:
        info["savedvariables"] = "ok"
        info["characters"] = len(loaded.characters)
        if loaded.captured_at is not None:
            info["characters_captured_at"] = datetime.fromtimestamp(
                loaded.captured_at, UTC
            ).isoformat()
    return info


def server_info(
    dataset: Path | None, savedvariables: Path | None = None
) -> dict[str, str | int | None]:
    """Describe this server, the dataset it found and the character file it was given."""
    info: dict[str, str | int | None] = {
        **_character_info(savedvariables),
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


def create_server(
    dataset: Path | str | None = None, savedvariables: Path | str | None = None
) -> MCPServer:
    """Build the server. Defaults come from $WOWC_DATASET and $WOWC_SAVEDVARIABLES."""
    chosen = dataset or os.environ.get(DATASET_ENV)
    path = Path(chosen) if chosen else None
    chosen_sv = savedvariables or os.environ.get(SAVEDVARIABLES_ENV)
    sv_path = Path(chosen_sv) if chosen_sv else None
    mcp = MCPServer("wowc-advisor", version=__version__)

    @mcp.tool(name="server_info")
    def _server_info() -> dict[str, str | int | None]:
        """Return the advisor version, dataset and game build, and the character file's status."""
        return server_info(path, sv_path)

    def _load():
        if sv_path is None:
            raise ToolError("No SavedVariables file configured (set WOWC_SAVEDVARIABLES).")
        try:
            return load_savedvariables(sv_path)
        except FileNotFoundError:
            raise ToolError(f"SavedVariables file not found: {sv_path}") from None
        except (OSError, UnicodeDecodeError, LuaParseError) as err:
            raise ToolError(f"Could not read the SavedVariables file: {err}") from None

    @mcp.tool(name="list_characters")
    def _list_characters() -> list[dict[str, str | int | None]]:
        """List the characters in the ForeverScribe capture: name, level, class, realm, capture time."""
        return build_list(_load())

    @mcp.tool(name="character_brief")
    def _character_brief(character: str | None = None) -> str:
        """Brief of one captured character (by name; omit if only one): level, class, race, XP,
        money, spec, gear, professions, quests and the capture time. Source: ForeverScribe capture."""
        try:
            return build_brief(choose_character(_load(), character))
        except CharacterLookupError as err:
            raise ToolError(str(err)) from None

    return mcp
