# In-game digest

`wowc-advisor digest [NAME]` (and the MCP tool `update_digest`) writes the data-only
`WoWCompanion_Digest/Digest.lua` the WoW Companion panel shows (format: schema 2.0, one schema 1.2 entry
per character; the addon repo's `docs/digest-format.md`). Type `/reload` in game afterwards.

**Every captured character.** The file holds one entry for each character in your SavedVariables, so an
alt gets advice without asking again:

```lua
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1795271211,        -- unix seconds, when the file was written
  characters = {
    ["Testchar"] = { schema = "1.2", ... },   -- exactly the 1.2 digest below, for that character
    ["Otherchar"] = { schema = "1.2", ... },
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
| `upgrades` | one line per slot: the top `upgrade_finder` candidate and its first source, with the item id | 8 |
| `keep_materials` | bag and bank reagents of the character's professions' recipes still worth making (`professions.KeepList`), most numerous first | 10 |
| `notes` | unspent talents, (naming the build's next node, see talents.md), failed quests, no dataset, bank not captured, dataset build differs from the game | 6 |

The entry's schema 1.2 adds two optional top-level lists beside `sections`. Readers that only know 1.0 or 1.1 ignore
unknown keys.

| Key | From | Entry | Cap |
| --- | ---- | ----- | --- |
| `dungeons` | `dungeon_plan`: the top ranked dungeons that have an upgrade | `{ name, line, item_id }`: `line` is the level range (and how far the character is outside it) and the top upgrade; `item_id` is that item | 3 |
| `talents` | `talent_plan`: the next points of the build | `{ line, spell_id }`: `line` is the node and the rank the point brings it to; `spell_id` is the node's spell, left out when the dataset has none | 3 |

Each list is left out when its plan has nothing to show (no dataset, no build, no upgrade).

A line is at most 60 characters. A section with nothing to say is left out.

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
