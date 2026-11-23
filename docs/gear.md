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

- Required level at most the character's level + 2. An item above the character's level is shown
  as "at level N" (the digest line adds ", at level N" inside its brackets). A required level of 0
  is read as unknown, not as "none" (the client holds 0 for many high-level items, about 2,300
  equippable ones in the 1.60.1 dataset): the level is estimated as `max(1, item level - 5)` and
  put through the same window, and the tool output says "(estimated)". The same estimate replaces a known level more than 20 below it (the dataset has about 80 level-1 items of item level 30 to 70). Poor-quality items, artifact
  quality, quest-bonding items (bonding 4 and 5) and items with no item level are skipped.
- An item is an upgrade only when its item level is above the equipped item's, and above 1 when the
  slot is empty (level 1 items are placeholders and cosmetics).
- The two slots of a pair (finger1/finger2, trinket1/trinket2) never offer the same item id: the
  first slot keeps its candidates and the second takes the next ones, and neither offers the item
  the other wears.
- The item's `allowable_class_mask` must include the class.
- Armor: head, shoulder, chest (robes too), waist, legs, feet, wrist and hands must be the
  class's own armor type (Priest cloth, Rogue leather). Warriors and paladins wear mail, and
  hunters and shamans leather, until level 40, when they gain plate and mail. The level 40 rule
  is the Classic Era trainer rule from general game knowledge; the dataset holds no spell or
  trainer data to check it against. Cloaks, necks, rings and trinkets are never restricted. Shields only for Warrior,
  Paladin and Shaman.
- Weapons by subclass from the table in `rules.go` (Priest: dagger, one-hand mace, staff, wand;
  Rogue: dagger, fist, one-hand sword, mace and axe, bow, crossbow, gun, thrown). One-hand weapons
  and off-hand weapons fit the off hand only once the character has learned Dual Wield (the level in
  the dataset's `class_spells`, else warrior and hunter 20, rogue 10, shaman 40): never by level
  alone. A shield is replaced only by a better shield, and a one-hand-and-shield or dual-wield
  character keeps that style, unless the stored play style is a two-hand or dual-wield damage one
  (Arms, Fury, Enhancement, Retribution and the like). A two-hander is offered over a one-hand and
  off hand only then, and its gain is weighed against both slots together. No off hand is offered
  beside a worn two-hander.
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

## Internal and placeholder items

The client data lists developer-only, deprecated and test items (names such as "DNT HEAD", "Deprecated Battle Chain Pauldrons", "[PH] ...", "Monster - ...", "Test ..."). The items table has no column that marks them, so they are recognised by name, by the single table in `internal/items`. Upgrade, inventory, digest, dungeon, farm and profession answers never name one, a search by part of a name offers the real item before a placeholder, and asking about one (`where_drops`, `farm_spots`) answers that it isn't obtainable in game.
