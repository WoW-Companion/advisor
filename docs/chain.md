# quest_chain

`quest_chain(quest)` places a quest in its quest line. `quest` is an id or a name; a name is matched
exactly (ignoring case), then as a unique partial match, and an ambiguous one lists the candidates.
`names.parquet` lists no quests, so names come from a scan of the `quests` table's `name` column, and
a dataset without quest names takes ids only.

- The line comes from the `quest_lines` table (`line_id`, `name`, `position`, `quest_id`). A quest in
  several lines gets the one with the lowest id. Steps are ordered by `position` when the table has
  it, else by file order, and numbered from 1.
- The answer is the line's name, every step (with its quest name when the dataset has one), the
  asked-for quest's position, and the next quest, if any.
- If the dataset has no `quest_lines` table, or no line holds the quest, the tool says so.
- `whats_next` uses the same lookup: see `advice.md`.

Operator check: run `quest_chain` and `whats_next` against a real ForeverScribe capture and a built dataset.
