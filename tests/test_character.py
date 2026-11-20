from pathlib import Path

import pytest

from wowc_advisor.character import LuaParseError, load_savedvariables, parse_lua

FIXTURES = Path(__file__).parent / "fixtures" / "foreverscribe"


@pytest.mark.parametrize(
    ("literal", "expected"),
    [
        ("42", 42),
        ("-7", -7),
        ("3.5", 3.5),
        ("1e3", 1000.0),
        ("0x1F", 31),
        ("true", True),
        ("false", False),
        ("nil", None),
        ('"plain"', "plain"),
        ("'single'", "single"),
        (r'"a\"b\\c\nd\065"', 'a"b\\c\ndA'),
        ("[[long\nstring]]", "long\nstring"),
        ("{}", {}),
        ('{ "a", "b" }', {1: "a", 2: "b"}),
        ('{ ["x"] = 1, [2] = "two", y = true }', {"x": 1, 2: "two", "y": True}),
        ("{ 1, 2; 3 }", {1: 1, 2: 2, 3: 3}),
        ("{ { 1 }, -- [1]\n { 2 }, -- [2]\n}", {1: {1: 1}, 2: {1: 2}}),
        ("{ [1.5] = 1, [true] = 2 }", {1.5: 1, True: 2}),
    ],
)
def test_literal_forms(literal, expected):
    assert parse_lua(f"V = {literal}") == {"V": expected}


def test_comments_and_multiple_assignments():
    text = "-- header\nA = 1 --[[ block\ncomment ]]\nB = { 2 }\n"
    assert parse_lua(text) == {"A": 1, "B": {1: 2}}


@pytest.mark.parametrize(
    ("text", "line", "fragment"),
    [
        ("V = {", 1, "unterminated table"),
        ('A = 1\nV = "abc', 2, "unterminated string"),
        ('V = "a\nb"', 1, "unterminated string"),
        ('V = "\\q"', 1, "invalid escape"),
        ("V = 1\nW = {1 2}", 2, "expected ','"),
        ("V = os.exit()", 1, "unexpected identifier"),
        ("V = print", 1, "unexpected identifier"),
        ("V = {[nil] = 1}", 1, "table key"),
        ("V = {[1 = 1}", 1, "expected ']'"),
        ("V =", 1, "end of file"),
        ("= 1", 1, "variable name"),
        ("V = @", 1, "unexpected character"),
        ('V = "\\999"', 1, "out of range"),
    ],
)
def test_malformed_input_names_the_line(text, line, fragment):
    with pytest.raises(LuaParseError) as err:
        parse_lua(text)
    assert err.value.line == line
    assert f"line {line}:" in str(err.value)
    assert fragment in str(err.value)


def test_level_1_character_has_uncaptured_fields_none():
    (char,) = load_savedvariables(FIXTURES / "level1.lua").characters
    assert (char.name, char.realm, char.level, char.class_name) == (
        "Testchar",
        "Testrealm",
        1,
        "WARRIOR",
    )
    assert char.rested is None
    assert char.talents is None and char.reputations is None and char.skills is None
    assert char.bank is None and char.completed_quest_ids is None
    assert char.quests == []
    assert char.equipment["chest"].enchant is None
    assert [(b.item_id, b.count) for b in char.bags] == [(100002, 1)]


def test_level_30_caster_full_model():
    file = load_savedvariables(FIXTURES / "level30_caster.lua")
    assert file.captured_at == 1700100000 and file.addon_version == "0.10.1"
    (char,) = file.characters
    assert (char.level, char.class_name, char.race, char.faction) == (
        30,
        "MAGE",
        "Gnome",
        "Alliance",
    )
    assert (char.money, char.xp, char.xp_max, char.rested) == (1234567, 12345, 56000, 2800)
    head = char.equipment["head"]
    assert (head.item_id, head.enchant, head.durability, head.durability_max) == (
        100101,
        2999,
        40,
        45,
    )
    assert char.equipment["chest"].name == 'Test Robe of "Quotes"'
    assert char.equipment["chest"].durability is None
    assert {(b.item_id, b.count) for b in char.bags} == {(100201, 20), (100202, 5)}
    assert [(b.item_id, b.count) for b in char.bank] == [(100301, 200)]
    fetch, report = char.quests
    assert (fetch.quest_id, fetch.complete, fetch.objectives[0].have) == (90001, False, 3)
    assert (report.complete, report.objectives) == (True, [])
    assert char.completed_quest_ids == [90010, 90011, 90012]
    assert [(p.name, p.rank) for p in char.professions] == [("Tailoring", 150), ("Enchanting", 75)]
    assert len(char.skills) == 3
    assert (char.talents.loadout, char.talents.unspent) == ("Test Loadout", 2)
    assert [(t.name, t.rank) for t in char.talents.trees[0].talents][:2] == [
        ("Test Arcane Focus", 2),
        ("Test Fire Focus", 1),
    ]
    (rep,) = char.reputations
    assert (rep.name, rep.standing, rep.value, rep.at_war) == ("Test Stormwind", 5, 4200, False)


def test_two_character_file():
    file = load_savedvariables(FIXTURES / "two_characters.lua")
    assert [c.name for c in file.characters] == ["Testbeta", "Testalpha"]
    assert file.captured_at == 1700300000
    assert all(c.bags is None and c.equipment is None for c in file.characters)


def test_missing_database_table():
    from wowc_advisor.character import load_database

    with pytest.raises(LuaParseError, match="ForeverScribeDB"):
        load_database({})
