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
