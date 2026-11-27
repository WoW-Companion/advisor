# Quest route plan

`wowc-advisor` plans an efficient order for the quests a character can do now: pick-ups, objectives and
turn-ins, each with a map position. The order comes from a deterministic optimiser, never from the
language model; the model only explains the plan. The plan needs the player's own Questie (see
`design/questie-interop.md`): coordinates are read at run time on the player's PC and written only to
the player's local files. Nothing Questie-derived is stored in this repo.

The plan follows the [product principles](product-principles.md): one numbered route, bundled errands.

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
- Open quests above the level reach (see "Schema 1.8"), stops on another map, and stops past the 20th go to `later`.
  The map with the most `later` stops is named as a final `travel` stop: "Fly to <place> (<flight master>), <zone>"
  when the player knows a flight master there, else "Walk or ride to <zone> from <side>"; across continents the
  hub comes first ("Zeppelin to Orgrimmar, then fly to ..."). A dungeon reads "Travel to <place> (<zone>)",
  never a map number. An unknown flight point is never suggested as a flight. See "Schema 1.5".
- Quests ready to turn in on the player's map (and a delivery within about 250 yards of the player) are the
  first steps, nearest first. A ready turn-in in a town
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
  schema = "1.6",
  generated = 1795363809,        -- unix seconds
  characters = {
    ["Hero"] = {                 -- the character name as captured, no realm
      start = { map = 7001, x = 48.0, y = 49.0, at = 1795363000 },   -- the save's position; see "Live re-routing data"
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
| `kind` | `pickup`, `objective`, `turnin`, `travel` or `errand` (see "Step kinds and sections") |
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
implies and the place where it is done. The quest log's own text decides the verb ("slain" is "Kill", "Talk to" stays,
"placed" and the like are kept). Without it, an NPC whose Questie faction includes the player's side, or a
quest giver with no faction, is "Talk to" (or the objective's own "Listen to"); any other is "Kill". A quest in the log that lists open objectives plans only those. Objective
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

## Schema 1.6: one shared plan

The panel and the map read one plan: **now, the run here, the bigger plan.** Every quest in the log is a
commitment and is in exactly one of three places.

- `steps` is the run here: the numbered stops on the player's map, the ones the map draws (1..N). A journey
  is never a step: there are no `travel` steps any more. `exit = { label, map, x, y, via }` is the run's
  last line, for example "Fly to Sun Rock Retreat (Tharm), Stonetalon Mountains". `via` is `flight`,
  `zeppelin`, `boat` or `walk`; `x`, `y` are -1 when the arrival is not known. It is absent when the next
  trip is in the player's own zone (or there is no trip).
- `trips` is the logged quests that wait outside the run, one entry per destination zone, in suggested
  travel order (`order` 1..n): a route over the travel graph (see "Schema 1.7: trip order"), not a ranking.

  ```lua
  trips = {
    { order = 1, map = 1434, zone = "Stranglethorn Vale", ready = 2, todo = 0, pickups = 0, questIDs = { 201, 202 },
      reason = "2 ready to turn in", travel = "Fly to Booty Bay (Test Master), Stranglethorn Vale" },
  },
  ```

  A quest belongs to the zone where its next action happens: a ready turn-in to its finisher's zone, open
  objectives to the zone of the first one. A dungeon quest is a trip to the instance's entrance zone with
  `instance` set, when the player has a group plan; otherwise it is parked. A trip whose `map` is the
  player's own holds quests the run's caps left out ("Here, once the run is done"). `pickups` counts
  quests not in the log that could be picked up there; they are not in `questIDs`.
- `parked` is `{ questID, quest, why, kind, level, diff }`, quests set aside, with `kind`: `toohigh` (open work more than
  the level reach above the player, 3 by default, never a done quest or a delivery: "level 40 · doable around 37"),
  `elite` ("Elite: needs a group"), `hidden` ("Hidden by you"), `grey` (grey to the player and nothing to hand in; with 30 or more of 40
  slots used, "grey at your level, worth abandoning"), `dungeon` (needs a group, and no group plan is set)
  or `seasonal` (a holiday quest). A parked quest is in no trip and in no step.

**Each quest appears once** across `steps` (its own stops, a quest with any stop here is a step quest and
is not in a trip), `trips` and `parked`. `later`, `find`, `zones` and every older key keep their meaning;
`later` still lists the stops off the run (it is not the parked list).

New pick-ups must earn their place in the run: the quest is for the player's side, within 3 levels below and
2 above the player, and its giver is on the way (within 6 map percent, about 300 yards, of the player or of
a stop of a logged quest) or in the zone's hub (within 12 map percent of the player). At most 3 are taken;
class quests and a town's dungeon quests are exempt. The rest wait in `later`.

The `here` block holds only what can be done at the spot now: ready turn-ins, pick-ups, the trainer and the
vendor. A turn-in that is not ready (the quest still needs something) is a stop in `steps` and is never a
`here` line.

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

## Stops with no position

A stop the planner cannot place is written with `x = -1, y = -1` and `unplaced = true`; no stop is ever
written at 0, 0. A stop placed at a subzone's centre, because the quest text names the subzone and the
dataset has no spawn for the objective, carries `approx = true` and real coordinates.

## Schema 1.7: every logged quest has a home, and a city has errands

Every quest in the log is in exactly one of `steps`, `trips`, `find` or `parked`. Difficulty only matters for
work not yet done: a quest whose objectives are all done is a free turn-in at any level, so it is a step (its
finisher is in the run's zone), else counted `ready` in the trip for the finisher's zone, else a `find` entry.
`parked` `toohigh` is for quests with open objectives only, and its `why` says when it becomes doable.

A `find` entry for a quest with no known finisher or place carries the quest's own words, read from the
player's Questie data at run time:

```lua
find = {
  { n = 1, questID = 79624, quest = "Anyone Can Cook", hint = "Find an ogre who can read the Illegible Recipe", done = false },
},
```

`zone` is added when the data narrows it down. Older find entries (an objective with no place) keep their keys.

When the player's zone or the run's zone is a capital or major hub, `errands` tags the steps that are errands,
in step order. The errand stops join `steps` as ordinary numbered stops (kind `errand`, and `errand` set on
the step); `errands` only says which steps they are:

```lua
errands = {
  { n = 4, kind = "train", label = "Train now at your class trainer", map = 1458, x = -1.0, y = -1.0, why = "2 new spells or ranks to train at this level" },
  { n = 5, kind = "bank", label = "Open your bank once", map = 1458, x = -1.0, y = -1.0, why = "open your bank once so the advisor can see it" },
},
```

`kind` is `train` (new ranks are trainable), `bank` (the bank is not captured), `class` (a class quest is
offered here) or `turnin` (a ready turn-in here). `x` and `y` are -1 when the position is not known.

### "While you're here" errands (additive)

An errand may ride on a step the route already makes. Such an errand has `attach = <step n>` and `npc` (who or
what to use), and `n = 0`: it is not a step of its own. The addon shows it as a sub-line of the attached step,
"While you're here: " plus `label` (for example "buy 4 Empty Vial from Sly Garrett"). `label` starts with a lower-case verb.

| `kind` | when | where |
|---|---|---|
| `restock` | reagents of a known recipe that still gives skill-ups are short of a batch of 5 | a vendor that sells them |
| `repair` | durability is under 50%, or an item is under 25% | an NPC with the repair flag |
| `selljunk` | bags are 80% full or more, or the route would fill them (see below), and something is safe to sell | a general vendor |
| `mail` | the advice sends items to another character (`label` names them and the character) | a mailbox |

The errand attaches to the nearest step within about 150 yards (5 map percent) of the NPC or mailbox. With no
such step it is dropped, unless it is urgent (an item under 25% durability, bags full): then it is its own
errand step (`n` is the step, `attach` absent) at the place nearest the route. Items a quest in the log needs
are never sold or mailed. Durability and bag size may not be captured; the errand is then skipped and `doctor`
says so. Consumers keep an errand kind they do not know.

**Bag forecast.** When the bag size and the bags are captured, the advisor walks the steps in order from the
bags in use. Quest items an objective collects, a rough allowance of loot for a quest with kills, a reward at
each turn-in, and gathering, crafting and buying lines add stacks; the items a turn-in hands in free them. If
the total passes the bag size at step k, a clearing errand rides on the last stop at or before k with the
service: `selljunk` at a general vendor, then `mail` at a mailbox, then `bank` at a banker, each only for
what is safe (never an item a quest in the log needs, never kept profession materials). `why` says "bags fill
around step 9 (Kill Shadowmaw Panther)" and how many slots it frees. With no such stop before k, the errand
rides on the first one after it and `why` says so. The 80% rule stays as the floor.

## Schema 1.7: trip order, transports and `eta`

Trips are ordered as a travel route, not a ranking. The advisor builds a small graph whose nodes are zones and
whose edges are the zone borders (on foot or by mount), the faction's zeppelins and boats (Horde: Undercity,
Orgrimmar and Grom'gol; the Ratchet and Booty Bay boat; Alliance: Menethil Harbor, Auberdine, Theramore and
Rut'theran, and the Deeprun Tram) and the flight paths the character has opened. Every edge has an estimated
time in minutes. The order minimises the total travel time from the run's exit, plus a value term (each ready
turn-in and each 2,000 XP weighs the time spent before reaching a trip), so a very valuable trip can still go
first. The search is nearest-neighbour from every start, improved by 2-opt.

- `eta` on a trip is the estimated minutes of travel from the previous trip (from the exit for the first one).
- A trip's `travel`, and the `exit` label, name the real legs: "Zeppelin from Undercity to Grom'gol, then ride
  to Stranglethorn Vale", "Fly Tarren Mill → Hammerfall". "Walk or ride to <zone>" is used only when no faster
  known link exists. A trip's travel starts where the trip before it ends.
- A dungeon's `parked` reason and a trip's `instance` name the instance and its entrance zone: "needs a group:
  Razorfen Kraul (The Barrens)". They never say "another area".
- A `train` or `bank` errand has the position of the class trainer or banker in the city (Questie's NPC spawns,
  read at run time). Only when none is known is it unplaced (`x = -1`), and its label then names the area when
  that is known ("Train now at your class trainer (Rogues' Quarter)").

## Schema 1.8: level reach, hidden quests, quest levels

The addon captures `planPrefs` per character (WoWCompanionDB.characters[key]); a missing table or key means the
defaults:

```lua
planPrefs = { levelReach = 3, hidden = { [12345] = true }, includeElite = false }
```

- `levelReach` is 0 to 10, or -1 for no limit. A logged quest with open objectives whose level is above the
  player's level + `levelReach` is no step and no trip: it is `parked` as `toohigh`, "level 38 · doable around 35".
  Pick-up candidates use the same limit. A ready turn-in and a delivery ("speak with", nothing to do) are never
  filtered by level.
- An elite quest (the capture's `elite = true` on a quest entry) is parked as `elite`, "Elite: needs a group", unless
  `includeElite`.
- A quest id in `hidden` is in no step, trip, `later` or `find`, and is not counted in a trip's totals. It is parked
  with `kind = "hidden"`, why "Hidden by you", so the player can bring it back.
- Every step and every `later`, `parked` and `find` entry carries `level` (the quest's level; absent when unknown)
  and `diff`, the game's colour for it against the player's level: `red` (5 or more above), `orange` (3-4), `yellow`
  (within 2), `green`, or `grey` (8 or more below).
- A ready turn-in or delivery within about 250 yards of the player (or the run's start) is the first step, so it is
  NOW. A delivery step reads "Turn in <quest> to <NPC> (<place>)".

## Live re-routing data: `after`, `flights`, `start`

These keys are additive (no schema bump): a reader that does not know them ignores them.

- `after = { n, ... }` is on every step in `steps` and `later` (`{ }` when nothing comes first): the numbers of
  the steps in the same `steps` list that must be done first. A quest's pick-up comes before its objectives
  and its turn-in, its objectives before its turn-in, and a locked follow-up waits for the turn-ins that
  unlock it. The addon re-orders the run from the player's live position, keeping each step after its
  `after` steps, instead of reading "after step 1" out of the label.
- `flights = { { map, x, y, name, node, known }, ... }` lists the flight masters on the run's map and on the
  maps of its other zones and trips, for the character's side: from Questie's taxi NPCs, and the character's
  captured flight paths (`known = true`; one the data does not have is added). `name` is the flight master,
  `node` the place the flight path is named after. At most 60 entries; the addon can suggest "Fly Booty Bay
  -> Grom'gol" for a far stop from `known` masters.
- `start = { map, x, y, at }` is always written: the position of the save the plan was built from and `at`,
  the capture time (unix seconds, 0 when unknown). With no position, `map = 0` and `x = y = -1`. The addon
  compares it with the player's live position to see how far they have moved since.
- A label names a place once: an unplaced objective reads "Locate the hunters' camp (somewhere in
  Stranglethorn Vale)", never with the zone repeated.

## Step kinds and sections (the contract)

Every step (in `steps`, `later` and `zones`) has `kind`, one of:

| `kind` | Meaning |
| ------ | ------- |
| `pickup` | pick a quest up from its giver |
| `objective` | do one open objective at its place |
| `turnin` | hand a quest in at its finisher |
| `travel` | a journey (older plans; schema 1.6 and later write none in `steps`) |
| `errand` | a city errand: train, bank, profession or class quest; the step's `errand` key names which |

A `Plan.lua` character holds these sections: `steps` (the run here), `exit` (how to leave it), `trips` (the
logged quests outside the run, in route order), `parked` (set aside, with a `why` and a `kind`), `later` (stops
off the run), `find` (quests with no known place) and `errands` (which steps are errands), beside `start`,
`flights`, `basis`, `guidance`, `here`, `finishers` and `zones`.

**Consumers must keep unknown kinds.** A step whose `kind` a reader does not know is shown or skipped, never a
reason to drop the plan: the advisor adds kinds without a schema bump. The same goes for unknown keys and
sections. (An addon that rejected `errand` dropped the whole plan.)

A golden example with synthetic names, covering every section, is `testdata/plan/Plan-1.7.lua`; the addon repo
copies it for its contract test.
