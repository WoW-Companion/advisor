package gear

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// Tooltip is what an item's tooltip shows that the dataset holds: its levels, how it binds and what that means for a trade, a mail and
// the Auction House. The stack size and a unique limit are on the tooltip in the game but in no column of the dataset.
type Tooltip struct {
	Name          string
	ItemLevel     int
	RequiredLevel int // 0 when the dataset lists none
	Binding       string
	Trade         string // what the binding allows between players
	SellPrice     *int64
}

// Tradeable reports whether the dataset's bonding lets an item pass between players (trade window, mail, Auction House): a piece bound on
// pickup (1) and a quest item (4, 5) cannot; every other binding can, grey items included.
func Tradeable(bonding int64) bool { return bonding != 1 && bonding != 4 && bonding != 5 }

// ItemTooltip reads the tooltip of an item row; ok is false for no row.
func ItemTooltip(row store.Row) (Tooltip, bool) {
	if row == nil {
		return Tooltip{}, false
	}
	t := Tooltip{ItemLevel: int(i64(row["item_level"])), RequiredLevel: int(i64(row["required_level"]))}
	t.Name, _ = row["name"].(string)
	if p, ok := row["sell_price"].(int64); ok {
		t.SellPrice = &p
	}
	b := i64(row["bonding"])
	t.Binding = bindingNames[b]
	if b == 0 && i64(row["class_id"]) == 12 {
		t.Binding = "quest item"
		b = 4
	}
	switch b {
	case 1:
		t.Trade = "soulbound when picked up: it cannot be traded, mailed or put on the Auction House, only sold to a vendor"
		if t.SellPrice == nil {
			// the client stores a zero sell price as null: no vendor buys the item, so there is nowhere to sell it
			t.Trade = "soulbound when picked up: it cannot be traded, mailed or put on the Auction House, and the data lists no vendor sell price for it, so no vendor buys it"
		}
	case 2:
		t.Trade = "binds when equipped: until it is worn it can be traded, mailed and put on the Auction House; once worn it is soulbound"
	case 3:
		t.Trade = "binds when used: until it is used it can be traded, mailed and put on the Auction House"
	case 4, 5:
		t.Trade = "bound to the one who looted it: it cannot be traded, mailed or put on the Auction House"
	default:
		t.Trade = "it binds to no one: it can be traded, mailed and put on the Auction House"
	}
	return t, true
}

// Text is the tooltip as one line.
func (t Tooltip) Text() string {
	req := "no required level listed in the data"
	if t.RequiredLevel > 0 {
		req = fmt.Sprintf("requires level %d", t.RequiredLevel)
	}
	parts := []string{fmt.Sprintf("item level %d", t.ItemLevel), req}
	if t.Binding != "" {
		parts = append(parts, t.Binding)
	}
	parts = append(parts, t.Trade)
	if t.SellPrice != nil {
		parts = append(parts, sellText(*t.SellPrice))
	}
	return strings.Join(parts, "; ") + "; stack size and any unique limit: not in the game data"
}

// sellText is the vendor's price in the game's money units, with the copper it is made of: a bare copper count is easy to misread as
// silver or gold.
func sellText(c int64) string {
	if m := vendors.Money(c); m != "" {
		return fmt.Sprintf("sells to a vendor for %s each (%d copper); the data holds no Auction House price to compare it with", m, c)
	}
	return "no vendor buys it: its sell price is 0"
}

// slotNames are the inventory types of the armor slots as a player says them.
var slotNames = map[int64]string{1: "head", 2: "neck", 3: "shoulder", 5: "chest", 20: "chest", 6: "waist", 7: "legs", 8: "feet", 9: "wrist", 10: "hands", 11: "ring", 12: "trinket", 16: "back"}

// PieceText is what an item row says about the piece itself: its slot, armor type, the classes its class limit names, the level it needs and how it binds
// ("legs, leather, requires level 38, Bind on Equip"). "" for no row. Only what the row holds is said.
func PieceText(row store.Row) string {
	if row == nil {
		return ""
	}
	var parts []string
	if s, ok := slotNames[i64(row["slot"])]; ok {
		parts = append(parts, s)
	}
	if w := ArmorTypeWord(int(i64(row["class_id"])), int(i64(row["subclass_id"]))); w != "" && i64(row["slot"]) != 16 {
		parts = append(parts, w)
	}
	if ClassRestricted(row) {
		parts = append(parts, "only "+ClassList(row))
	}
	if r := i64(row["required_level"]); r > 0 {
		parts = append(parts, fmt.Sprintf("requires level %d", r))
	}
	if b := bindingNames[i64(row["bonding"])]; b != "" {
		parts = append(parts, b)
	}
	return strings.Join(parts, ", ")
}
