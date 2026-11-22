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
- Follow-ups: see "Chain-aware plan" below.
- Optimiser: starts at the last known position; minimises the straight-line length in map percent.
  Up to 12 stops it is exact (Held-Karp with the order constraints); above that it is nearest-neighbour,
  improved by order-respecting 2-opt and or-opt, and started again from the quest-by-quest order, so it
  is never longer than that.

## `Plan.lua`

Data only: one assignment of tables, strings, numbers and booleans. Never code.

```lua
WoWCompanionPlan = {
  schema = "1.1",
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

Schema 1.1 adds these keys; each appears only when it applies:

| Field | On | Meaning |
| ----- | -- | ------- |
| `locked` | a follow-up's step | `true`: the quest cannot be picked up until the turn-ins in `unlockedBy` are done |
| `unlockedBy` | a locked step | `{ <questID>, ... }`: the turn-ins that unlock it, sorted |
| `depth` | a locked step | links from a planned quest: 1, or 2 |
| `chain` | a locked step | the quest line's name, when the dataset knows it |
| `unlocks` | a turn-in step | `{ <questID>, ... }`: the follow-ups this turn-in unlocks |

An older addon ignores them and shows the steps in order. Example:

```lua
{ n = 3, kind = "turnin", questID = 30, quest = "Quest 30", label = "Turn in Quest 30 to Elder Test", map = 7001, x = 50.0, y = 50.0, done = false, unlocks = { 31 } },
{ n = 4, kind = "pickup", questID = 31, quest = "Quest 31", label = "Pick up Quest 31 from Elder Test", map = 7001, x = 50.0, y = 50.0, done = false, locked = true, unlockedBy = { 30 }, depth = 1, chain = "The Test Line" },
```

Minor schema changes only add keys; readers ignore keys they do not know.

## Chain-aware plan

Turning a quest in often makes the next one available, usually from the same NPC. The plan includes
those follow-ups so it does not go stale between saves. All of it is worked out at run time on the
player's PC.

Where the links come from, in order of trust:

1. Questie's quest rows: a quest whose single or group prerequisite is a planned quest, and a planned
   quest's next-in-chain.
2. The dataset's `quest_lines` order (the quest after this one in its line), where Questie has no
   prerequisite for it. The line's name becomes `chain`.
3. Unlocks seen in game (below).

A follow-up is kept only when all of these hold:

- every prerequisite is done or is a planned turn-in (a group prerequisite needs all of them, a single one
  any); a quest that is already available without the planned turn-ins is an ordinary pick-up, not a follow-up;
- its level is in the available-quest band (the character's level -2 to +2) and not more than 1 above it,
  and its race, class and other eligibility pass;
- it is not exclusive with a planned or done quest, and not a breadcrumb for one;
- its giver is on the plan's map and its position is known: a follow-up with no known start is left out,
  never guessed.

The lookahead is 2 links. A follow-up's own turn-in can unlock the next one.

Order: a follow-up's pick-up comes after every planned turn-in that unlocks it, on top of the rules above
(pick-up before objectives, objectives before the turn-in). The exact optimiser and the 2-opt and or-opt
moves keep every precedence: a move that breaks one is not taken. A follow-up whose giver is the NPC of
the unlocking turn-in is moved right after it when that does not lengthen the route.

Cap: the plan holds at most 25 steps in all. Ordinary stops keep their cap of 20; follow-ups use the rest,
nearer and shallower ones first, a quest at a time. A follow-up that does not fit is left out and a note says
how many.

The digest's turn-in entries gain a `details` line `Unlocks: <Quest> (from <NPC>, here)` (`here` when the
NPC also takes the turn-in, `observed` when the link or giver was seen in game), and the turn-in `prompt`
names the first follow-up.

## In-game observations

The addon writes a bounded log into its own SavedVariables (`WoWCompanion.lua`, beside `ForeverScribe.lua`)
as `WoWCompanionDB.observed`. The advisor reads it on every refresh. This is the contract with the addon:

```lua
WoWCompanionDB = {
  observed = {
    version = 1,
    events = {                     -- oldest first; at most 400 kept (the advisor reads the newest 400)
      { t = 1795363809,            -- unix seconds
        ev = "turnedin",           -- "accepted", "turnedin" or "abandoned"; other values are ignored
        questID = 30,
        character = "Hero",        -- the name as captured, no realm
        map = 7001, x = 50.0, y = 50.0,   -- uiMapID and the player's position (map percent) at the event
        npc = "Elder Test",        -- the NPC being talked to, when there is one
        offPlan = true,            -- accepted events only: the quest was not in Plan.lua (optional)
      },
    },
  },
}
```

On each refresh the advisor:

- applies the events newer than the character's last capture: a quest accepted since is in the log and is
  routed from its objectives to its turn-in (even when it was never in the plan); one turned in is done; one
  abandoned is gone. Events of another character, and older ones, are ignored;
- learns unlocks: a turn-in of A followed within 120 seconds, on the same map within 12 map percent, by an
  accepted B is recorded as "A unlocks B at <npc>, <position>". The file is `observed-unlocks.json` in the
  user config folder (`wowc-advisor`, or the path in `WOWC_OBSERVED_UNLOCKS`), beside the saved play styles:
  `{ "version": 1, "unlocks": [ { "from": 30, "to": 31, "npc": "...", "map": 7001, "x": 50, "y": 50, "count": 1, "last": 1795363809 } ] }`.
  It fills the gaps where Questie and the dataset are silent (an unlock they do not list, or a giver Questie
  has no position for) and the plan then says "observed". It stays on the player's machine: no network, and
  it is never copied into the dataset or the repo.
