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
