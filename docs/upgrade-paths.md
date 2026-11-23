# Upgrade paths: rules

`upgrade_paths(character?, item? | slot?)` says how a character can dependably get an item, or the best
upgrades for a slot that can be got. The code is in `internal/paths`; the digest's upgrades section is built
on it. An upgrade is only an upgrade if the player can dependably get it: an item with no way is never listed.
Output uses names only, never an item, quest, recipe or NPC number.

## Ways, in order (dependability, then effort)

1. **Quest reward.** `loot_sources` rows of a quest kind. The quest is in the log, a plan step, or available now
   (not completed, not failed, no more than 2 levels above the character). "Finish <quest> and choose it" when the
   quest has several rewards, else "Finish <quest> for it", with the giver and zone when the dataset holds them.
2. **You can make it.** A `recipes` row creating the item, in a profession the character has, whose learn skill is at most
   their current skill. Each reagent from `recipe_reagents` shows have/need from bags and bank. Every missing reagent
   needs a way: gather it with the character's own gathering profession (node and zones from the `objects` table when
   present), buy it from a vendor, or buy it on the Auction House when it can be traded; otherwise the craft is dropped.
   The capture does not say which recipes are known, so the line says the recipe needs skill N and to learn it from a
   trainer if you haven't.
3. **Auction House.** Only tradeable items: not Bind on Pickup, not a quest item, common quality or better, with a known
   binding. "Usually on the Auction House (it's Bind on Equip)", or "about 1g 20s (seen <date>)" only when a real
   auction price is held. The vendor sell price (`items.sell_price`) is never shown as an auction price. An item with no
   source rows and no real price is not offered.
4. **Dungeon boss.** A boss (`instance_bosses`) of a dungeon whose level range is within 3 levels of the character,
   dropping the item "often" (over 15%) or "sometimes" (5% to 15%).
5. **Named creature.** One to three creatures carry it at 10% or more, each with `tier` verified or confirmed, in the
   character's level range (5 below to 3 above); never more than 5 creature sources in all.

## Not dependable

Random world drops: more than 5 creature sources, or every creature source under 5%. Unknown sources. Bind-on-pickup
items whose only sources are world drops. Such an item gets a reason ("a random drop, not worth chasing") and the tool
names the best gettable item for its slot.

## Auction prices

ForeverScribe's SavedVariables hold an `auctions` table that this advisor ignores (docs/foreverscribe-format.md), and the
character model has no price field. So no auction price is shown today; the code takes real prices through
`paths.Options.Prices` (copper and the day seen) and shows only those.
