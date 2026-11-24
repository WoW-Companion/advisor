# In-game digest

`wowc-advisor digest [NAME]` (and the MCP tool `update_digest`) writes the data-only
`WoWCompanion_Digest/Digest.lua` the WoW Companion panel shows (format: schema 2.0, one schema 1.6 entry
per character; the addon repo's `docs/digest-format.md`). Type `/reload` in game afterwards.

**Every captured character.** The file holds one entry for each character in your SavedVariables, so an
alt gets advice without asking again:

```lua
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1795271211,        -- unix seconds, when the file was written
  characters = {
    ["Testchar"] = { schema = "1.9", ... },   -- exactly the 1.9 digest below, for that character
    ["Otherchar"] = { schema = "1.9", ... },
  },
}
```

The key is the character name as captured (what `UnitName("player")` returns), with no realm. At most
30 characters are written, the most recently captured first; the rest, and a repeated name, are listed
as skipped. `NAME` (or `update_digest`'s optional `character`) does not choose one: it is checked to be
captured. If it is not, nothing is written and the command exits non-zero with "<NAME> isn't in your
SavedVariables yet. In game, /reload on that character so ForeverScribe saves it, then run this again."
It prints each character's section counts and the `/reload` reminder.

The digest adds no advice logic: it is built from `whats_next` and `upgrade_finder`.

| Section | From | Cap |
| ------- | ---- | --- |
| `next_quests` | turn-ins and open quests from `whats_next`, with their zone and quest id | 8 |
| `upgrades` | one line per slot: the best upgrade that has a dependable way to be got (see [upgrade-paths.md](upgrade-paths.md)), with the item id | 8 |
| `keep_materials` | bag and bank reagents of the character's professions' recipes still worth making (`professions.KeepList`), most numerous first | 10 |
| `notes` | unspent talents, (naming the build's next node, see talents.md), failed quests, no dataset, bank not captured, dataset build differs from the game | 6 |

The entry's schema 1.2 adds two optional top-level lists beside `sections`. Readers that only know 1.0 or 1.1 ignore
unknown keys.

| Key | From | Entry | Cap |
| --- | ---- | ----- | --- |
| `dungeons` | `dungeon_plan`: the top ranked dungeons that have an upgrade | `{ name, line, item_id }`: `line` is the level range (and how far the character is outside it) and the top upgrade; `item_id` is that item | 3 |
| `talents` | `talent_plan`: the next points of the build | `{ line, spell_id }`: `line` is the node and the rank the point brings it to; `spell_id` is the node's spell, left out when the dataset has none | 3 |

Each list is left out when its plan has nothing to show (no dataset, no build, no upgrade).

**Structured line fields (schema 1.3).** A `next_quests` line keeps `text` and `quest` as before, for older
addons, and adds these when known. None is truncated; only `text` is capped (60 characters).

| Field | Meaning |
| ----- | ------- |
| `kind` | `"pickup"`, `"turnin"`, `"progress"`, `"talent"` or `"note"` |
| `title` | the full quest title |
| `level` | the quest's level (a number) |
| `class` / `elite` | `true` for a class quest / an elite or group quest; left out otherwise |
| `npc` | the giver of a pick-up, or the turn-in NPC |
| `zone` | the zone name |
| `objectives` | `{ { text = "Windsong Crawler Meat", have = 0, need = 6 }, ... }`, every objective of a logged quest including optional ones; `have` and `need` only when the capture has a count |
| `points` | the unspent talent point count, on the talent line |

An `upgrades` line adds `slot` (plain words: "Cloak", "Wrists", "Two-hand", "One-hand", "Shield", "Off hand"), `ilvl` (not shown to players) and `atLevel` (the level the item needs, only when the character is not yet that level) beside `text` and `item`.

**Upgrades you can get (schema 1.7).** An upgrades line is written only for an item with at least one dependable way
to get it, and carries `wear` ("Wear now" or "From level 12"), `gain` ("Big upgrade", "Solid upgrade" or
"Small upgrade"), `replaces` (the worn item's name alone; left out for an empty slot), `how` (the best way, one line) and
`pathKind` (`quest`, `craft`, `auction`, `dungeon` or `creature`, for the icon). The gain comes from the existing score
(item level plus quality, or the stat-weighted equivalent): 10 points or more, or an empty slot, is a big upgrade; 5 to
10 is solid; the rest is small. The section has `summary = "N upgrades you can get"`. A quest whose reward is an upgrade
has that as its `why` ("Rewards Traveler's Shawl, a big cloak upgrade"); a material needed for a craftable upgrade shows
in `keep_materials` with "for <item>". The line's `asks` follow the click: a fit question and a craft-or-save question for a craft.
`elite` is set only when the dataset's quests table has an `elite` or `group` column that says so.

**Stat check (schema 1.7, optional).** A character with a saved play style has `stats = { profile = "Fury · leveling", insights = { ... }, asks = { ... } }`: one to three plain sentences of at most 90 characters (what to raise and why, what is wasted, what is fine; never a number, percentage or id), the profile they were read for, and questions such as "How do I get more Strength?". It is left out with no play style. The weights behind it are in `docs/stats.md`. An upgrade's `why` may read "Adds the Strength you're short on".

**Training and details (schema 1.4).** From the dataset's `class_spells` table, `next_quests` gets one
`kind = "training"` line, after the talent and class lines, when the character's level unlocks trainer-taught
abilities or ranks. It covers the levels after the last capture's level, or the character's own level when
that is unknown (its `details` then say "Available at your level"). The addon filters out spells already
known.

| Field | Meaning |
| ----- | ------- |
| `level` | the character's level |
| `spells` | `{ { id = 6546, name = "Rend", rank = 2, new = false }, ... }` at most 8; `rank` only when the ability has one; `new = false` is an upgrade of a known rank |
| `npc`, `zone`, `x`, `y` | the class trainer, only when the capture holds an NPC named "<Class> Trainer" (the one in the character's zone first); `x`/`y` are map percent when captured. A trainer is never guessed, so these are usually absent |

Every `next_quests` line (except notes), `upgrades` line, `dungeons` entry, `talents` entry and the training line
also has:

| Field | Meaning |
| ----- | ------- |
| `details` | a list of 1 to 4 short strings (at most 120 characters each): a quest's reason, giver, location, level and open objectives (a turn-in that unlocks a follow-up adds `Unlocks: <Quest> (from <NPC>, here)`, see `plan-format.md`); an upgrade's slot, item level gain, source and level needed; a dungeon's level fit and top drop; the talent plan's next pick and why |

Only facts the advisor holds go in; no guide text.

A line's `text` is kept to about 110 characters by dropping an optional tail, and is never cut short (the complete
wording is in `details`); notes are never shortened. A section with nothing to say is left out.

**Natural asks and refs (schema 1.5).** Additive and optional: older addons ignore both fields. Every entry
of every section (`next_quests`, `upgrades`, `keep_materials`, `notes`) and every `dungeons` and `talents` entry has:

| Field | Meaning |
| ----- | ------- |
| `asks` | a list of 1 to 3 short questions in the player's voice, written for that entry, each at most 90 characters, complete and never cut. No "I'm a level 10 warrior": the advisor knows the character. Each names its subject (the item, the slot and suffix, the quest, the NPC, the talent or the ability), asks the natural next question the card does not answer, and reads whole when pasted alone: no "this" or "it" without a named antecedent, and nothing the entry's own text, why, how or details already state. Examples: "Would 'of the Bear' do as well as 'of Strength' for my cloak?", "Is it worth making Gemmed Copper Gauntlets now, or should I save the Copper Bar?", "What does Wind Hollow Essence lead to?", "Why does the Penance talent suit my play style?", "Is Lantern in the Mist worth the trip to Endaria Mistgaze now?" |
| `ref` | a stable subject reference, never shown to players: `quest:<id>`, `npc:<id>`, `talents`, `training:<level>`, `item:<id>`, `dungeon:<id>` or `note:<slug>`. Sent back with the question (the asks record's optional `ref`), the advisor expands it into the entry's facts and its place in the plan |

**Schema 1.8: no `prompt`, tidier strings.** The legacy `prompt` field is gone from every entry: addons 0.6.2 and
later use `asks` and `ref`, and the advisor builds its own question from the ref. `how` is one line of at most 90
characters ("Cheap on the Auction House: search 'of Strength', level 7–10"); an Auction House entry's full search steps
are its last `details` line. No player-facing string is cut mid-sentence or ends in "..." or an ellipsis: a line
that does not fit is shortened to a whole, shorter wording (the "(zone)" or "from <giver>" tail goes first) or kept whole.

**Schema 1.9: each fact once.** An upgrade's `text` is only the item's name, or for an Auction House entry the
shopping description ("any green mail chest, level 7–10, 'of Strength'"): no slot, no "Wear now", no gain, no
"instead of". `slot`, `wear`, `gain` and `replaces` carry those facts alone, and `replaces` is just the worn item's
name (absent for an empty slot). `details` and `why` do not repeat them either: an upgrade's `why` is only "Adds the
<stat> you're short on", when that is true. `slot` always matches the item's inventory type ("One-hand", "Main hand",
"Two-hand", "Off hand", "Shield", "Held in off hand"). Weapons follow what the character can use: an off-hand weapon
only once Dual Wield is learned (from the dataset's class spell levels), a shield only for a better shield, and a
two-hander over a one-hand and shield only for a two-hand or dual-wield damage play style. A craft's `how` leads with the skill check, e.g. "You can make it now (your Blacksmithing 45, needs 5)"; the
rest (where to learn the recipe, the materials) is in `details`.

**A "why" line (schema 1.6).** Additive and optional. `next_quests`, `upgrades`, `dungeons` and `talents` entries
may carry `why`: one short reason, at most 70 characters, plain words, no ids, never cut and never ending in "...".
It is built only from facts the advisor holds, and left out when none fits: a class quest says "Class quest — these
are worth doing first" (the dataset names no quest rewards, so none is claimed); a turn-in says what it unlocks or
"Done — hand it in on your way"; an unspent talent point says "Your plan's next pick: <talent>" or "Choose a play style
to get a pick"; training says "New: <ability>"; an upgrade says the stat it adds that you are short on; a
giver note gives the quest's level fit. The overlay, the addon panel and the card details show the same text.

**Companion API (for the overlay).** `GET /api/state` characters carry `suggestions`: at most 3, ranked (no play
style, a ready turn-in or class quest, an unspent talent point, a plan older than the newest save or dataset), each
`{id, label, why, action, question?, ref?}` with `action` one of `ask`, `play_style`, `refresh_plan`, `open_entry`;
`GET /api/suggestions?character=` returns the same list. `POST /api/refresh-plan` rewrites Digest.lua and Plan.lua from
the newest capture and answers `{message, new_steps, files}`. Each entry in `entries` has `why` and `class`;
`plan_steps` carry the action in `text` and the place in `x` and `y`. Questions are asked in the overlay only: the advisor no longer watches for in-game asks,
shows an "asked in game" card or writes `Answers.lua`; the in-game question chips copy the question for pasting.

**Giver notes.** A `notes` line with `kind = "giver"` (fields `npc`, `zone`, `x`, `y`, `quest`, `title`) appears
only when the NPC has a quest the character can pick up now (from the player's own Questie data) that is not
already a plan step, and whose NPC is not a pick-up giver in the plan. Its text is "<NPC> in <Zone> has a quest for you: <Title>". An NPC who is only
the turn-in of an open quest never gets a note, and with nothing to offer there is no giver note.

**Materials.** `keep_materials` is `professions.KeepList` (see `docs/professions.md`): bag and bank items
that are reagents for the character's recipes. Only when the dataset has no recipes for the
professions does it fall back to matching whole words of the item name against a per-profession
fragment table (`internal/digest/materials.go`, bags only); an item whose name holds an excluded
phrase (`Blood Shard`, `Soul Shard`, `Hearthstone`, ...) is never kept there. A profession missing
from the table keeps nothing.

**Where it is written.** The path is derived from the SavedVariables path:
`<game>/WTF/Account/<account>/SavedVariables/ForeverScribe.lua` gives
`<game>/Interface/AddOns/WoWCompanion_Digest/Digest.lua`, for Windows and Unix paths. It is an error,
not a created folder, when the slot addon is not installed. `--digest-path FILE` overrides (its folder
must exist). The file is replaced atomically (temp file, then rename).

**Data only.** Strings are escaped and the file holds one `WoWCompanionDigest = {...}` assignment of
literals, keys included. The tests run a Go port of the addon repo's `tests/slotcheck.lua` over the output.

## Route plan

Every time the digest is written, `Plan.lua` is written beside it: the character's quest route with map
coordinates. See `plan-format.md`.
