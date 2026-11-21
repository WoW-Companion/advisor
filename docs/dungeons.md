# dungeon_plan

`dungeon_plan(character, limit=3)` lists the dungeons that fit a character, the boss drops there
that are upgrades for their spec, and the open quests that lead there.

- **Dungeons** come from the dataset's `instances` table (`instance_type` dungeon; raids are not
  planned). A dungeon fits when its level range comes within 3 levels of the character's level.
- **Level unknown**: a dungeon with no level range is listed apart, unranked, with its drops. It is
  never guessed into the ranking.
- **Upgrades** are the items `gear.Upgrades` scores as better than what the character wears, by the
  rules in `gear.md`. A boss's drops come from `instance_bosses` (creature id) joined to
  `loot_sources`; each drop lists the boss, the slot, the gain and the drop chance, or says the
  chance is unknown.
- **Unlinked bosses** (an encounter the dataset cannot tie to one creature) are named; their drops
  are unknown, not guessed.
- **Quests**: unfinished quests of the capture are listed under a dungeon when the dataset's
  `quests` row has the dungeon's map as its point-of-interest map or the dungeon's name as its zone
  name, or the capture's own zone is the dungeon's name. A quest the dataset cannot place is left
  out.
- **Rank**: most distinct upgrade items, then fewest levels outside the dungeon's range (0 inside
  it), then map id.
- The text also gives the group size (`max_players`) where the dataset has it.

Operator check: run `dungeon_plan` against a real ForeverScribe capture and a built dataset.
