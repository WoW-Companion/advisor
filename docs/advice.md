# What's next: ranking rules

`whats_next(character, limit=8)` returns steps in this order. Each step has a kind, a one-line
title, a reason, the zone when known, and its sources (a capture field and/or a dataset table and
row id).

1. **Turn-ins** (`turn_in`): completed quests. Zones with the most turn-ins come first, ties by
   zone name; quests with no known zone last.
2. **Talents** (`talent`): one step when points are unspent, naming the tree (group) with the most
   points spent, or the trees tied for it.
3. **Open quests** (`quest`): ordered by how close the quest level is to the character's. Quests
   more than 5 levels below (grey) or more than 3 above are skipped. A quest whose level is unknown
   (not in the capture, not in the dataset `quests` table) is listed after the others with the
   reason "quest level unknown"; it is never guessed.
4. **Failed quests** (`quest`, "Abandon ..."): failed quests that cannot be completed.

Quest level and zone come from the capture; when the capture lacks one, from the dataset
`quests` table's `level` and `zone` columns, if the table has them (the current client-derived
table has neither, so those stay unknown). The dataset row is named in the step's sources only
when it supplied a value.

**Profession** (`profession`) and **gear** (`gear`) steps are not produced: the dataset holds no
trainer or upgrade data yet, and the advisor omits what it cannot source.

Without a dataset, the capture alone is used. Without a captured level, no quest is filtered.

**Digest.** The in-game digest (schema 1.2, see `digest.md`) carries two more advice lists beside the
quest steps: `dungeons`, the top 3 dungeons of `dungeon_plan` that have an upgrade, each with its level
fit and best drop, and `talents`, the next 3 points of `talent_plan`. Each is left out when its plan
has nothing to show.
