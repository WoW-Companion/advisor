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
