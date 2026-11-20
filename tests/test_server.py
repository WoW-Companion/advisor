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
        assert [t.name for t in tools.tools] == ["server_info"]
        result = await client.call_tool("server_info", {})
    assert result.structured_content["game_build"] == "12345"
    assert result.structured_content["version"] == __version__
