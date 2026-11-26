# dungeon_plan

`dungeon_plan(character, limit=3)` lists the dungeons that fit a character, the boss drops there
that are upgrades for their spec, and the open quests that lead there.

- **Dungeons** come from the dataset's `instances` table (`instance_type` dungeon; raids are not
  planned). A dungeon fits from its minimum level up to 3 levels past its maximum. Its entrance
  zone is `entrance_zone_name`; `level_source` `curated` ranges are worded "about levels A-B".
- **Too low**: when the character is under every dungeon's minimum, the answer is "Too low for
  dungeons yet", the first dungeon (lowest minimum) with its range and entrance, the levels to go
  and one thing to do meanwhile (an open quest of the capture).
- **Faction**: a dungeon's `faction` column, else its entrance capital (Orgrimmar, Stormwind...),
  hides it from the other side.
- **Dungeon quests**: `instance_quests` rows for the map, minus quests already in the log, the
  other side's, and any more than 3 levels above the character; at most 3, with their givers.
- **Not dungeons**: an `instances` row typed dungeon with no `entrance_zone_name`, no `max_players` and no Questie entrance (the client's Ruins of Lordaeron) is not a dungeon players run, and is never listed.
- **Plan ahead**: `dungeon_plan(dungeon="Ragefire Chasm")` describes that one dungeon whatever the character's level or side: level range, entrance, every boss (the dataset's, plus the creatures Questie lists), up to 8 quests with givers (not held to the level window) and the drops for the class and spec. Under its minimum it opens with one line, "You're 10; it opens around 13. Here's the plan for when you're ready." Level filtering stays for "which dungeons should I do now?".
- **Raids by name**: `dungeon_plan(dungeon="Molten Core")` describes a raid the same way (group size, level range, entrance, every boss), though a raid is never planned or ranked. The dataset holds no lockout, reset period or attunement for a raid, so the text says "Lockout and attunement: not known yet" and states none.
- **Boss list**: the encounters of difficulty 0 and 1 only. The dataset keeps another mode of the same map (ids 198, 201, 215: bosses fought as one, a different boss set) under a later difficulty id; those are left out when the map has ordinary rows, and kept when it has nothing else.
- **Level unknown**: a dungeon with no level range is named apart, unranked.
- **Wording**: the text never narrates data gaps or sources; one short clause ("Drops not known
  yet") only where a gap changes the advice.
- **Upgrades** are the items `gear.Upgrades` scores as better than what the character wears, by the
  rules in `gear.md`. A boss's drops come from `instance_bosses` (creature id) joined to
  `loot_sources`; each drop lists the boss, the slot, the gain and the drop chance (whole percent from 1%, one decimal below it, "under 0.1%" below
  that, never "0%" for a drop that happens), or says the chance is unknown.
- **Unlinked bosses** (an encounter the dataset cannot tie to one creature) are named as "Drops not known yet"; their drops
  are unknown, not guessed.
- **Quests**: unfinished quests of the capture are listed under a dungeon when the dataset's
  `quests` row has the dungeon's map as its point-of-interest map or the dungeon's name as its zone
  name, or the capture's own zone is the dungeon's name. A quest the dataset cannot place is left
  out.
- **Rank**: most distinct upgrade items, then fewest levels outside the dungeon's range (0 inside
  it), then map id.
- The text also gives the group size (`max_players`) where the dataset has it.

Operator check: run `dungeon_plan` against a real ForeverScribe capture and a built dataset.
