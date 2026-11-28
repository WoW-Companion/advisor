package gear

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

func TestItemTooltipSoulboundWithoutPrice(t *testing.T) {
	for _, tc := range []struct {
		price     any
		want, not string
	}{
		{nil, "no vendor buys it", "only sold to a vendor"},
		{int64(500), "only sold to a vendor", "no vendor buys it"},
	} {
		tip, ok := ItemTooltip(store.Row{"name": "Synthetic Mount", "bonding": int64(1), "required_level": int64(40), "item_level": int64(40), "sell_price": tc.price})
		if !ok || !strings.Contains(tip.Trade, tc.want) || strings.Contains(tip.Trade, tc.not) {
			t.Errorf("price %v: trade %q, want %q and not %q", tc.price, tip.Trade, tc.want, tc.not)
		}
	}
}

func TestPieceTextSaysSlotTypeLimitLevelAndBinding(t *testing.T) {
	for _, tc := range []struct {
		row  store.Row
		want string
	}{
		{store.Row{"slot": int64(7), "class_id": int64(4), "subclass_id": int64(2), "required_level": int64(38), "bonding": int64(2), "allowable_class_mask": int64(-1)}, "legs, leather, requires level 38, Bind on Equip"},
		{store.Row{"slot": int64(1), "class_id": int64(4), "subclass_id": int64(4), "required_level": int64(0), "bonding": int64(1), "allowable_class_mask": int64(1)}, "head, plate, only warriors, Bind on Pickup"},
		{store.Row{"slot": int64(16), "class_id": int64(4), "subclass_id": int64(1), "required_level": int64(10), "bonding": int64(0), "allowable_class_mask": int64(-1)}, "back, requires level 10"},
		{store.Row{"slot": int64(20), "class_id": int64(4), "subclass_id": int64(1), "bonding": int64(0), "allowable_class_mask": int64(0)}, "chest, cloth"},
	} {
		if got := PieceText(tc.row); got != tc.want {
			t.Errorf("PieceText = %q, want %q", got, tc.want)
		}
	}
	if PieceText(nil) != "" {
		t.Error("PieceText(nil) is not empty")
	}
}
