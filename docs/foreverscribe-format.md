# ForeverScribe SavedVariables format

The advisor reads the file the [ForeverScribe](https://github.com/MattMaximo/foreverscribe) addon
(MIT) writes in the game. This page records the format the reader targets.

- **Pinned commit:** `903be32d3dcc6596a204aa691b6bf794829f061a` (addon version `0.10.1`,
  `FS.SCHEMA = 1`).
- **Sources read:** `addon/ForeverScribe/Core.lua`, `addon/ForeverScribe/Collect.lua`,
  `addon/ForeverScribe/Util.lua`, and `docs/CAPTURE-MATRIX.md`.
- **File:** `WTF/Account/<account>/SavedVariables/ForeverScribe.lua`, written by the game client on
  `/reload` or logout. It holds one global assignment, `ForeverScribeDB = { ... }`.

The reader (`wowc_advisor.character`) never executes the file. `luatable.py` tokenises and parses
the literal subset the client writes: tables, strings with escapes, numbers, booleans, nil, keys
`["x"]`, `[1]` and bare names, and `--` comments (the client appends `-- [n]` to array entries).
A malformed file raises `LuaParseError`, whose message starts `line N:`.

## Top level (`Core.lua`, `H.ADDON_LOADED`)

| Key | Holds |
|---|---|
| `schema`, `addonVersion` | `FS.SCHEMA` and `FS.VERSION` |
| `chars["<Realm>:<Name>"]` | one record per character (`StartCharacter`) |
| `lastChar`, `settings`, `sessionCounter` | addon bookkeeping; not read |

## Character record (`chars[key]`)

| Key | Holds | Model field | Source |
|---|---|---|---|
| `snapshot.identity` | `name`, `realm`, `class` (class file name, e.g. `MAGE`), `race`, `faction`, `guild` | `name`, `realm`, `class_name`, `race`, `faction` | `Core.lua` `Identity()` |
| `snapshot.level`, `xp`, `xpMax`, `rested`, `money` (copper) | progress | `level`, `xp`, `xp_max`, `rested`, `money` | `Core.lua` `Progress()` |
| `snapshot.capturedAt` | Unix time of the last snapshot | `captured_at` | `SnapshotFast`, `SnapshotFull` |
| `snapshot.equipment[slotName]` | `itemID`, `link`, `name`, `ilvl`, `quality`, `enchant`, `dur`, `durMax`, `stats` | `equipment` (slot name to `EquippedItem`) | `Core.lua` `Equipment()`; the enchant is parsed from the item link by `Util.lua` `ParseItemLink` and is absent when 0 |
| `snapshot.bags.items[itemID]` | `itemID`, `count`, `name`, `link`, `quality` | `bags` (`ItemStack`) | `Core.lua` `ScanContainers` |
| `bank.items[itemID]` | the same shape; present only after the player opened the bank | `bank` | `Core.lua` `ScanInventory` |
| `snapshot.quests[i]` | `questID`, `title`, `level`, `zone`, `complete`, `failed`, `obj[j] {text, have, need, done, type}` | `quests` | `Core.lua` `QuestLog()` |
| `snapshot.questsCompleted` | sorted list of completed quest IDs | `completed_quest_ids` | `Core.lua` `CompletedQuests()` |
| `snapshot.skills[i]` | `name`, `skillID`, `rank`, `max`, `mod`, `group` (the header the skill sat under) | `skills`; `professions` is the subset whose group names professions | `Core.lua` `Skills()` |
| `snapshot.talents` | `system` (`traits` or `classic`), `loadout`, `unspent`, `trees[i] {name, points, talents[j] {nodeID, name, rank, max, spellID}}` | `talents` | `Collect.lua` `C.Talents`, `TraitTree` |
| `reputation.factions[i]` | `id`, `name`, `standing`, `value`, `min`, `max`, `atWar`, `group` | `reputations` | `Collect.lua` `C.Reputation` |

Sections the reader does not model (spellbook, stats, durability summary, Legacy trees,
achievements, mail, auctions, NPC sightings, events) are present in the file and ignored.

## Rules the reader follows

- A section that is missing from the file becomes `None` on the model. An empty section that is
  present (for example `quests = {}`) becomes an empty list. Nothing is filled in.
- `professions` rests on the `group` header text the client reports for a skill (`Professions`,
  `Primary Skills`, `Secondary Skills`, or any group containing "profession"). The capture matrix
  marks skills "seen", but the header text is client-supplied, so a client that words it
  differently yields an empty `professions` list while `skills` stays complete.
- ForeverScribe keeps the last good value rather than overwrite it with an empty teardown read
  (`docs/CAPTURE-MATRIX.md`, "Teardown rule"), so a section can be older than `captured_at`.
- Several sections are marked *unverified* in the capture matrix (reputation, for one). The
  fixtures follow the writer's code, not a real file.
