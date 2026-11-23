# Stat check: rules

`stat_check(character?)` reads a character's stats against the stat priorities of their saved play style and
answers in one to three plain insights. The code is in `internal/stats`. Upgrade ranking (`internal/gear`), the
Auction House suffix list (`internal/paths`) and the digest use the same priorities, so the advice agrees everywhere.

## Priorities are ballpark

`internal/stats/profiles.go` is our own curated table, in our own words (no guide text is copied): per class, spec and
role (damage, tank, healer), ordered stat weights from 0 to 1 and a one-line plain reason per stat. **The weights are
ballpark guidance for leveling and dungeon play, not a simulation; players differ, and the answer says so in one
clause at most.** Every class has a balanced fallback (its first profile) for a character with no spec chosen.
A spec the player chose (or the top talent group) wins when it names a profile of the class; else the role's first
profile; else the class fallback.

**Stat preference** is optional and stored with the play style (`stat_pref`: `balanced`, `damage` or `survival`;
shown as Balanced, More damage, More survivability). "More damage" lifts the damage stats and eases Stamina and the
defensive stats; "More survivability" does the reverse.

## What the check reads

1. The addon's recorded totals, `WoWCompanionDB.stats[<character key>] = { at, level, str, agi, sta, int, spi,
   armor, ap, crit, dodge, parry, block }`, every field optional and read tolerantly.
2. When there are none, gear-only totals: how many worn items carry each stat, from the dataset's item stats. The
   result says which source it used (`source`: `captured`, `gear` or `none`).

Each of Strength, Agility, Stamina, Intellect and Spirit is compared as a share of the five against the share the
profile wants. The most short stat that matters is "what to raise"; a stat the profile barely uses that the gear
carries is "wasted" (the worn items are listed to swap first); a stat that matters and is covered is "fine". At low
levels the check adds that the numbers are small and quest gear matters more than fine-tuning.

## What the player sees

At most three insights of at most 90 characters, never a number, ratio, percentage or id. The totals, per-stat lean and
weights are returned for the answer's own reasoning (and the report) only. With no play style the result is
`needs = "play_style"` with the hook "Choose a play style for a stat check", like the talent question.

The digest's character entry (schema 1.7) gains an optional `stats = { profile, insights, asks }`.
