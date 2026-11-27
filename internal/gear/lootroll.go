package gear

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
)

// Roll is what a player rolls on a dropped item in a group.
type Roll string

const (
	RollNeed    Roll = "Need"
	RollGreed   Roll = "Greed"
	RollPass    Roll = "Pass"
	RollUnknown Roll = "Unknown"
)

// LootRule is the loot roll for an item: what to roll, how the item binds and why.
type LootRule struct {
	Roll    Roll
	Binding string // "Bind on Pickup", "Bind on Equip", "Bind on Use", "quest item" or "" when the item does not bind
	Why     string
}

// Text is the rule as one line.
func (r LootRule) Text() string {
	out := string(r.Roll)
	if r.Binding != "" {
		out += " (" + r.Binding + ")"
	}
	return out + ": " + r.Why
}

// bindingNames are the dataset's item binding values: 0 none, 1 on pickup, 2 on equip, 3 on use, 4 and 5 quest items.
var bindingNames = map[int64]string{1: "Bind on Pickup", 2: "Bind on Equip", 3: "Bind on Use", 4: "quest item", 5: "quest item"}

// LootRoll is what a character of the class token at level rolls on the dataset item row. Need is for an item the character can use
// (worn gear their class can wear or wield); Greed is for anything they cannot use or that is not gear; a quest item is not rolled for.
// Whether a usable piece beats what is worn is the gear advice's call, so the rule says to Need only if it does. An item the dataset
// does not hold is Unknown, never a guess.
func LootRoll(class string, level int, row store.Row) LootRule {
	if row == nil {
		return LootRule{RollUnknown, "", "the dataset has no row for this item, so how it binds and whether you can use it are not known"}
	}
	b := i64(row["bonding"])
	bind := bindingNames[b]
	var bound string
	switch b {
	case 1:
		bound = "it binds to you when you pick it up and cannot be traded or sold to a player afterwards, so Need only if you will use it"
	case 2:
		bound = "it binds when equipped, so until then it can be traded or sold: Need if you will wear it, else Greed to sell it"
	}
	if b == 4 || b == 5 || i64(row["class_id"]) == 12 {
		return LootRule{RollPass, bind, "a quest item is looted for the quest, not rolled for"}
	}
	usable, known := CanUse(class, level, row)
	switch {
	case !known:
		why := "it is not worn gear the class rules cover, so Greed unless you will use it yourself"
		if bound != "" {
			why += "; " + bound
		}
		return LootRule{RollGreed, bind, why}
	case !usable:
		return LootRule{RollGreed, bind, fmt.Sprintf("a %s cannot wear or wield it (armor or weapon type, or class restriction), so Greed for its vendor price", strings.ToLower(class))}
	}
	why := "your class can use it: Need only if it beats what you wear in that slot, else Greed"
	if bound != "" {
		why += "; " + bound
	}
	return LootRule{RollNeed, bind, why}
}
