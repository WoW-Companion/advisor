package ask

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
)

// ammoBagAsk is a question that names a quiver, an ammo pouch or a class bag (a soul, herb, enchanting, mining or gem bag): who may use it, which ammo it holds, what level it needs.
var ammoBagAsk = regexp.MustCompile(`(?i)\b(?:quivers?|ammo ?pouch(?:es)?|ammo bags?|soul (?:bags?|pouch(?:es)?)|felcloth bags?|herb(?:alism)? bags?|enchanting bags?|mining bags?|gem bags?|class bags?|special bags?)\b`)

// AmmoBagFacts says what the dataset holds for a quiver, an ammo pouch or a class bag: the class limit on each row and the ammo each holds. "" for any other question, or without a dataset.
func AmmoBagFacts(ds store.DataStore, text string) string {
	if ds == nil || !ammoBagAsk.MatchString(text) {
		return ""
	}
	rows, err := ds.Filter("items", store.Range{Column: "slot", Min: int64(18), Max: int64(18)})
	if err != nil {
		return ""
	}
	var quivers, limited int
	classBags := map[string]bool{}
	for _, r := range rows {
		cl, _ := r["class_id"].(int64)
		switch {
		case cl == 11:
			quivers++
			if gear.ClassRestricted(r) {
				limited++
			}
		case cl == 1 && gear.ClassRestricted(r):
			name, _ := r["name"].(string)
			classBags[name+" ("+gear.ClassList(r)+" only)"] = true
		}
	}
	var b strings.Builder
	b.WriteString("Quivers, ammo pouches and class bags: ")
	if quivers > 0 {
		fmt.Fprintf(&b, "a quiver (container subclass 2) holds arrows and an ammo pouch (subclass 3) holds bullets; %d of the %d quivers and pouches in the data carry a class limit, so none is for one class, and only a hunter, rogue or warrior fires ammo (a bow or crossbow takes arrows, a gun bullets, a thrown weapon neither), so for any other class one has no use. A row's required level (the Heavy Quiver needs 30) is the level to carry it. ", limited, quivers)
	}
	if len(classBags) > 0 {
		var names []string
		for n := range classBags {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Fprintf(&b, "Bags with a class limit in the data: %s; the other special bags carry none. ", strings.Join(names, ", "))
	}
	b.WriteString("The data holds no slot count or ammo speed for any of them. Say so; give none from memory.")
	return b.String()
}
