# Live guidance: what we have, what we can get, and a design

Status: audit and design only. Nothing here is built. Everything below is from the sources named
beside it; where a fact could not be checked, it says so.

## 1. What the saved data holds today

Source: ForeverScribe at the commit pinned in `foreverscribe-format.md` (`903be32`), files
`Core.lua` and `Collect.lua`, plus the synthetic fixtures in `testdata/foreverscribe`.

The game writes SavedVariables only on `/reload`, logout or exit, so every value is "as of the last
save". ForeverScribe's snapshot is refreshed on zone changes and quest events, but the file on disk
only moves at those three moments.

| Datum | In the file? | Where (ForeverScribe) | Read by the advisor? |
|---|---|---|---|
| Zone name | yes | `snapshot.location.zone` (`GetRealZoneText`, `Core.lua` `Location()`) | yes, since this change (`Character.Location`) |
| Subzone | yes | `snapshot.location.subzone`, `.minimap` | yes (`Location.Subzone`); not used for ranking |
| Map id and name | yes | `snapshot.location.mapID`, `.mapName` (`C_Map.GetBestMapForUnit`) | yes |
| Map coordinates | yes, when the client returns them | `snapshot.location.x`, `.y`, percent of the map to 2 decimals (`MapPosition`); absent in instances and on maps without a position | yes (`Location.X`, `.Y`); not used yet |
| Instance flag | yes | `snapshot.location.instance`, `.inInstance` | no |
| Hearthstone bind | name only | `snapshot.hearth` (`GetBindLocation`), also the `HEARTH_BOUND` event | yes (`Character.Hearth`); name only, no coordinates or map id |
| Objective progress | yes | `snapshot.quests[i].obj[j] {text, have, need, done, type}` (`numFulfilled`, `numRequired`) | yes (`Quest.Objectives`), unused by advice |
| Quest zone | the log's header text | `snapshot.quests[i].zone` is the quest-log header, not a map id | yes |
| Taxi nodes | node names and ids per continent, current flag | `char.taxi` (`FS.ScanTaxi`, `C_TaxiMap.GetAllTaxiNodes`) | no; no coordinates |
| Quest objective positions | **no** | the addon stores none | n/a |

Fixtures: none of the four synthetic captures carries a `location` block (they were written from the
quest-log and progress paths only), so the loader test builds one in code. A real capture should be
checked by the Operator for `location.x` and `.y`.

Before this change the advisor ignored `location` and `hearth` entirely, and "nearby" had to come from
the zone headers of quests already in the log.

## 2. What the dataset can give (client data via wago.tools)

Facts only, from the dataset's own documents (`data` repo, `docs/schema.md`, `docs/sources.md`) and a
read of the published build 1.60.1.71037 on 2026-11-21. Client data is credited to "Blizzard game
client data, retrieved via wago.tools".

| Want | Client table | State in this build |
|---|---|---|
| Quest list and line membership | `QuestV2`, `QuestLineXQuest` | 6,626 quests, but only **22** are in a line (3 lines); ids only, **no names** |
| Quest level, minimum level | none | **not in any allowed source**; `level` and `min_level` are null on every row |
| Quest objective areas | `QuestPOIBlob` (57 rows), `QuestPOIPoint` (121 rows) | blob to map and UiMap is in `quests` (`map_id`, `ui_map_id`, `zone_id`, `zone_name`, 22 quests, 0.33%). `QuestPOIPoint` coordinates exist in the client but are **not fetched** yet |
| Zone of quest giver and turn-in | none | **not available**: no quest-to-NPC table in the client data; the `npcs` table has no quest relation and no spawn coordinates |
| Class and race restrictions, prerequisites | none | not available. Only line order gives a "previous step" |
| Area triggers | `AreaTrigger` | not fetched; presumably present in the client, unverified |
| Flight paths | `TaxiNodes`, `TaxiPath` | not fetched; unverified |
| Zone names | `UiMap`, `AreaTable` (1,404 rows) | names only; nothing ties a quest to an area |

Plainly missing from the client data: NPC spawn coordinates, quest giver and turn-in NPCs, quest
levels and names, quest prerequisites and class or race masks, and anything about Zephras Isle (the
published build has no quest, NPC or zone row named for it).

