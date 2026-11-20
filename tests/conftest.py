import json

import duckdb
import pytest

ROWS = [
    (1, "Linen Cloth", 1, "common"),
    (2, "Wool Cloth", 1, "common"),
    (3, "Sunfire Staff", 60, "rare"),
    (4, "Linen Bandage", 1, "common"),
]


def write_dataset(directory, manifest=None):
    """Write a tiny synthetic Parquet dataset (not real game data)."""
    con = duckdb.connect(":memory:")
    con.execute("CREATE TABLE items (id INTEGER, name VARCHAR, level INTEGER, quality VARCHAR)")
    con.executemany("INSERT INTO items VALUES (?, ?, ?, ?)", ROWS)
    con.execute(f"COPY items TO '{directory / 'items.parquet'}' (FORMAT PARQUET)")
    if manifest is not None:
        (directory / "manifest.json").write_text(json.dumps(manifest))
    return directory


@pytest.fixture(scope="session")
def dataset(tmp_path_factory):
    return write_dataset(
        tmp_path_factory.mktemp("data"), {"version": "2026.11.1", "game_build": "12345"}
    )
