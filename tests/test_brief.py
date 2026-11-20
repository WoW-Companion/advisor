from datetime import UTC, datetime
from pathlib import Path

import pytest

from wowc_advisor.brief import (
    CharacterLookupError,
    character_brief,
    choose_character,
    format_money,
    list_characters,
)
from wowc_advisor.character import load_savedvariables

FIXTURES = Path(__file__).parent / "fixtures" / "foreverscribe"
FRESH = datetime.fromtimestamp(1700100000 + 60, UTC)
STALE = datetime.fromtimestamp(1700100000 + 3 * 3600 + 5, UTC)


def brief(fixture, name=None, now=FRESH):
    file = load_savedvariables(FIXTURES / fixture)
    return character_brief(choose_character(file, name), now)


@pytest.mark.parametrize(
    ("copper", "text"),
    [(None, "not captured"), (0, "0g 0s 0c"), (1234567, "123g 45s 67c"), (99, "0g 0s 99c")],
)
def test_format_money(copper, text):
    assert format_money(copper) == text


@pytest.mark.parametrize(
    ("fixture", "name", "present", "absent"),
    [
        (
            "level30_caster.lua",
            None,
            [
                "Level: 30",
                "Class: MAGE",
                "Race: Gnome",
                "XP 12345/56000, rested 2800",
                "Money: 123g 45s 67c",
                "Spec: Test Class Tree (3 points)",
                "- head: Test Hood of Wisdom, ilvl 28",
                "- mainhand: Test Staff, ilvl 31",
                "empty slots: neck,",
                "- Tailoring: 150/225",
                "- Enchanting: 75/150",
                "- Test Quest: Fetch (level 29, Testzone) [in progress]",
                "- Test Quest: Report (level 30, Testzone) [complete]",
                "Source: ForeverScribe capture at 2023-11-16 02:00 UTC",
            ],
            ["Staves"],
        ),
        (
            "level1.lua",
            "testchar",
            [
                "Level: 1",
                "Class: WARRIOR",
                "Spec: unknown (no talents captured)",
                "- chest: Test Shirt",
                "Professions:\n- not captured",
                "- none in the quest log",
                "Money: 0g 0s 0c",
            ],
            [],
        ),
        (
            "two_characters.lua",
            "Testbeta",
            [
                "Level: 8",
                "XP not captured, rested not captured",
                "Equipped gear:\n- not captured",
                "Current quests:\n- not captured",
            ],
            [],
        ),
    ],
)
def test_brief_content(fixture, name, present, absent):
    text = brief(fixture, name, now=FRESH)
    for fragment in present:
        assert fragment in text
    for fragment in absent:
        assert fragment not in text


def test_stale_capture_warns():
    text = brief("level30_caster.lua", now=STALE)
    assert "WARNING: this capture is 3h old; /reload in game to refresh." in text
    assert "WARNING" not in brief("level30_caster.lua", now=FRESH)


def test_list_characters():
    rows = list_characters(load_savedvariables(FIXTURES / "two_characters.lua"))
    assert [(r["name"], r["level"], r["class"], r["realm"]) for r in rows] == [
        ("Testbeta", 8, "ROGUE", "Otherrealm"),
        ("Testalpha", 12, "PRIEST", "Testrealm"),
    ]
    assert rows[0]["captured_at"] == "2023-11-18 09:33 UTC"


@pytest.mark.parametrize(
    ("name", "fragment"),
    [(None, "name one"), ("Nobody", "No captured character 'Nobody'")],
)
def test_choose_character_errors(name, fragment):
    file = load_savedvariables(FIXTURES / "two_characters.lua")
    with pytest.raises(CharacterLookupError, match=fragment):
        choose_character(file, name)
