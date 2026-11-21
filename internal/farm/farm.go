// Package farm lists the creatures and objects that drop an item, ranked for a captured character,
// from the dataset's loot_sources and npcs tables. The rules are in docs/farm.md.
package farm

import (
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Spot is one creature or object that drops the item.
type Spot struct {
	Kind     string // creature or object
	ID       int64
	Name     string // empty when the dataset has no name
	MinLevel *int
	MaxLevel *int
	// InRange is true when the level range is known and overlaps the character's level window.
	InRange bool
	// Zone is empty when unknown; ZoneSource then is too.
	Zone       string
	ZoneSource string
	// Chance is 100 * drops / kills over the source's loot windows; nil when kills is zero or
	// unrecorded.
	Chance *float64
	Kills  int64
	Drops  int64
}

func i64(v any) int64 { n, _ := v.(int64); return n }

func str(v any) string { s, _ := v.(string); return s }

func optInt(v any) *int {
	n, ok := v.(int64)
	if !ok {
		return nil
	}
	i := int(n)
	return &i
}

// IsCrafted is true for a loot_sources kind that is a recipe or other crafting source, not a drop.
func IsCrafted(kind string) bool {
	k := strings.ToLower(kind)
	return strings.Contains(k, "craft") || strings.Contains(k, "recipe") || strings.Contains(k, "spell")
}

// Spots returns the creatures and objects that drop itemID, one per source, in-range sources
// first, then by drop chance (unknown last), then by kind and id. Rows of one source are summed.
// Crafted sources are left out (see Crafted); a missing table or no rows gives none.
func Spots(c character.Character, ds store.DataStore, itemID int64) []Spot {
	rows, err := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: itemID, Max: itemID})
	if err != nil {
		return nil
	}
	type key struct {
		kind string
		id   int64
	}
	byKey := map[key]*Spot{}
	var out []*Spot
	for _, row := range rows {
		kind := str(row["source_kind"])
		if kind != "creature" && kind != "object" {
			continue
		}
		k := key{kind, i64(row["source_id"])}
		s := byKey[k]
		if s == nil {
			s = &Spot{Kind: kind, ID: k.id}
			byKey[k] = s
			out = append(out, s)
		}
		s.Kills += i64(row["kills"])
		s.Drops += i64(row["drops"])
	}
	level := c.Level
	res := make([]Spot, 0, len(out))
	for _, s := range out {
		if s.Kills > 0 {
			p := 100 * float64(s.Drops) / float64(s.Kills)
			s.Chance = &p
		}
		if s.Kind == "creature" {
			if n, err := ds.Get("npcs", s.ID); err == nil {
				s.Name = str(n["name"])
				s.MinLevel, s.MaxLevel = optInt(n["min_level"]), optInt(n["max_level"])
				if z := str(n["zone_name"]); z != "" {
					s.Zone = z
					s.ZoneSource = str(n["zone_source"])
					if s.ZoneSource == "" {
						s.ZoneSource = "npcs.zone_name"
					}
				}
			}
		}
		lo, hi := s.MinLevel, s.MaxLevel
		if lo == nil {
			lo = hi
		}
		if hi == nil {
			hi = lo
		}
		s.InRange = level != nil && lo != nil && *hi >= *level-gear.SourceBelow && *lo <= *level+gear.SourceAbove
		res = append(res, *s)
	}
	sort.SliceStable(res, func(i, j int) bool {
		a, b := res[i], res[j]
		if a.InRange != b.InRange {
			return a.InRange
		}
		if (a.Chance != nil) != (b.Chance != nil) {
			return a.Chance != nil
		}
		if a.Chance != nil && *a.Chance != *b.Chance {
			return *a.Chance > *b.Chance
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.ID < b.ID
	})
	return res
}

// Crafted is true when the dataset lists a crafting source for the item, which profession_plan
// covers instead.
func Crafted(ds store.DataStore, itemID int64) bool {
	rows, err := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: itemID, Max: itemID})
	if err != nil {
		return false
	}
	for _, r := range rows {
		if IsCrafted(str(r["source_kind"])) {
			return true
		}
	}
	return false
}

// Resolve turns an item id or name into an item id and name. A name is matched exactly, then by
// prefix, through the dataset's name index; otherwise it returns the candidate names and id 0.
func Resolve(ds store.DataStore, text string) (id int64, name string, candidates []string) {
	text = strings.TrimSpace(text)
	if n, err := strconv.ParseInt(text, 10, 64); err == nil && n > 0 {
		row, err := ds.Get("items", n)
		if err != nil {
			return 0, "", nil
		}
		return n, str(row["name"]), nil
	}
	return names.Resolve(ds, names.Items, text)
}
