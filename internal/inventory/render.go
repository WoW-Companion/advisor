package inventory

import (
	"fmt"
	"strings"
)

var titles = map[Group]string{
	Equip:   "Equip now",
	Keep:    "Keep for professions",
	Quest:   "Quest items",
	Vendor:  "Vendor",
	Bank:    "Bank",
	Unknown: "Unknown",
}

// Money formats copper as gold, silver and copper.
func Money(c int64) string {
	return fmt.Sprintf("%dg %ds %dc", c/10000, c/100%100, c%100)
}

// Render is the plain-text form of a result.
func Render(r Result) string {
	var b strings.Builder
	total := 0
	for _, g := range Order {
		total += len(r.Groups[g])
	}
	if total == 0 {
		b.WriteString("No bag or bank items captured for this character.\n")
	}
	for _, g := range Order {
		l := r.Groups[g]
		if len(l) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s (%d)\n", titles[g], len(l))
		for _, e := range l {
			fmt.Fprintf(&b, "- %s x%d (%s): %s", e.Name, e.Count, e.Where, e.Reason)
			if g == Vendor {
				if e.SellPrice != nil {
					fmt.Fprintf(&b, "; sells for %s", Money(*e.SellPrice*int64(e.Count)))
				} else {
					b.WriteString("; sell price unknown")
				}
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if len(r.Groups[Vendor]) > 0 {
		if r.VendorUnpriced == len(r.Groups[Vendor]) {
			b.WriteString("Vendor value: unknown\n")
		} else {
			fmt.Fprintf(&b, "Vendor value: %s", Money(r.VendorValue))
			if r.VendorUnpriced > 0 {
				fmt.Fprintf(&b, " (%d stack(s) unpriced, left out)", r.VendorUnpriced)
			}
			b.WriteString("\n")
		}
	}
	fmt.Fprintf(&b, "Bag slots freed: %d\n", r.SlotsFreed)
	for _, n := range r.Notes {
		b.WriteString("Note: " + n + "\n")
	}
	return b.String()
}
