# Questie interop (optional)

The advisor uses your installed Questie, if present, to answer "what should I pick up" and "where do
I go", reading its files on your own PC at runtime.

## What it does

- **Setup row.** "Questie (optional)" says `Questie vX with QuestieDB vY` found, not installed, or
  `QuestieDB unreadable (vY)`. None is an error.
- **Pick-ups.** Quests not in the log or completed, within two levels, allowed for the character's race
  and class, with prerequisites done (per ForeverScribe's completed quests) and a giver on their last
  known map. Quests needing a skill, reputation, spell or special flag, and DNT or deprecated names,
  are skipped. Ranked by level fit, then distance; at most three.
- **Locations.** The first five logged quests get the nearest objective, or the turn-in once complete,
  as `Name, Subzone (x, y)` in map percent.
- **Fallback.** No Questie, an unreadable build or no known position: the dataset's pick-ups are used
  and no locations added. Answering never fails because of Questie.

## What it reads

The baked `QuestieDB_Forever.toc` (`X-Mode: baked`, `X-Contract-Version: 4`): `## X-<Type>-<id>-<field>`
lines of base64 CBOR (long values in `~N~` chunks). Quest scalars `S` (fields 1, 4, 5, 6, 7, 17, 22, 24,
25, 32, 33, 36, plus the skill, reputation, spell and rank fields to skip such quests) and 2, 3, 10, 12,
13, 16; NPC 1, 7, 14; object 1, 4; and `support/Forever/Zones/areaIdToUiMapId.lua`, read as text.
Quest field 8 (the objective text, a list of lines) is read as well.
Any other contract or mode shows as unreadable. No Lua runs.

## Licence boundary

Questie and QuestieDB are GPL-3.0, read at runtime on the player's machine only.

- No Questie code or data is copied, vendored or derived into any repo, release, dataset, fixture or
  test. Fixtures are hand-written in the same shape with invented ids and names. A test fails if a file
  under `testdata/` holds over 40 characters found in a Questie checkout (`WOWC_QUESTIE_SOURCE`;
  skipped otherwise).
- The parsed index lives only in the player's local cache (`os.UserCacheDir()/wowc-advisor/questie`),
  keyed by the install's TOC path, size, time and version, and is rebuilt from their install.
- The app shows Questie's name only in the checklist.

## Quest lines and quest facts (`quest_chain`)

When the dataset has no quest line for a quest (WoW Forever's own quests, ids in the 90000s, are not
in it), `quest_chain` builds the chain from the index, marked `source: "questie"`:

- Before: the quest's pre-quest fields, quest field 13 (any one of) and 12 (all of).
- After: its field 22 (`nextQuestInChain`), plus every quest whose field 12 or 13 names it ("leads to").
- Each step: name (S1), level (S5), the giver (field 2) and turn-in (field 3) with the zone of their
  first known spawn, and the objective text (field 8). Class and race limits come from S7 and S6, the
  required level from S4.

The dataset wins whenever it has a line. A quest name the dataset lacks is matched against Questie's
names. With no follow-up in either source the answer says "No follow-up quest is listed for it in the
game data you have."

## Dungeon facts (`dungeon_plan`)

`dungeon_plan` adds what the player's own QuestieDB knows about a dungeon, marked "from your Questie
database". The dataset always wins where it has an answer; Questie fills the gaps:

- **Entrance.** The dataset's entrance zone first; else the parent zone's name. The coordinates are
  shown only when Questie's parent zone is the zone being shown.
- **Inside.** Creatures with a spawn in the dungeon's areas, bosses (rank 3) first, then elites
  (rank 1 and 2); the first 60 are kept.
- **Drops.** Items whose `npcDrops` (item field 2) name one of those creatures, scored with the same
  upgrade scoring as the dataset's drops; an item the dataset already lists for the dungeon is not
  repeated.
- **Quests.** Quests whose zone (quest `S` field 17) is a dungeon area, whose kill, credit or item
  objectives are creatures or drops inside; added after the dataset's quests, up to the same limit,
  skipping quests in the log, too high, restricted or without a giver.

Files and fields read, all as text or the same baked rows, never run:

- `support/Forever/Zones/dungeons.lua`: one row per dungeon, keyed by area id: name (string), the
  alternative area ids (a list of whole numbers), the parent zone's area id (a plain number) and the
  entrance (`{x, y}`, or `{zone, x, y}`). A row naming its own area id has two plain numbers: area,
  then parent zone. The first table literal in the file is read; a row with no name is skipped.
- `support/Forever/Zones/instanceIdToAreaId.lua`: `[instance] = area` lines, linking the dataset's map
  id to the dungeon.
- NPC `S` fields 5 (level) and 6 (rank) and 7 (spawns by area); item field 2 (`npcDrops`); quest `S`
  field 17 (zone) and objective field 10.

A missing or unrecognised file leaves the dungeon answer as the dataset gave it, with the reason kept
on the index (`DungeonReason`). The facts are cached with the rest of the parsed index, in the
player's local cache only.

## Gathering nodes and fishing schools (`where_to_gather`, `profession_plan`)

The index also keeps objects that yield items, with these fields (all read as the same baked rows,
never run):

- Object `S` field 1 (name) and field 4 (spawns by area, with coordinates).
- Item `S` field 1 (name) and field 10 (required level, 0 when absent), and item field 3
  (`objectDrops`: the object ids that contain it). An object's yields are the items whose
  `objectDrops` name it (at most 24 kept); an object with no yield or no spawn is dropped.

Classification (`internal/gather`), first match wins, containers (chest, crate, barrel, ...) skipped:

1. the dataset's `gathering_nodes` names the object: its profession (Herbalism, Mining, Fishing);
2. the name says vein or deposit: **ore**; school, wreckage, floating debris or oil spill: **fishing school**;
3. it yields an item that a Mining recipe uses as a reagent: **ore**;
4. it yields an item that an Alchemy or Inscription recipe uses and no Mining recipe does: **herb**.

Anything else is not gathered. Several object ids of one name (ore veins) are separate nodes that
yield the same item; their spawns add up by zone. Zones are named from `areaIdToUiMapId.lua`; a spawn
whose area has no name is skipped.

A database with no object rows, or none that yield an item, gives no nodes and a reason in the index
(`NodesReason`); the answer then says no place is known. Nothing is guessed. The parsed nodes sit in
the same local cache as the rest of the index (schema 6) and nothing from Questie is shipped.

## Profession trainers (`where_to_learn`)

`where_to_learn` uses the dataset's `trainers` rows (`npc_id`, `name`, `profession`, `zone_id`,
`zone_name`, `x`, `y`, `faction`, `recipes`, `reporters`; `trainer_spells`: `npc_id`, `spell_id`,
`required_skill`) for a profession and the character's faction. When the dataset has no trainer
for the profession, or none for the faction (Blacksmithing has none at all), the trainers come
from the Questie NPCs (`internal/questie/trainers.go`, index schema 7, which also keeps NPC field 15,
`npcFlags`). The rule, on the NPC's subname (lower-cased):

1. a subname with "supplies" is a vendor, never a trainer;
2. "<Profession> Trainer" and "Junior <Profession> Trainer" (Blacksmithing, Tailoring, Leatherworking,
   Alchemy, Enchanting, Engineering, Cooking, Fishing, First Aid, Mining, Herbalism, Skinning): tier 75;
3. a rank word before the person noun (Blacksmith, Tailor, Leatherworker, Alchemist, Enchanter,
   Engineer): Journeyman tier 150, Expert 225, Artisan 300, Master 375;
4. a bare person noun ("Blacksmith") is a trainer only when `npcFlags` has the trainer bit (0x10),
   at tier 75.

Faction is Questie's `friendlyToFaction` ("A", "H", "AH"). Zone and coordinates are the NPC's spawn
nearest the character (else its first). A trainer whose tier is below the skill wanted, the higher of
the recipe's gate and the character's skill, sorts after those that reach it; then the usual order:
same zone, same continent, nearest. The continent comes from a zone-name table in `internal/learn`
(the dataset has no continent column, and the area table here carries zone names only).
