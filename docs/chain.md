# quest_chain

`quest_chain(quest)` places a quest in its quest line. `quest` is an id or a name; a name is matched
exactly (ignoring case), then as a unique partial match, and an ambiguous one lists the candidates.
`names.parquet` lists no quests, so names come from a scan of the `quests` table's `name` column, and
a dataset without quest names takes ids only, except that a name is also matched against the titles
players reported in `quests_observed` (Everloot): an exact title first, else a unique partial one, and a title held by
several quests lists them with their ids.

- The line comes from the `quest_lines` table (`line_id`, `name`, `position`, `quest_id`). A quest in
  several lines gets the one with the lowest id. Steps are ordered by `position` when the table has
  it, else by file order, and numbered from 1.
- The answer is the line's name, every step (with its quest name when the dataset has one), the
  asked-for quest's position, and the next quest, if any.
- If the dataset has no `quest_lines` table, or no line holds the quest, the tool says so.
- `whats_next` uses the same lookup: see `advice.md`.

- Quest facts (level, giver with zone, turn-in, faction, category) fall back in this order, each field from the
  first source that has it: the dataset's client `quests` table, then `quests_observed` (Everloot, CC BY-SA;
  the giver's name and zone come from `npcs`), then the player's Questie. A fact set that used Everloot is
  marked `source: "everloot"` with `credit: "Everloot (CC BY-SA)"`, and reads "Seen by players on Everloot".

Operator check: run `quest_chain` and `whats_next` against a real ForeverScribe capture and a built dataset.
