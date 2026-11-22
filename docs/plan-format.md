# Quest route plan

`wowc-advisor` plans an efficient order for the quests a character can do now: pick-ups, objectives and
turn-ins, each with a map position. The order comes from a deterministic optimiser, never from the
language model; the model only explains the plan. The plan needs the player's own Questie (see
`design/questie-interop.md`): coordinates are read at run time on the player's PC and written only to
the player's local files. Nothing Questie-derived is stored in this repo.

## Where it appears

- MCP tool `quest_plan` (`character` optional): the plan as JSON plus plain text.
- The AI's pre-fetched facts: the first 8 steps and a count of the rest.
- The overlay: a numbered "Plan" list under Next, with the stop count and the estimated distance.
- `Plan.lua`, written next to `Digest.lua` in the `WoWCompanion_Digest` folder every time the digest is
  written (`wowc-advisor digest`, `update_digest`, the overlay's auto-digest).

## Rules

- Stops: `pickup` (the giver), `objective` (one cluster centroid per mob or object that has spawns;
  the biggest cluster nearest the giver or the player, on the player's map when it has one) and
  `turnin` (the finisher). Only quests that are eligible now (the pick-up rules in `advice.md`) or
  already in the log are planned; failed quests and finished objectives are skipped.
- Objectives: kills, objects, item objectives (the nearest cluster of the NPCs that drop the item and the
  objects that hold it) and kill-credit objectives are placed from Questie. An unfinished objective with no
  location is not routed: it is listed under `find` as "<text> (location unknown)", and its quest's
  turn-in is withheld (a pick-up still goes on the route). Item vendors are not resolved.
- Order: a quest's pick-up comes before its objectives, and they come before its turn-in. Identical
  stops are merged (several turn-ins at one NPC, objectives within 3 map percent of each other).
- Quests more than 1 level above the character, stops on another map, and stops past the 20th go to `later`.
  The map with the most `later` stops is named as a final `travel` stop ("Travel to <zone>").
- Optimiser: starts at the last known position; minimises the straight-line length in map percent.
  Up to 12 stops it is exact (Held-Karp with the order constraints); above that it is nearest-neighbour,
  improved by order-respecting 2-opt and or-opt, and started again from the quest-by-quest order, so it
  is never longer than that.

## `Plan.lua`

Data only: one assignment of tables, strings, numbers and booleans. Never code.

```lua
WoWCompanionPlan = {
  schema = "1.0",
  generated = 1795363809,        -- unix seconds
  characters = {
    ["Hero"] = {                 -- the character name as captured, no realm
      start = { map = 7001, x = 48.0, y = 49.0 },   -- last known position; absent when unknown
      zone = "Testisle",         -- the map the route is on ("" when unknown)
      distance = 106.9,          -- route length in map percent
      steps = {                  -- in order; n counts from 1
        { n = 1, kind = "pickup", questID = 10, quest = "Quest 10", label = "Pick up Quest 10 from Elder Test", map = 7001, x = 50.0, y = 50.0, done = false },
        { n = 2, kind = "travel", questID = 0, quest = "", label = "Travel to Otherisle", map = 7002, x = 10.0, y = 10.0, done = false },
      },
      later = { },               -- same fields; not on the route
      find = { },                -- objectives with no known location: same fields without `map`, `x`, `y`
    },
  },
}
```

| Field | Meaning |
| ----- | ------- |
| `kind` | `pickup`, `objective`, `turnin` or `travel` |
| `questID`, `quest` | the first quest of the stop and the names of every quest merged into it; `0` and `""` for travel |
| `label` | a short line to show |
| `map` | the `uiMapID` |
| `x`, `y` | map percent, 0 to 100 |
| `done` | always `false` when written; the addon may track a visited stop |

Minor schema changes only add keys; readers ignore keys they do not know.
