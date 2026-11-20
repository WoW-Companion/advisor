"""Typed character model. Fields ForeverScribe did not capture are `None`, never invented."""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(frozen=True)
class EquippedItem:
    slot: str
    item_id: int | None
    enchant: int | None
    link: str | None
    name: str | None
    item_level: int | None
    quality: int | None
    durability: int | None
    durability_max: int | None


@dataclass(frozen=True)
class ItemStack:
    item_id: int
    count: int
    name: str | None
    link: str | None


@dataclass(frozen=True)
class Objective:
    text: str | None
    have: int | None
    need: int | None
    done: bool | None
    type: str | None


@dataclass(frozen=True)
class Quest:
    quest_id: int
    title: str | None
    level: int | None
    zone: str | None
    complete: bool | None
    failed: bool | None
    objectives: list[Objective]


@dataclass(frozen=True)
class Talent:
    name: str | None
    rank: int
    max_rank: int | None
    node_id: int | None
    spell_id: int | None


@dataclass(frozen=True)
class TalentTree:
    name: str | None
    points: int | None
    talents: list[Talent]


@dataclass(frozen=True)
class Talents:
    system: str | None
    loadout: str | None
    unspent: int | None
    trees: list[TalentTree]


@dataclass(frozen=True)
class Skill:
    name: str
    skill_id: int | None
    rank: int | None
    max_rank: int | None
    group: str | None


@dataclass(frozen=True)
class Reputation:
    faction_id: int
    name: str
    standing: int | None
    value: int | None
    min: int | None
    max: int | None
    at_war: bool | None
    group: str | None


@dataclass(frozen=True)
class Character:
    key: str
    name: str | None
    realm: str | None
    level: int | None
    class_name: str | None
    race: str | None
    faction: str | None
    money: int | None
    xp: int | None
    xp_max: int | None
    rested: int | None
    captured_at: int | None
    addon_version: str | None
    equipment: dict[str, EquippedItem] | None
    bags: list[ItemStack] | None
    bank: list[ItemStack] | None
    quests: list[Quest] | None
    completed_quest_ids: list[int] | None
    talents: Talents | None
    skills: list[Skill] | None
    professions: list[Skill] | None
    reputations: list[Reputation] | None


@dataclass(frozen=True)
class CharacterFile:
    """Everything one SavedVariables file holds."""

    addon_version: str | None
    schema: int | None
    characters: list[Character] = field(default_factory=list)

    @property
    def captured_at(self) -> int | None:
        """The newest capture time over all characters."""
        times = [c.captured_at for c in self.characters if c.captured_at is not None]
        return max(times) if times else None
