# In-game digest

`wowc-advisor digest [NAME]` (and the MCP tool `update_digest`) writes the data-only
`WoWCompanion_Digest/Digest.lua` the WoW Companion panel shows (format: schema 1, the addon repo's
`docs/digest-format.md`). Type `/reload` in game afterwards.

The digest adds no advice logic: it is built from `whats_next` and `upgrade_finder`.

| Section | From | Cap |
| ------- | ---- | --- |
| `next_quests` | turn-ins and open quests from `whats_next`, with their zone and quest id | 8 |
| `upgrades` | one line per slot: the top `upgrade_finder` candidate and its first source, with the item id | 8 |
| `keep_materials` | bag and bank reagents of the character's professions' recipes still worth making (`professions.KeepList`), most numerous first | 10 |
| `notes` | unspent talents, failed quests, no dataset, bank not captured, dataset build differs from the game | 6 |

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
literals. The tests run a Go port of the addon repo's `tests/slotcheck.lua` over the output.
