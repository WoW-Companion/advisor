package professions

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Coverage says what the advisor has for one profession: the recipes the character's own window listed,
// and the dataset's recipes, trainers, vendors of recipe items and recipe sources.
type Coverage struct {
	Profession string
	Captured   int // recipes in the capture; 0 when the window was never read
	Recipes    int // recipes the dataset lists
	Trainers   int
	Vendors    int            // distinct vendors that sell an item teaching one of the recipes
	Sources    map[string]int // recipes by where they are learned from
}

// craftWindowProfessions are the professions whose recipes sit in the craft window, by class, besides
// Enchanting, which every class may have.
var craftWindowProfessions = map[string][]string{"rogue": {"Poisons"}, "hunter": {"Beast Training"}}

// CoverageOf lists a row for each of the character's professions, each recipe window professions the
// class has (Poisons for a rogue) and each profession with captured recipes. A nil store leaves the
// dataset counts at zero.
func CoverageOf(c character.Character, ds store.DataStore) []Coverage {
	names := []string{}
	seen := map[string]bool{}
	add := func(n string) {
		if k := strings.ToLower(n); n != "" && !seen[k] {
			seen[k] = true
			names = append(names, n)
		}
	}
	for _, p := range c.Professions {
		add(p.Name)
	}
	for _, n := range craftWindowProfessions[strings.ToLower(c.Class)] {
		add(n)
	}
	var teach map[int64][]int64 // recipe spell -> items that teach it
	var offered map[int64][]int64
	if ds != nil {
		teach, offered = teachingItems(ds)
	}
	var out []Coverage
	for _, n := range names {
		cv := Coverage{Profession: n, Captured: len(c.Recipes[strings.ToLower(n)].Entries), Sources: map[string]int{}}
		if ds != nil {
			vend := map[int64]bool{}
			if rows, err := ds.Filter("recipes", store.Range{Column: "skill_line_name", Min: n, Max: n}); err == nil {
				for _, r := range rows {
					cv.Recipes++
					from, _ := r["learned_from"].(string)
					if from == "" {
						from = "unknown"
					}
					cv.Sources[from]++
					for _, it := range teach[asI64(r["spell_id"])] {
						for _, npc := range offered[it] {
							vend[npc] = true
						}
					}
				}
			}
			cv.Vendors = len(vend)
			if rows, err := ds.Filter("trainers", store.Range{Column: "profession", Min: n, Max: n}); err == nil {
				cv.Trainers = len(rows)
			}
		}
		out = append(out, cv)
	}
	return out
}

// teachingItems maps a recipe spell to the items that teach it, and an item to the vendors that sell it.
func teachingItems(ds store.DataStore) (teach, offered map[int64][]int64) {
	teach, offered = map[int64][]int64{}, map[int64][]int64{}
	rows, err := ds.Filter("recipe_items", store.Range{Column: "teaches_spell_id", Min: int64(1)})
	if err != nil {
		return
	}
	for _, r := range rows {
		teach[asI64(r["teaches_spell_id"])] = append(teach[asI64(r["teaches_spell_id"])], asI64(r["item_id"]))
	}
	if vs, err := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: int64(1)}); err == nil {
		for _, v := range vs {
			offered[asI64(v["item_id"])] = append(offered[asI64(v["item_id"])], asI64(v["npc_id"]))
		}
	}
	return
}

// Line is one report line: "Poisons: recipes NOT captured (open the window); dataset: 12 recipes, 1 trainer ...".
func (cv Coverage) Line() string {
	cap := "recipes NOT captured (open the window once)"
	if cv.Captured > 0 {
		cap = fmt.Sprintf("%d recipes captured", cv.Captured)
	}
	var src []string
	for k, n := range cv.Sources {
		src = append(src, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(src)
	s := fmt.Sprintf("%s: %s; dataset: %d recipes, %d trainers, %d recipe vendors", cv.Profession, cap, cv.Recipes, cv.Trainers, cv.Vendors)
	if len(src) > 0 {
		s += "; sources: " + strings.Join(src, ", ")
	}
	if cv.Recipes == 0 {
		s += "; GAP: the dataset lists no recipes"
	} else if cv.Trainers == 0 {
		s += "; GAP: no trainer in the dataset"
	}
	return s
}
