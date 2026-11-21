# farm_spots

`farm_spots(character, item, limit=5)` lists the creatures and objects that drop an item. `item`
is an item id or a name; a name is matched exactly, then by prefix, after folding case, accents and punctuation
(`Thunderfury's` is `thunderfurys`), and an ambiguous one lists the candidates. Names are found
through the dataset's `names.parquet` index; a dataset without it is scanned by the `items` table's
`name` column instead (any part of a name matches), and the server logs that it did; the knowledge-only server refuses this tool because it reads the character.

- **Sources** come from `loot_sources` rows with kind `creature` or `object`, one spot per source
  (rows of the same source are summed).
- **Drop chance** is `100 * drops / kills` over the source's loot windows. With no kills recorded
  it is unknown, never zero.
- **Level and zone** come from the `npcs` row (`min_level`, `max_level`, `zone_name`, and
  `zone_source` when the table has it, else the zone is attributed to `npcs.zone_name`). Objects
  have no table, so their level and zone are unknown. A missing value is said to be unknown, never
  guessed.
- **Rank**: sources whose level range overlaps `[level-5, level+3]` of the character first (the
  same window as `upgrade_finder`'s sources), then by drop chance, highest first, unknown chances
  after known ones, then kind and id.
- **Crafted** sources (a kind naming crafting, a recipe or a spell) are left out; the result says
  the item can be crafted and points to `profession_plan`.
- `profession_plan` ends with a one-line hint to `farm_spots` when a reagent is missing.

Operator check: run `farm_spots` against a real ForeverScribe capture and a built dataset.
