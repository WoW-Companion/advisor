"""A safe reader for the Lua table-literal subset that SavedVariables files use.

The file is a sequence of `Name = <value>` assignments. Values are tables, strings, numbers,
booleans and nil. Nothing is executed: the text is tokenised and parsed by hand. Every Lua table
becomes a `dict`; positional entries get the integer keys 1..n, as Lua numbers them.
"""

from __future__ import annotations

import re
from typing import Any

LuaValue = Any


class LuaParseError(ValueError):
    """Malformed SavedVariables text. The message names the 1-based line."""

    def __init__(self, message: str, line: int):
        super().__init__(f"line {line}: {message}")
        self.line = line


_NUMBER = re.compile(r"-?(?:0[xX][0-9a-fA-F]+|(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?)")
_NAME = re.compile(r"[A-Za-z_][A-Za-z0-9_]*")
_LONG_OPEN = re.compile(r"\[(=*)\[")
_SIMPLE_ESCAPES = {
    "a": "\a", "b": "\b", "f": "\f", "n": "\n", "r": "\r", "t": "\t", "v": "\v",
    "\\": "\\", '"': '"', "'": "'", "\n": "\n",
}  # fmt: skip


class _Parser:
    def __init__(self, text: str):
        self.text = text
        self.pos = 0

    def line(self, pos: int | None = None) -> int:
        return self.text.count("\n", 0, self.pos if pos is None else pos) + 1

    def error(self, message: str, pos: int | None = None) -> LuaParseError:
        return LuaParseError(message, self.line(pos))

    def skip(self) -> None:
        """Skip whitespace and comments."""
        text = self.text
        while self.pos < len(text):
            ch = text[self.pos]
            if ch.isspace():
                self.pos += 1
            elif text.startswith("--", self.pos):
                self.pos += 2
                m = _LONG_OPEN.match(text, self.pos)
                if m:
                    self.long_string(m)
                else:
                    end = text.find("\n", self.pos)
                    self.pos = len(text) if end < 0 else end
            else:
                return

    def long_string(self, opener: re.Match[str]) -> str:
        start = self.pos
        close = "]" + opener.group(1) + "]"
        end = self.text.find(close, opener.end())
        if end < 0:
            raise self.error("unterminated long string", start)
        body = self.text[opener.end() : end]
        self.pos = end + len(close)
        return body.removeprefix("\n")

    def peek(self) -> str:
        return self.text[self.pos] if self.pos < len(self.text) else ""

    def expect(self, ch: str) -> None:
        self.skip()
        if self.peek() != ch:
            found = repr(self.peek()) if self.peek() else "end of file"
            raise self.error(f"expected {ch!r}, found {found}")
        self.pos += 1

    def string(self) -> str:
        start = self.pos
        quote = self.text[self.pos]
        self.pos += 1
        out: list[str] = []
        text = self.text
        while True:
            if self.pos >= len(text) or text[self.pos] == "\n":
                raise self.error("unterminated string", start)
            ch = text[self.pos]
            self.pos += 1
            if ch == quote:
                return "".join(out)
            if ch != "\\":
                out.append(ch)
                continue
            if self.pos >= len(text):
                raise self.error("unterminated string", start)
            esc = text[self.pos]
            if esc in _SIMPLE_ESCAPES:
                out.append(_SIMPLE_ESCAPES[esc])
                self.pos += 1
            elif esc.isdigit():
                digits = re.match(r"\d{1,3}", text[self.pos :]).group()  # type: ignore[union-attr]
                if int(digits) > 255:
                    raise self.error(f"decimal escape \\{digits} out of range")
                out.append(chr(int(digits)))
                self.pos += len(digits)
            else:
                raise self.error(f"invalid escape sequence \\{esc}")

    def value(self) -> LuaValue:
        self.skip()
        ch = self.peek()
        if not ch:
            raise self.error("expected a value, found end of file")
        if ch == "{":
            return self.table()
        if ch in "\"'":
            return self.string()
        if ch == "[":
            m = _LONG_OPEN.match(self.text, self.pos)
            if m:
                return self.long_string(m)
        m = _NUMBER.match(self.text, self.pos)
        if m:
            self.pos = m.end()
            raw = m.group()
            if re.match(r"-?0[xX]", raw):
                return int(raw, 16)
            return float(raw) if re.search(r"[.eE]", raw) else int(raw)
        m = _NAME.match(self.text, self.pos)
        if m:
            word = m.group()
            literals = {"true": True, "false": False, "nil": None}
            if word in literals:
                self.pos = m.end()
                return literals[word]
            raise self.error(f"unexpected identifier {word!r} (only literals are allowed)")
        raise self.error(f"unexpected character {ch!r}")

    def table(self) -> dict[LuaValue, LuaValue]:
        self.expect("{")
        result: dict[LuaValue, LuaValue] = {}
        index = 1
        while True:
            self.skip()
            ch = self.peek()
            if ch == "}":
                self.pos += 1
                return result
            if not ch:
                raise self.error("unterminated table: expected '}', found end of file")
            if ch == "[" and not _LONG_OPEN.match(self.text, self.pos):
                self.pos += 1
                key = self.value()
                if key is None or isinstance(key, dict):
                    raise self.error("a table key must be a string, number or boolean")
                self.expect("]")
                self.expect("=")
                result[key] = self.value()
            else:
                named = _NAME.match(self.text, self.pos)
                after = self.text[named.end() :].lstrip() if named else ""
                if named and after.startswith("=") and not after.startswith("=="):
                    self.pos = named.end()
                    self.expect("=")
                    result[named.group()] = self.value()
                else:
                    result[index] = self.value()
                    index += 1
            self.skip()
            sep = self.peek()
            if sep in (",", ";"):
                self.pos += 1
            elif sep != "}":
                found = repr(sep) if sep else "end of file"
                raise self.error(f"expected ',' or '}}' in table, found {found}")

    def assignments(self) -> dict[str, LuaValue]:
        result: dict[str, LuaValue] = {}
        while True:
            self.skip()
            if self.pos >= len(self.text):
                return result
            m = _NAME.match(self.text, self.pos)
            if not m:
                raise self.error(f"expected a variable name, found {self.peek()!r}")
            self.pos = m.end()
            self.expect("=")
            result[m.group()] = self.value()


def parse_lua(text: str) -> dict[str, LuaValue]:
    """Parse a SavedVariables file into `{variable name: value}`."""
    return _Parser(text.lstrip("﻿")).assignments()


def as_list(table: LuaValue) -> list[LuaValue]:
    """The positional entries (keys 1..n) of a parsed table, in order."""
    if not isinstance(table, dict):
        return []
    out = []
    i = 1
    while i in table:
        out.append(table[i])
        i += 1
    return out
