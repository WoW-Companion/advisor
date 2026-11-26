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
  location is not routed for a quest not yet in the log: it is listed under `find` as "<text> (location unknown)",
  and its quest's turn-in is withheld. For a quest in the log every open objective is a step: area objectives
  ("reach this place") are placed from the player's Questie database and labelled "(from your Questie
  database)"; with no position anywhere the step reads "(somewhere in <zone>)" and carries x = 0, y = 0
  (no position). Item vendors are not resolved.
- Priority: a class quest's objectives come early, after whatever can be done at the player's current
  spot; the panel's Next quests and the answers' facts follow the same order.
- Order: a quest's pick-up comes before its objectives, and they come before its turn-in. Identical
  stops are merged (several turn-ins at one NPC, objectives within 3 map percent of each other).
- Town dungeon pick-ups: a quest offered on the character's map that leads to a dungeon whose level
  range fits (its minimum at most 3 levels above, its maximum at most 3 below) is a pick-up step even
  above the 1-level margin, first on the route, labelled "Pick up X (Dungeon, 2,100 XP) from NPC" (no XP
  when the quest XP table is missing). Holiday and festival quests wait in `later` unless nothing else is
  planned. With 2 or fewer free quest-log slots (40, or the client's size) only that many pick-ups are
  planned, dungeon quests first, then class quests, then by XP, and a note says the log is nearly full.
- Quests more than 1 level above the character, stops on another map, and stops past the 20th go to `later`.
  The map with the most `later` stops is named as a final `travel` stop: "Fly to <place> (<flight master>), <zone>"
  when the player knows a flight master there, else "Walk or ride to <zone> from <side>"; across continents the
  hub comes first ("Zeppelin to Orgrimmar, then fly to ..."). A dungeon reads "Travel to <place> (<zone>)",
  never a map number. An unknown flight point is never suggested as a flight. See "Schema 1.5".
- Quests ready to turn in on the player's map are the first steps, nearest first. A ready turn-in in a town
  the player has a flight to (any town, when no flight is known) gets a `travel` step "Fly to <town>: turn in
  <quests>", one per town, before the final travel stop.
- Follow-ups: see "Chain-aware plan" below.
- Optimiser: starts at the last known position; minimises the straight-line length in map percent.
  Up to 12 stops it is exact (Held-Karp with the order constraints); above that it is nearest-neighbour,
  improved by order-respecting 2-opt and or-opt, and started again from the quest-by-quest order, so it
  is never longer than that.

## `Plan.lua`

Data only: one assignment of tables, strings, numbers and booleans. Never code.

```lua
WoWCompanionPlan = {
  schema = "1.5",
  generated = 1795363809,        -- unix seconds
  characters = {
    ["Hero"] = {                 -- the character name as captured, no realm
      start = { map = 7001, x = 48.0, y = 49.0 },   -- last known position; absent when unknown
      basis = { captured = 1795363000, inLog = { 10, 12 } },  -- what the plan was built from (below)
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

`basis` is what the plan starts from: `captured` is the capture's time (0 when unknown) and `inLog` the
quest ids in the log when the plan was built, sorted. A quest in `inLog` has no pick-up step, and steps are
numbered from 1 in every plan. "Completed" means done after `basis.captured`: a reader judges steps against
the basis and never counts what was already true when the plan was built. When the capture has no quest log,
no pick-ups are planned and a note says so. An older addon ignores `basis`; a newer one treats a plan
without it as having no basis.

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

## Schema 1.2: guidance for a quest once accepted

The game writes the app's files only on a reload, so the addon never learns of an accept in time. The plan
therefore carries, for every quest it lists (route steps, `later`, `find`, follow-ups and class quests), what
the quest needs once taken. The addon switches to it live when the quest log shows the accept. Steps gain
`questIDs = { ... }` when a merged stop serves more than one quest; every non-travel step has `questID` and
`kind`, so the addon can mark a step done when the log shows it done and advance its step counter.

```lua
guidance = {                      -- per character, keyed by quest id; data only
  [31] = {
    questID = 31, quest = "Quest 31", class = true,
    steps = { 2, 5 },             -- the route steps that serve it (empty: not on the route)
    objectives = {                -- what to do once accepted; may be empty
      { text = "Kill Test Wolf", map = 7001, x = 30.0, y = 20.0, zone = "Testisle" },
      { text = "Collect Test Shield", zone = "Testisle" },   -- position unknown: the zone only
    },
    turnin = { text = "Turn in Quest 31 to Elder Test", map = 7001, x = 50.0, y = 50.0, zone = "Testisle" },
                                  -- absent when the turn-in is withheld (an unplaced objective) or unknown
    after = "After this: Pick up Quest 40 from Elder Test.",   -- the next planned step
  },
},
```

Names and positions come from the dataset or the player's own capture, never invented; an item without
`map`, `x` and `y` has only a `zone`. Older addons ignore the block.

## Schema 1.3: places and "here" data

Steps are a quest's open objectives, one per thing left to do, each with the verb its objective type
implies and the place where it is done. A friendly or quest-giving NPC is "Talk to" (or the objective's
own "Listen to"), never "Kill". A quest in the log that lists open objectives plans only those. Objective
stops merge only when they say the same thing (the same label) within 3 map percent.

Order: after the optimiser, a pick-up or turn-in within 3 map percent of the player's position, or of an
earlier stop, is moved right after it when every precedence holds and the route grows by no more than
twice that (the cost of the detour). What can be done at one place is done together.

Every step gains `place = "Windsong Lake"` (the subzone, else the zone; absent when unknown). A pin on the
world map sits at the step's `x`, `y`, which is the place the work is done (the objective's cluster, the
giver or the finisher), never the quest giver for an objective. Its marker is `kind` (`objective`,
`turnin`, `pickup`). Steps at one position are one stacked pin that lists each step's own `label`.
Guidance items gain `place` too; an item with no position has only `zone`.

A per-NPC `here` list, one block per NPC the route picks up from or turns in to:

```lua
here = {
  {
    npc = "Test Windshaper", npcID = 3, map = 7001, x = 50.0, y = 50.0, place = "Test Village", steps = { 4 },
    can = { "turn in", "buy and sell", "repair" },   -- also "pick up", "train"
    quests = {
      { questID = 30, quest = "The Test Construct", role = "turnin",   -- or "pickup"
        text = "Turn in The Test Construct once you have done: Collect Test Gyro (0/1) (Test Lake); Collect Test Core (0/1) (Test Camp).",
        missing = { { text = "Collect Test Gyro (0/1)", map = 7001, x = 10.0, y = 70.0, zone = "Testisle", place = "Test Lake" } } },
    },
    nearby = {                   -- quests a giver within 12 map percent offers that are not on the route
      [32] = { questID = 32, quest = "Hill Errand", giver = "Test Cultist", map = 7001, x = 55.0, y = 52.0,
               guidance = { ... } },   -- the 1.2 guidance block of that quest
    },
  },
},
```

`missing` is what the quest still needs, in the 1.2 item format; with nothing missing, `text` says so.
`can` comes from the NPC's service flags. `nearby` holds quests the planner listed but did not route (too
high a level, past the cap); a quest Questie says is not available to the character is never in it.

Minor schema changes only add keys; readers ignore keys they do not know.

## Schema 1.4: where every logged quest is handed in

The plan is rewritten only when the game saves its files (a reload or logout), so a quest that became
ready, or a place reached, since then is not in the steps. The addon can read the log's ready state live;
`finishers` gives it the places. Per character, a flat list, data only:

```lua
finishers = {
  { questID = 10, quest = "Quest 10", npc = "Elder Test", map = 7001, x = 50.0, y = 50.0, place = "Test Village" },
  { questID = 12, quest = "Quest 12", object = "Test Chest", map = 7001, x = 70.0, y = 70.0, place = "Test Village" },
},
```

- Every quest in the captured log gets rows, ready or not, on the route or not. A failed quest gets none.
- A row is one finisher spawn: `npc` (a creature) or `object`, never both. `place` is the subzone, else the zone.
- Spawns are limited to the player's continent when both the player's zone and the spawn's zone have a known
  continent. At most 3 per quest, nearest the quest's objective area first (the player's position when no
  objective is placed); at most 40 quests. A spawn with no known place is left out, and so is a quest
  whose finisher has none.
- Rows of one quest are adjacent, in the log's order. Step order is unchanged. An older addon ignores the key.

## Schema 1.5: one route per other zone

Every zone other than the player's that holds `later` stops gets one connected route that starts where the
player arrives. `later` lists the player's zone first, then each other zone's stops together, in route order.
Per character, keyed by the zone's map id, data only:

```lua
zones = {
  [1442] = {
    zone = "Stonetalon Mountains",
    travel = "Fly to Sun Rock Retreat (Tharm), Stonetalon Mountains",   -- the route step that goes there
    arrival = { kind = "flight", npc = 3881, name = "Tharm", place = "Sun Rock Retreat", map = 1442, x = 45.0, y = 59.0 },
    order = { 1, 2, 3, 4, 5, 6, 7, 8, 9, 10 },   -- the `n` of the zone's stops in `later`, each once
  },
},
```

- `arrival.kind` is `flight` (the nearest known flight master in the zone, from the flight points the capture
  recorded; `npc`, `name`, `x`, `y`, and `place` when the dataset places the NPC) or `walk` (no flight point
  known there: the entrance on the player's side, `name` is the side; no `map`, `x`, `y` when the entrance is
  not known). A flight is never suggested for an unknown flight point.
- `order` is one path from the arrival point (nearest-neighbour with 2-opt above 12 stops, exact below) with
  every quest's pick-up before its objectives before its turn-in. A stop with no position comes just before
  the first stop that needs it. A dungeon map has no entry.
- Stops past the 30-stop `later` cap are not in `order`. Readers ignore keys they do not know.

## Schema 1.5: trips

A quest with no counted objectives (no kill, item, kill-credit or area objective) whose finisher is not one of
its givers is a **trip**: a "go and find", a delivery, a "speak to". It is a journey to the finisher, never a
quick stop on the way. The turn-in step's label says where: "Find Deathstalker Vincent inside Shadowfang Keep
(Silverpine Forest) (dungeon) to turn in Deathstalkers in Shadowfang", or "Deliver to Sputtervalve in Ratchet
(The Barrens) to turn in Further Instructions". A trip into a dungeon is never a fly-and-turn-in leg. A quest
whose counted objectives are all done and that is handed in where it was given keeps "Turn in X to NPC".
Each key appears only on a trip (additive; an older addon ignores them):

| Field | On | Meaning |
| ----- | -- | ------- |
| `trip` | a `turnin` step, a `later` entry, a `finishers` row | `true` |
| `finisher` | the same | the NPC or object that takes the quest in |
| `finisherPlace` | the same | where it is: "Ratchet (The Barrens)", or the zone alone |
| `instance` | the same, when the finisher is inside a dungeon | the dungeon's name |

```lua
{ n = 7, kind = "turnin", questID = 1098, quest = "Deathstalkers in Shadowfang", label = "Find Deathstalker Vincent inside Shadowfang Keep (Silverpine Forest) (dungeon) to turn in Deathstalkers in Shadowfang", map = 33, x = 30.0, y = 40.0, done = false, trip = true, finisher = "Deathstalker Vincent", finisherPlace = "Shadowfang Keep (Silverpine Forest)", instance = "Shadowfang Keep" },
```

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
