"""The data-access seam: a protocol and a DuckDB implementation over a Parquet directory."""

from __future__ import annotations

import json
import re
from collections.abc import Mapping
from pathlib import Path
from typing import Any, Protocol, runtime_checkable

import duckdb

Row = dict[str, Any]
_IDENT = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")


def _ident(name: str) -> str:
    """Quote a table or column name, refusing anything that is not a plain identifier."""
    if not _IDENT.match(name):
        raise ValueError(f"invalid identifier: {name!r}")
    return f'"{name}"'


@runtime_checkable
class DataStore(Protocol):
    """Read access to game data tables."""

    def get(self, table: str, id: int | str) -> Row | None:
        """Return the row whose `id` column equals `id`, or None."""
        ...

    def filter(self, table: str, **columns: Any) -> list[Row]:
        """Return rows where every named column equals its value."""
        ...

    def search_name(self, table: str, text: str, limit: int = 20) -> list[Row]:
        """Return rows whose `name` column contains `text`, case-insensitively."""
        ...


class DuckDBStore:
    """A DataStore reading `<directory>/<table>.parquet` with DuckDB."""

    def __init__(self, directory: Path | str) -> None:
        self.directory = Path(directory)
        self._con = duckdb.connect(":memory:")

    def manifest(self) -> Mapping[str, Any] | None:
        path = self.directory / "manifest.json"
        if not path.is_file():
            return None
        return json.loads(path.read_text(encoding="utf-8"))

    def _source(self, table: str) -> str:
        _ident(table)
        path = self.directory / f"{table}.parquet"
        if not path.is_file():
            raise KeyError(f"no such table: {table}")
        return str(path)

    def _query(self, table: str, where: str, params: list[Any], tail: str = "") -> list[Row]:
        sql = f"SELECT * FROM read_parquet(?) {where} {tail}"
        cur = self._con.execute(sql, [self._source(table), *params])
        names = [d[0] for d in cur.description]
        return [dict(zip(names, row, strict=True)) for row in cur.fetchall()]

    def get(self, table: str, id: int | str) -> Row | None:
        rows = self._query(table, "WHERE id = ?", [id], "LIMIT 1")
        return rows[0] if rows else None

    def filter(self, table: str, **columns: Any) -> list[Row]:
        clauses = [f"{_ident(c)} = ?" for c in columns]
        where = "WHERE " + " AND ".join(clauses) if clauses else ""
        return self._query(table, where, list(columns.values()), "ORDER BY id")

    def search_name(self, table: str, text: str, limit: int = 20) -> list[Row]:
        return self._query(
            table, "WHERE name ILIKE ?", [f"%{text}%"], f"ORDER BY id LIMIT {int(limit)}"
        )
