"""Turn a parsed `ForeverScribeDB` into typed character models.

Field names follow ForeverScribe's writer at the pinned commit; see docs/foreverscribe-format.md.
"""

from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import Any

from wowc_advisor.character.luatable import LuaParseError, as_list, parse_lua
from wowc_advisor.character.model import (
    Character,
    CharacterFile,
    EquippedItem,
    ItemStack,
    Objective,
    Quest,
    Reputation,
    Skill,
    Talent,
    Talents,
    TalentTree,
)

DB_VARIABLE = "ForeverScribeDB"


def _int(v: Any) -> int | None:
    if isinstance(v, bool) or not isinstance(v, (int, float)):
        return None
    return int(v)


def _str(v: Any) -> str | None:
    return v if isinstance(v, str) else None


def _bool(v: Any) -> bool | None:
    return v if isinstance(v, bool) else None


def _table(v: Any) -> dict | None:
    return v if isinstance(v, dict) else None


def _each[T](v: Any, build: Callable[[dict], T | None]) -> list[T] | None:
    """Build one value per table entry (ordered by key); None when the section is absent."""
    table = _table(v)
    if table is None:
        return None
    ordered = sorted(table.items(), key=lambda kv: (not isinstance(kv[0], int), str(kv[0])))
    out = [build(t) for _, t in ordered if isinstance(t, dict)]
    return [x for x in out if x is not None]


def _equipment(v: Any) -> dict[str, EquippedItem] | None:
    table = _table(v)
    if table is None:
        return None
    return {
        str(slot): EquippedItem(
            slot=str(slot),
            item_id=_int(e.get("itemID")),
            enchant=_int(e.get("enchant")),
            link=_str(e.get("link")),
            name=_str(e.get("name")),
            item_level=_int(e.get("ilvl")),
            quality=_int(e.get("quality")),
            durability=_int(e.get("dur")),
            durability_max=_int(e.get("durMax")),
        )
        for slot, e in table.items()
        if isinstance(e, dict)
    }


def _stack(e: dict) -> ItemStack | None:
    item_id = _int(e.get("itemID"))
    if item_id is None:
        return None
    return ItemStack(item_id, _int(e.get("count")) or 1, _str(e.get("name")), _str(e.get("link")))


def _stacks(items: Any) -> list[ItemStack] | None:
    return _each(items, _stack)


def _quest(e: dict) -> Quest | None:
    quest_id = _int(e.get("questID"))
    if quest_id is None:
        return None
    objectives = _each(
        e.get("obj"),
        lambda o: Objective(
            _str(o.get("text")),
            _int(o.get("have")),
            _int(o.get("need")),
            _bool(o.get("done")),
            _str(o.get("type")),
        ),
    )
    return Quest(
        quest_id=quest_id,
        title=_str(e.get("title")),
        level=_int(e.get("level")),
        zone=_str(e.get("zone")),
        complete=_bool(e.get("complete")),
        failed=_bool(e.get("failed")),
        objectives=objectives or [],
    )


def _talents(v: Any) -> Talents | None:
    table = _table(v)
    if table is None:
        return None
    trees = (
        _each(
            table.get("trees"),
            lambda t: TalentTree(
                _str(t.get("name")),
                _int(t.get("points")),
                _each(
                    t.get("talents"),
                    lambda n: Talent(
                        _str(n.get("name")),
                        _int(n.get("rank")) or 0,
                        _int(n.get("max")),
                        _int(n.get("nodeID")),
                        _int(n.get("spellID")),
                    ),
                )
                or [],
            ),
        )
        or []
    )
    return Talents(
        _str(table.get("system")), _str(table.get("loadout")), _int(table.get("unspent")), trees
    )


def _skill(e: dict) -> Skill | None:
    name = _str(e.get("name"))
    if name is None:
        return None
    return Skill(
        name, _int(e.get("skillID")), _int(e.get("rank")), _int(e.get("max")), _str(e.get("group"))
    )


def _is_profession(skill: Skill) -> bool:
    group = (skill.group or "").lower()
    return "profession" in group or group in {"primary skills", "secondary skills"}


def _reputation(e: dict) -> Reputation | None:
    faction_id, name = _int(e.get("id")), _str(e.get("name"))
    if faction_id is None or name is None:
        return None
    return Reputation(
        faction_id, name, _int(e.get("standing")), _int(e.get("value")), _int(e.get("min")),
        _int(e.get("max")), _bool(e.get("atWar")), _str(e.get("group")),
    )  # fmt: skip


def _completed(v: Any) -> list[int] | None:
    if not isinstance(v, dict):
        return None
    return [i for i in (_int(x) for x in as_list(v)) if i is not None]


def load_character(key: str, record: dict) -> Character:
    snap = _table(record.get("snapshot")) or {}
    ident = _table(snap.get("identity")) or {}
    bags = _table(snap.get("bags"))
    bank = _table(record.get("bank"))
    skills = _each(snap.get("skills"), _skill)
    rep = _table(record.get("reputation"))
    return Character(
        key=key,
        name=_str(ident.get("name")),
        realm=_str(ident.get("realm")),
        level=_int(snap.get("level")),
        class_name=_str(ident.get("class")),
        race=_str(ident.get("race")),
        faction=_str(ident.get("faction")),
        money=_int(snap.get("money")),
        xp=_int(snap.get("xp")),
        xp_max=_int(snap.get("xpMax")),
        rested=_int(snap.get("rested")),
        captured_at=_int(snap.get("capturedAt")),
        addon_version=_str(snap.get("addonVersion")),
        equipment=_equipment(snap.get("equipment")),
        bags=_stacks(bags.get("items")) if bags else None,
        bank=_stacks(bank.get("items")) if bank else None,
        quests=_each(snap.get("quests"), _quest),
        completed_quest_ids=_completed(snap.get("questsCompleted")),
        talents=_talents(snap.get("talents")),
        skills=skills,
        professions=[s for s in skills if _is_profession(s)] if skills is not None else None,
        reputations=_each(rep.get("factions"), _reputation) if rep else None,
    )


def load_database(variables: dict[str, Any]) -> CharacterFile:
    """Build models from parsed variables (the result of `parse_lua`)."""
    db = _table(variables.get(DB_VARIABLE))
    if db is None:
        raise LuaParseError(f"no {DB_VARIABLE} table found", 1)
    chars = _table(db.get("chars")) or {}
    return CharacterFile(
        addon_version=_str(db.get("addonVersion")),
        schema=_int(db.get("schema")),
        characters=[
            load_character(str(key), rec)
            for key, rec in sorted(chars.items(), key=lambda kv: str(kv[0]))
            if isinstance(rec, dict)
        ],
    )


def load_savedvariables(path: Path | str) -> CharacterFile:
    """Read and parse a ForeverScribe.lua file. Raises LuaParseError on malformed input."""
    return load_database(parse_lua(Path(path).read_text(encoding="utf-8")))
