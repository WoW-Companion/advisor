from pathlib import Path

import pytest
from mcp.client import Client

from wowc_advisor import __version__
from wowc_advisor.server import create_server, server_info


@pytest.mark.parametrize(
    ("use_dataset", "expected"),
    [
        (False, {"dataset": "no dataset configured", "game_build": None}),
        (True, {"dataset": "ok", "manifest_version": "2026.11.1", "game_build": "12345"}),
    ],
)
def test_server_info(dataset, tmp_path, use_dataset, expected):
    info = server_info(dataset if use_dataset else None)
    assert info["version"] == __version__
    assert expected.items() <= info.items()


def test_server_info_no_manifest(tmp_path):
    assert server_info(tmp_path)["dataset"] == "no manifest found"


async def test_smoke_server_info_over_memory_transport(dataset):
    async with Client(create_server(dataset)) as client:
        tools = await client.list_tools()
        assert [t.name for t in tools.tools] == [
            "server_info",
            "list_characters",
            "character_brief",
        ]
        result = await client.call_tool("server_info", {})
    assert result.structured_content["game_build"] == "12345"
    assert result.structured_content["version"] == __version__


FIXTURE = Path(__file__).parent / "fixtures" / "foreverscribe"


@pytest.mark.parametrize(
    ("file", "expected"),
    [
        (None, {"savedvariables": "no SavedVariables file configured", "characters": None}),
        (
            FIXTURE / "two_characters.lua",
            {
                "savedvariables": "ok",
                "characters": 2,
                "characters_captured_at": "2023-11-18T09:33:20+00:00",
            },
        ),
        (FIXTURE / "missing.lua", {"savedvariables": "file not found", "characters": None}),
    ],
)
def test_server_info_savedvariables(file, expected):
    assert expected.items() <= server_info(None, file).items()


def test_server_info_savedvariables_parse_error(tmp_path):
    bad = tmp_path / "ForeverScribe.lua"
    bad.write_text("ForeverScribeDB = {")
    info = server_info(None, bad)
    assert info["savedvariables"].startswith("could not parse: line 1")


async def test_server_info_reports_characters_over_transport():
    async with Client(create_server(None, FIXTURE / "level1.lua")) as client:
        result = await client.call_tool("server_info", {})
    assert result.structured_content["characters"] == 1


async def test_smoke_character_tools_over_transport():
    async with Client(create_server(None, FIXTURE / "level30_caster.lua")) as client:
        listed = await client.call_tool("list_characters", {})
        brief = await client.call_tool("character_brief", {})
        missing = await client.call_tool("character_brief", {"character": "Nobody"})
    assert listed.structured_content["result"][0]["name"] == "Testmage"
    assert "Spec: Test Class Tree (3 points)" in brief.content[0].text
    assert "ForeverScribe capture at" in brief.content[0].text
    assert missing.is_error


async def test_character_tools_without_file_say_so():
    async with Client(create_server()) as client:
        result = await client.call_tool("list_characters", {})
    assert result.is_error
    assert "No SavedVariables file configured" in result.content[0].text
