"""Plain-text briefs of captured characters. Every line comes from the capture; gaps are stated."""

from __future__ import annotations

from datetime import UTC, datetime

from wowc_advisor.character import Character, CharacterFile
from wowc_advisor.character.model import EquippedItem, Talents

STALE_AFTER_SECONDS = 3600

# The standard inventory slots, by the lowercase names ForeverScribe uses for them
# (docs/foreverscribe-format.md; only `head`, `chest` and `mainhand` are seen in fixtures).
SLOTS = [
    "head",
    "neck",
    "shoulder",
    "back",
    "chest",
    "shirt",
    "tabard",
    "wrist",
    "hands",
    "waist",
    "legs",
    "feet",
    "finger1",
    "finger2",
    "trinket1",
    "trinket2",
    "mainhand",
    "offhand",
    "ranged",
]


class CharacterLookupError(ValueError):
    """The requested character cannot be chosen; the message says what to do."""


def _utc(ts: int) -> str:
    return datetime.fromtimestamp(ts, UTC).strftime("%Y-%m-%d %H:%M UTC")


def format_money(copper: int | None) -> str:
    if copper is None:
        return "not captured"
    gold, rest = divmod(copper, 10000)
    silver, cop = divmod(rest, 100)
    return f"{gold}g {silver}s {cop}c"


def _describe(c: Character) -> str:
    level = f"level {c.level}" if c.level is not None else "level unknown"
    return f"{c.name or c.key} ({level} {c.class_name or 'class unknown'}, {c.realm or 'realm unknown'})"


def list_characters(file: CharacterFile) -> list[dict[str, str | int | None]]:
    return [
        {
            "name": c.name or c.key,
            "key": c.key,
            "level": c.level,
            "class": c.class_name,
            "realm": c.realm,
            "captured_at": _utc(c.captured_at) if c.captured_at is not None else None,
        }
        for c in file.characters
    ]


def choose_character(file: CharacterFile, name: str | None) -> Character:
    chars = file.characters
    if not chars:
        raise CharacterLookupError("The SavedVariables file holds no characters.")
    if not name:
        if len(chars) == 1:
            return chars[0]
        known = ", ".join(c.name or c.key for c in chars)
        raise CharacterLookupError(f"Several characters were captured ({known}); name one.")
    wanted = name.strip().casefold()
    found = [c for c in chars if wanted in {(c.name or "").casefold(), c.key.casefold()}]
    if len(found) == 1:
        return found[0]
    known = ", ".join(c.name or c.key for c in chars)
    if found:
        raise CharacterLookupError(f"'{name}' matches several characters; use one of: {known}.")
    raise CharacterLookupError(f"No captured character '{name}'; captured: {known}.")


def _spec(talents: Talents | None) -> str:
    if talents is None or not talents.trees:
        return "unknown (no talents captured)"
    scored = [t for t in talents.trees if t.points]
    if not scored:
        return "unknown (no talent points captured)"
    top = max(t.points or 0 for t in scored)
    best = [t for t in scored if t.points == top]
    if len(best) > 1:
        return f"unknown (trees tied at {top} points)"
    return f"{best[0].name or 'unnamed tree'} ({top} points)"


def _gear_line(slot: str, item: EquippedItem) -> str:
    parts = [item.name or (f"item {item.item_id}" if item.item_id else "name not captured")]
    if item.item_level is not None:
        parts.append(f"ilvl {item.item_level}")
    return f"- {slot}: {', '.join(parts)}"


def _gear(c: Character) -> list[str]:
    if c.equipment is None:
        return ["- not captured"]
    lines = [_gear_line(s, c.equipment[s]) for s in SLOTS if s in c.equipment]
    lines += [_gear_line(s, i) for s, i in c.equipment.items() if s not in SLOTS]
    empty = [s for s in SLOTS if s not in c.equipment]
    if empty:
        lines.append(f"- empty slots: {', '.join(empty)}")
    return lines


def _quests(c: Character) -> list[str]:
    if c.quests is None:
        return ["- not captured"]
    if not c.quests:
        return ["- none in the quest log"]
    out = []
    for q in c.quests:
        state = "complete" if q.complete else "failed" if q.failed else "in progress"
        level = f"level {q.level}" if q.level is not None else "level unknown"
        zone = f", {q.zone}" if q.zone else ""
        out.append(f"- {q.title or f'quest {q.quest_id}'} ({level}{zone}) [{state}]")
    return out


def _professions(c: Character) -> list[str]:
    if c.professions is None:
        return ["- not captured"]
    if not c.professions:
        return ["- none captured"]
    return [
        f"- {p.name}: {p.rank if p.rank is not None else '?'}/{p.max_rank if p.max_rank is not None else '?'}"
        for p in c.professions
    ]


def _progress(c: Character) -> str:
    if c.xp is None or c.xp_max is None:
        xp = "XP not captured"
    else:
        xp = f"XP {c.xp}/{c.xp_max}"
    rested = f"rested {c.rested}" if c.rested is not None else "rested not captured"
    return f"{xp}, {rested}"


def character_brief(c: Character, now: datetime | None = None) -> str:
    now = now or datetime.now(UTC)
    lines = [
        f"# {c.name or c.key}",
        f"Level: {c.level if c.level is not None else 'not captured'}",
        f"Class: {c.class_name or 'not captured'}",
        f"Race: {c.race or 'not captured'}",
        f"Realm: {c.realm or 'not captured'}",
        f"Progress: {_progress(c)}",
        f"Money: {format_money(c.money)}",
        f"Spec: {_spec(c.talents)}",
        "Equipped gear:",
        *_gear(c),
        "Professions:",
        *_professions(c),
        "Current quests:",
        *_quests(c),
        "",
    ]
    if c.captured_at is None:
        lines.append(
            "Source: ForeverScribe capture, time not captured. /reload in game to refresh."
        )
    else:
        lines.append(f"Source: ForeverScribe capture at {_utc(c.captured_at)}")
        age = now.timestamp() - c.captured_at
        if age > STALE_AFTER_SECONDS:
            lines.append(
                f"WARNING: this capture is {int(age // 3600)}h old; /reload in game to refresh."
            )
    return "\n".join(lines)
