# inventory_review

`inventory_review(character)` sorts every bag and bank stack of a captured character into one
group, each with a short reason, and sums the vendor value and the bag slots freed. The digest's
`keep_materials` section is unchanged.

**Groups.** A stack takes the first group that fits:

1. **Equip now**: `gear.Upgrades` lists the item as an upgrade for one of the character's slots.
2. **Keep for professions**: `professions.KeepList` lists it (a reagent of a recipe learnable within
   50 skill points and not yet grey).
3. **Quest item**: an open (not failed) quest of the capture has an unfinished item objective whose
   text starts with the item's name. The dataset holds no quest-to-item requirement table, so a quest
   the capture does not state this for is not linked.
4. **Vendor**: grey (quality 0) in the dataset. The sell price is read from an `items.sell_price`
   column when the dataset has one; the current dataset has none, so the value is reported as
   unknown.
5. **Bank**: a reagent only of recipes more than 50 skill points above the character's skill
   (`professions.FarMaterials`), or equippable gear that is not an upgrade.
6. **Unknown**: everything else, including items the dataset has no row for.

**Summary.** *Vendor value* adds the priced vendor stacks; unpriced stacks are counted and left out,
and with none priced the value is "unknown". *Bag slots freed* counts the stacks in the bags that
fall in Vendor or Bank (stacks already in the bank free no bag slot).

Without a dataset, only quest links come from the capture; everything else is unknown. With no bank
capture the review covers the bags only and says so.

Operator check: run `inventory_review` against a real ForeverScribe capture and a built dataset.

## bags_across_characters

`bags_across_characters` reads every captured character at once. Read-only: nothing is mailed, sold or moved.

- `mode: find` (`item`): each character holding the item, with count and bags or bank; characters whose bank was never opened with the addon running are named ("bank not seen yet").
- `mode: review`: each stack is **Sell**, **Mail** (to a named alt), **Keep**, **Toss** or *Not sure*. Grey items sell; a reagent used only by a profession no character has sells (on the Auction House when it is tradeable and not grey); a reagent or gear upgrade another character uses is mailed to them; a quest item no character's log asks for is tossed (the dataset cannot say whether such a quest could still be started), and a quest item an alt's log asks for is tossed too, because a quest item is bound to its looter and cannot be mailed or traded; gear no character's class can use, or more than 10 levels below the wearer, sells; of food, drink and ammo only the best the character can use now is kept (a nectar or a dew is a drink; a warrior or a rogue has no mana, so a drink goes to an alt with mana, else it sells), a potion or poison made for other classes (the item's class list) goes to an alt of those classes, else it sells, and ammo the equipped ranged weapon cannot fire (arrows for a gun, bullets for a bow or a crossbow) is *Not sure*; a key is always kept (it is never tossed as a quest item, sold or mailed). Prices the dataset lacks stay unknown and are left out of the total.
- `mode: mail` (`character`): the lines above that are addressed to that character.
