package farm

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func levels(s Spot) string {
	switch {
	case s.MinLevel != nil && s.MaxLevel != nil && *s.MinLevel != *s.MaxLevel:
		return fmt.Sprintf("level %d-%d", *s.MinLevel, *s.MaxLevel)
	case s.MinLevel != nil:
		return fmt.Sprintf("level %d", *s.MinLevel)
	case s.MaxLevel != nil:
		return fmt.Sprintf("level %d", *s.MaxLevel)
	}
	return "level unknown"
}

// Render is the plain-text form of the first limit spots.
func Render(item string, spots []Spot, crafted bool, limit int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Farm spots for %s\n", item)
	if len(spots) == 0 {
		if crafted {
			b.WriteString("No creature or object drops it in the dataset: it is crafted, see profession_plan.\n")
			return b.String()
		}
		b.WriteString("No creature or object drops it in the dataset.\n")
	}
	for i, s := range spots {
		if i == limit {
			fmt.Fprintf(&b, "(%d more not shown)\n", len(spots)-limit)
			break
		}
		name := s.Name
		if name == "" {
			name = s.Kind + " " + itoa(s.ID)
		} else {
			name += " (" + s.Kind + " " + itoa(s.ID) + ")"
			if s.Everloot {
				name += " (name from Everloot)"
			}
		}
		zone := "zone unknown"
		if s.Zone != "" {
			zone = s.Zone + " (from " + s.ZoneSource + ")"
		}
		chance := "chance unknown"
		if s.Chance != nil {
			chance = fmt.Sprintf("%.1f%% of %d loot windows", *s.Chance, s.Kills)
		}
		fit := ""
		if s.InRange {
			fit = ", in your level range"
		}
		fmt.Fprintf(&b, "%d. %s: %s, %s, %s%s\n", i+1, name, levels(s), zone, chance, fit)
	}
	if crafted {
		b.WriteString("It can also be crafted: see profession_plan.\n")
	}
	return b.String()
}

// Value is the line that says what a drop is worth, for a farming answer: the vendor's price for the item from
// items.sell_price. The dataset holds no Auction House price and no time per kill, so it says so and gives no gold
// per hour. "" when the item has no sell price.
func Value(ds store.DataStore, itemID int64) string {
	if ds == nil {
		return ""
	}
	it, err := ds.Get("items", itemID)
	if err != nil {
		return ""
	}
	m := vendors.Money(i64(it["sell_price"]))
	if m == "" {
		return ""
	}
	return "A vendor pays " + m + " for each. The dataset has no Auction House price, so no gold per hour is given.\n"
}
