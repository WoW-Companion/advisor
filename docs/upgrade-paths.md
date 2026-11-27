# Upgrade paths: rules

`upgrade_paths(character?, item? | slot?)` says how a character can dependably get an item, or the best
upgrades for a slot that can be got. The code is in `internal/paths`; the digest's upgrades section is built
on it. An upgrade is only an upgrade if the player can dependably get it: an item with no way is never listed.
Output uses names only, never an item, quest, recipe or NPC number.

## Ways, in order

1. **Quest reward in the log.** `loot_sources` rows of a quest kind. The quest is in the log. "Finish <quest> and
   choose it" when the quest has several rewards, else "Finish <quest> for it", with the giver and zone when the
   dataset holds them.
2. **You can make it, with every material on hand.** A `recipes` row creating the item, in a profession the
   character has, whose learn skill is at most their current skill. Each reagent from `recipe_reagents` shows
   have/need from bags and bank.
3. **Auction House.** Only an item that can be traded (not Bind on Pickup, not a quest item, uncommon quality or
   better, with a known binding) and is either a random world drop (the test below) or Bind on Equip from any
   source. "Common world drop. Usually cheap on the Auction House." There is no price data (Auction House
   prices from ForeverScribe are too thin to depend on), so no price is ever read, ranked on or shown; the server
   is assumed to keep these cheap.
3b. **Vendor** (Everloot's `vendor_sources`, credited "Everloot (CC BY-SA)"). When a named vendor sells the item,
   "Buy from <vendor> in <zone> for <price>", with "(limited stock)" when the offer is limited; the cheapest
   offer is used. A price up to 1 gold per ten character levels (1g at levels 1-9, 2g at 10-19, ...) is cheap
   and ranks beside the Auction House; a dearer one ranks below dungeon bosses and crafts but above farming a
   creature. A craft reagent a vendor sells gets "buy from <vendor> ..." in its how-to-get. No vendor row, no
   path. Auction prices are still never read. `upgrade_finder` shows the same vendor on each candidate.
4. **Other quests**: in a plan or available now (not completed, not failed, no more than 2 levels above the
   character), not yet picked up.
5. **Dungeon boss.** A boss (`instance_bosses`) of a dungeon whose level range is within 3 levels of the character,
   dropping the item "often" (over 15%) or "sometimes" (5% to 15%). A boss of an alternate mode of the dungeon
   (difficulty above 1, when the map has ordinary encounters) is not one of its bosses and is never named.
6. **You can make it, missing materials.** Every missing reagent needs a way: gather it with the character's own
   gathering profession, buy it from a vendor, or buy it on the Auction House when it can be traded; otherwise the
   craft is dropped. See the skill check below.
7. **Named creature.** One to three creatures carry it at 10% or more, each with `tier` verified or confirmed, in the
   character's level range (5 below to 3 above); never more than 5 creature sources in all.

## Random world drops

More than 5 creature sources, or every creature source under 5%. A tradeable one is an Auction House item
(above); a Bind on Pickup one has no way and gets a reason ("a random drop, not worth chasing"), and the tool
names the best gettable item for its slot.

## Slot shopping list

For a slot whose best way is the Auction House, or that has no quest or craft, `upgrade_paths` (and the digest's
upgrade entry) gives a shopping list instead of one item: the slot and armor type, a level range of up to five
levels ending at the character's level, "green or better", the stat suffixes that fit the class and the stored
role, and one to three example base items from the dataset in that range. Example: "Chest: any green mail chest,
level 8-10, 'of the Bear' - cheap on the Auction House", with the in-game search steps.

The suffix table (`internal/paths/shopping.go`) is a small set of game facts: of the Bear (Strength + Stamina), of the
Tiger (Strength + Agility), of the Monkey (Agility + Stamina), of the Eagle (Stamina + Intellect), of the Owl
(Intellect + Spirit), of the Falcon (Agility + Intellect), and the single stats. A suffix is offered only when the
dataset holds an item with that suffix in its name, and, where that item's stats are known, the stats match; the
repo's dataset notes record no random-property table, so nothing else can confirm one. A suffix that cannot be
verified is left out.

## Crafting: the skill check

Every craft path carries a `skill` object (`profession`, `have`, `need`, `gap`, `status`) and a `recipe_source`,
and its line leads with the check against the character's captured profession rank:

- `can_make`: rank at or above the recipe's learn skill: "You can make it now (your Blacksmithing X, needs N)".
- `short`: a gap of at most `ShortMax` (25) points: "You're at X, it needs N: Y points to go", then `skill_up`:
  up to 3 recipes the character knows or can learn now that are orange or yellow at their rank, those whose
  materials are held (bags and bank) first. Crafts are estimates (orange about 1 point per craft, yellow about 1
  per 2) and each step lists the materials still missing.
- `far`: a larger gap: one line, leading with the Auction House (only when the item can be traded) and a crafter.
- `not_profession`: "Made by Blacksmiths (skill N). You don't have Blacksmithing", with the same other ways.

`far` and `not_profession` paths are `informational`: they never count as a dependable way and are never
the best path. When the capture lists known recipes (`KnownSpells`) it says "you already know the recipe" or
"learn it first"; otherwise "learn it if you haven't". `recipe_source` names where to learn it: the recipe item's
vendor (Everloot `vendor_sources`: vendor, zone, price), the creature or quest it drops from, else a profession
trainer. The recipe item is `recipes.recipe_item_id` when the dataset has it, else the recipe item (item class 9)
named "<Kind>: <recipe>"; a recipe item with no known source says so rather than "trainer".