Not allowed as a source, ever: Wowhead, forever-db, and Questie's database. They are GPL or otherwise
not redistributable. Everloot (CC BY-SA 4.0, API route in `docs/sources.md`) is the one sanctioned
non-client source; it holds NPC-to-zone but no quest rows and no coordinates.

Consequence for Part A: the suggestion code is complete and tested, but on this build it finds
nothing, because there is no quest level to place a quest in a band and no name to say. See
`docs/advice.md` ("Quests to pick up") for the columns it reads.

## 3. Design: live guidance in the game, planned by the overlay

### Split of work

- **Overlay and AI, at each save**: the strategic plan. Which quests, in what order, and why. Inputs:
  the saved log, completed list, `location`, `hearth`, the dataset. Output: a short ordered plan.
- **Addon, live**: carries the plan. It already sees the live position and quest progress through
  Blizzard's read APIs, so it needs no reload.

### The plan file

The advisor writes a data-only Lua file into the addon folder, next to the digest the existing
`WoWCompanion_Digest` addon already reads (`docs/digest.md`). Sketch of the content, a plain table
literal with no code:

```lua
WoWCompanionPlan = {
  schema = "1.0", generated = 1800000000, character = "Name",
  steps = {
    { quest = 1234, text = "Gather Kelp", mapID = 77, area = { x = 41.5, y = 20.2, r = 6 }, kind = "pickup" },
    { quest = 1190, text = "Logged Task", mapID = 77, area = { x = 52.0, y = 33.1, r = 8 }, kind = "objective", obj = 1 },
  },
}
```

`area` is a centre and radius in map percent, from `QuestPOIPoint` where the dataset has it. Where the
dataset has no area, the step carries only `mapID` or zone name, and the addon says "somewhere in
<zone>" and gives no direction. It never invents a point.

### What the addon does

- Reads the plan, then picks the first step whose quest is not complete, using `C_QuestLog` progress
  (`have/need`) it already reads live.
- Shows one line: the step text, progress (3/8), and the distance and compass direction from
  `C_Map.GetPlayerMapPosition` to the step's area, when both are on the same map.
- Re-evaluates on `QUEST_LOG_UPDATE` and a slow timer. No reload is needed, since nothing is written
  back.

Hard limits, as for the existing addons: read-only, no protected calls, no automation, no chat or mail.
No reading game memory, no hooks, no screen scraping, no side channel. The only channel out of the game
stays SavedVariables; the only channel in is the data-only file the advisor writes. The advisor never
learns the live position; it plans from the last save.

### Complement to Questie, not a copy

Questie draws pins for every quest from its own database. This design draws none. It answers a
different question, "what should I do next, and which way", and takes its order from the advisor, so
it adds sequencing, level fit and "pick this up on the way". It reads no Questie data and adds no map
pins. If Questie is loaded, the addon may point at Questie's tracked quest in text only; that is a
follow-up, not a dependency.

Does Questie cover WoW: Forever, including Zephras Isle? **Unverified.** Questie's database is built
for Classic-era clients, and nothing available in this audit shows it holding Forever's custom zones.
The published dataset has no row for Zephras Isle, so we cannot say what its quests or coordinates are
either. The Operator should check in game whether Questie shows pins on Zephras Isle; that single
observation decides how much the addon must supply itself.

### Open design questions

1. Where do quest levels, names and giver zones for Forever's custom quests come from, given the
   client tables carry none of them? Options: a Blizzard API read in game (a data-only dump by an
   addon, then merged by the advisor), or Everloot, if it grows quest rows. Without one, Part A stays
   silent on real data.
2. Plan staleness: the plan is valid as of the save; the addon must show its age and drop steps whose
   quest has since been completed.

## 4. Proposed follow-up missions (not drafted)

| Area | Idea | Size |
|---|---|---|
| data | Fetch `QuestPOIPoint`, `TaxiNodes`, `AreaTrigger`; add objective-area rows and a taxi table with credit | M |
| data | Find a sanctioned source of quest level, name, class/race masks and giver zone (client dump from an addon, or Everloot) | L |
| advisor | Use `location` x/y and the hearth in advice: distance to a step, "hearth first" | S |
| advisor | Write the plan file (`WoWCompanionPlan`) with a schema doc and tests | M |
| addon | Read the plan, show next step, progress and direction, strictly read-only | M |
| addon | Optional dump of the quest data the client does expose for Forever's custom zones | M |
