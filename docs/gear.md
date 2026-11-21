# Gear upgrades: rules

`upgrade_finder(character, slot?, limit=3)` lists, per slot, items the character's class can use,
that they can equip, and that beat what they wear. The code is in `internal/gear`.

## What the dataset has

The `items` table has: `item_level`, `required_level`, `quality`, `class_id` (armor 4, weapon 2),
`subclass_id`, `slot` (inventory type), `allowable_class_mask`, `bonding`, `stat_types` (the stat
type ids an item carries) and `stats_status` (`known`, `none`, `unknown`). **It has no stat
values**, only which stats an item has. `loot_sources` has the source kind (creature, object,
quest reward), the source id, kills, drops and rate. The `npcs` table has a creature's name and
level range. There is no object table and no zone on any of them, so object sources show as
"object <id>" and every source says "zone unknown".

## Which items

- Required level at most the character's level + 3. A required level of 0 is read as unknown, not
  as "none" (the client holds 0 for many high-level items, about 2,300 equippable ones in the
  1.60.1 dataset): such an item is offered only when its item level is at most the character's
  level + 3, and is shown as "requires level unknown". Poor-quality items and items with no item level
  are skipped.
- The item's `allowable_class_mask` must include the class.
- Armor: head, shoulder, chest (robes too), waist, legs, feet, wrist and hands must be the
  class's own armor type (Priest cloth, Rogue leather; mail and plate for the classes that wear
  them). Cloaks, necks, rings and trinkets are never restricted. Shields only for Warrior,
  Paladin and Shaman.
- Weapons by subclass from the table in `rules.go` (Priest: dagger, one-hand mace, staff, wand;
  Rogue: dagger, fist, one-hand sword, mace and axe, bow, crossbow, gun, thrown). One-hand weapons
  and off-hand weapons fit the off hand only for classes that dual-wield. Two-handers are compared
  with the main hand only.
- Shirts, tabards and bags are not considered. A class with no entry in `rules.go` (Death
  Knight, Monk, Demon Hunter, Evoker) is reported as unsupported.

## Score and ranking

With the spec's weights and known stat types on both the item and the item it replaces:
`score = item_level × (0.5 + 0.5 × mean weight of the item's stat types) + quality`.
Otherwise `score = item_level + quality`, and the answer says it is ranked by item level only and
why (no stats in the dataset, spec unknown, or no weights for the spec). The comparison for a slot
always uses one basis for both items. A slot's candidates are the items whose score is higher than the
equipped item's (an empty slot takes any item), best gain first. An equipped item the dataset does
not hold is compared by its captured item level and quality.

Weights are in `weights.json`: advisor defaults, not authoritative, one rationale per spec. The
spec is the talent group holding the most points, matched by name inside the group's name.

## Sources

Per candidate, up to 3 sources: creatures whose level range reaches within 5 below to 3 above the
character first, then by drop chance (drops / kills, shown as "of kills"), then by id. A candidate
with no `loot_sources` row says "source unknown".
