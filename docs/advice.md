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
4. **Quests to pick up** (`quest`, "Pick up: ..."): up to 3 quests not in the log, see below.
5. **Failed quests** (`quest`, "Abandon ..."): failed quests that cannot be completed.

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

**Quest chains.** When the dataset has a `quest_lines` table and a line holds a turn-in or open quest,
its reason ends with "step N of M in <line>" and its sources name the line. A turn-in that has a next
step also says "unlocks step N+1" and sorts ahead of the other turn-ins of its own zone; zone order is
unchanged, so the boost is slight. A dataset without `quest_lines` leaves every step as before. See
`chain.md`.

**Quests to pick up.** A quest that is not in the log is suggested when the dataset's `quests` row
gives its `level` within the character's level -2 to +2, and: it is not completed; it is not a DNT,
deprecated or placeholder name (the `items` pattern table); if it sits in a quest line, the step
before it is completed; `allowable_class_mask` and `allowable_race_mask` (when the table has them; 0
means anyone) allow the class and race; and its `zone_name` is the last-saved zone
(`snapshot.location`) or the zone of a quest in the log. Order: level fit, then continuing a quest
line already in the log, then a shared zone, then name. Phrase: "Pick up: X: level 9 quest, you are 9;
not in your log, in Zone, on the way ...". A rule the dataset cannot support is skipped and recorded
in `AvailableQuests(...).Unchecked`, never guessed; the built dataset (build 1.60.1.71037) has no
quest levels, names, masks or prerequisites, so on it this section stays empty. See
`design/live-guidance.md`.
