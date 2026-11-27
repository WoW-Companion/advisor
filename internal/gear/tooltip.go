package gear

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
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
		parts = append(parts, fmt.Sprintf("sells to a vendor for %d copper each", *t.SellPrice))
	}
	return strings.Join(parts, "; ") + "; stack size and any unique limit: not in the game data"
}
