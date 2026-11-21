# Questie interop (optional)

The advisor can read the player's own installed Questie data to answer "what should I pick up" and
"where do I go". It uses your installed Questie, if present. It never ships, copies or caches any of it
where a release or dataset could pick it up.

## What it does

- **Setup row.** "Questie (optional)" reports `Questie vX with QuestieDB vY` found, not installed, or
  `QuestieDB unreadable (vY)`. Neither state is an error.
- **Pick-ups.** For the captured character it lists quests not in the log and not completed, whose
  level is within two of the character's, whose race and class masks allow them, whose prerequisites
  (`preQuestSingle`, `preQuestGroup`, `exclusiveTo`, and the chain fields) are satisfied by
  ForeverScribe's `questsCompleted`, and whose giver stands on the character's last known map. Quests
  that need a skill, reputation, spell or special flag, and placeholder names (DNT, deprecated), are
  never suggested. Ranked by level fit, then distance from the last known position. At most three are
  added, like the dataset's own pick-ups.
- **Locations.** The first five quests in the log get the nearest objective (creature, object or
  trigger spawn) or, when complete, the turn-in giver, as `Name, Subzone (x, y)` in map percent.
- **Fallback.** No Questie, an unreadable build, or no known position: the advisor uses the dataset's
  pick-ups and adds no locations. Answering never waits on or fails because of Questie.

## What it reads

`Interface/AddOns/QuestieDB/QuestieDB_Forever.toc` in the **baked** build (`X-Mode: baked`,
`X-Contract-Version: 4`): the `## X-<Type>-<id>-<field>` lines, each base64 CBOR. Fields used:

- Quest: `S` (name 1, requiredLevel 4, questLevel 5, requiredRaces 6, requiredClasses 7, zoneOrSort 17,
  nextQuestInChain 22, specialFlags 24, parentQuest 25, requiredMaxLevel 32, availableUntilCompleted
  33, disabledByQuest 36, and the skill, reputation, spell, specialisation and rank fields as
  "cannot check"), plus `2` startedBy, `3` finishedBy, `9` triggerEnd, `10` objectives, `12`
  preQuestGroup, `13` preQuestSingle, `16` exclusiveTo.
- Npc: `S` (name 1, subname 14) and `7` spawns. Object: `S` (name 1) and `4` spawns.
- `support/Forever/Zones/areaIdToUiMapId.lua`: read as text for the area id to uiMapID table and area
  names. No Lua is run.

Any other contract version or mode (such as a source checkout) is shown as unreadable. Nothing is
executed from the addon.

## Licence boundary

Questie and QuestieDB are GPL-3.0. WoW Companion reads them at runtime on the player's machine only.

- No Questie code or data is copied, vendored or derived into any WoW Companion repo, release,
  dataset, fixture or test. Test fixtures are hand-written in the same file shape with invented ids
  and names, and `internal/questie` has a test that fails if any file under `testdata/` holds more than
  40 characters copied from a Questie checkout (set `WOWC_QUESTIE_SOURCE` to its folder; it is skipped
  otherwise).
- The parsed index is cached only in the player's local cache folder (`os.UserCacheDir()/wowc-advisor/questie`),
  keyed by the install's TOC path, size, modified time and version, and rebuilt from their install. It
  is not written inside the repo, the release, or the dataset build.
- Docs credit Questie as an optional integration. The app shows no Questie branding beyond its name in
  the checklist.
