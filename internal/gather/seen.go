package gather

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// seenKind maps the addon's gathering kind to ours; "" for skinning, which no answer here is about.
func seenKind(k string) string {
	switch k {
	case "herb":
		return Herb
	case "mine":
		return Ore
	case "fish":
		return Fish
	}
	return ""
}

func itemName(ds store.DataStore, id int64) string {
	if ds == nil {
		return ""
	}
	if r, err := ds.Get("items", id); err == nil {
		return str(r["name"])
	}
	return ""
}

// zoneName is the zone of a map the capture knows, else a plain "map N" so the place is still named.
func zoneName(c character.Character, mapID int) string {
	if z := c.ZoneOfMap(mapID); z != "" {
		return z
	}
	return fmt.Sprintf("map %d", mapID)
}

// seenZones groups the rows by map into zones: the most loots first, the character's own map first.
func seenZones(c character.Character, rows []character.SeenGather, nodeLabel func(character.SeenGather) string) []Zone {
	type cell struct {
		cx, cy, n int
	}
	type agg struct {
		zone  Zone
		cells []cell
		n     int
		mapID int
	}
	by := map[int]*agg{}
	var order []int
	for _, r := range rows {
		a := by[r.Map]
		if a == nil {
			a = &agg{zone: Zone{Zone: zoneName(c, r.Map), Seen: true}, mapID: r.Map}
			by[r.Map] = a
			order = append(order, r.Map)
		}
		a.n += r.N
		a.zone.Spawns += r.N
		a.cells = append(a.cells, cell{r.CX, r.CY, r.N})
		if l := nodeLabel(r); l != "" {
			dup := false
			for _, x := range a.zone.Nodes {
				dup = dup || x == l
			}
			if !dup {
				a.zone.Nodes = append(a.zone.Nodes, l)
			}
		}
	}
	here := -1
	if l := c.Location; l != nil && l.MapID != nil {
		here = *l.MapID
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, b := by[order[i]], by[order[j]]
		if (a.mapID == here) != (b.mapID == here) {
			return a.mapID == here
		}
		return a.n > b.n
	})
	var out []Zone
	for _, m := range order {
		a := by[m]
		a.zone.Here = m == here
		sort.SliceStable(a.cells, func(i, j int) bool { return a.cells[i].n > a.cells[j].n })
		for _, cl := range a.cells[:min(2, len(a.cells))] {
			a.zone.Coords = append(a.zone.Coords, [2]float64{float64(cl.cx)*5 + 2.5, float64(cl.cy)*5 + 2.5})
		}
		out = append(out, a.zone)
	}
	return out
}

// seenFind lists the zones where the player gathered an item matching the query, with that item's
// name and kind; empty when the player never has.
func (g *Gatherer) seenFind(c character.Character, query, verb string) ([]Zone, string, string) {
	if c.Seen == nil || query == "" {
		return nil, "", ""
	}
	var ds store.DataStore
	if g != nil {
		ds = g.ds
	}
	kinds := verbKinds(verb)
	q := strings.ToLower(query)
	var rows []character.SeenGather
	name, kind, exact := "", "", false
	for _, r := range c.Seen.Gather {
		k := seenKind(r.Kind)
		if k == "" || !inKinds(k, kinds) {
			continue
		}
		hit := false
		for id := range r.Items {
			n := itemName(ds, id)
			if n == "" || !strings.Contains(strings.ToLower(n), q) {
				continue
			}
			hit = true
			if ex := strings.EqualFold(n, query); name == "" || ex && !exact {
				name, exact = n, ex
			}
		}
		if hit {
			rows, kind = append(rows, r), k
		}
	}
	if len(rows) == 0 {
		return nil, "", ""
	}
	return seenZones(c, rows, func(r character.SeenGather) string {
		if r.Source != "" {
			return r.Source
		}
		return map[string]string{Herb: "herb nodes", Ore: "mining nodes", Fish: "fishing schools"}[seenKind(r.Kind)]
	}), name, kind
}

// seenLevel lists the zones where the player gathered a kind at about this skill.
func seenLevel(c character.Character, kind string, rank int) []Zone {
	if c.Seen == nil {
		return nil
	}
	var rows []character.SeenGather
	for _, r := range c.Seen.Gather {
		if seenKind(r.Kind) == kind && r.SkillMin <= rank && r.SkillMax >= rank-50 {
			rows = append(rows, r)
		}
	}
	return seenZones(c, rows, func(r character.SeenGather) string { return r.Source })
}

// mergeZones puts the seen zones first, then the others in a zone the seen ones do not name, at most MaxZones.
func mergeZones(seen, rest []Zone) []Zone {
	out := append([]Zone{}, seen[:min(MaxZones, len(seen))]...)
	for _, z := range rest {
		if len(out) >= MaxZones {
			break
		}
		dup := false
		for _, s := range seen {
			dup = dup || strings.EqualFold(s.Zone, z.Zone)
		}
		if !dup {
			out = append(out, z)
		}
	}
	return out
}

// AnyQuestie reports whether any zone came from the player's QuestieDB rather than their own sightings.
func AnyQuestie(zs []Zone) bool { return hasQuestie(zs) }

func hasQuestie(zs []Zone) bool {
	for _, z := range zs {
		if !z.Seen {
			return true
		}
	}
	return false
}

// zonesText is "<lead>: <zones> (from your Questie database)." with the Questie mark only when a
// zone came from there; the seen zones carry their own mark.
func zonesText(lead string, zs []Zone) string {
	var parts []string
	for _, z := range zs {
		parts = append(parts, describe(z))
	}
	s := lead + ": " + strings.Join(parts, "; ")
	if hasQuestie(zs) {
		s += " (" + FromQuestie + ")"
	}
	return s + "."
}

// ItemZonesFor is ItemZones with the zones where the player gathered the item themselves first (the
// planner's feeders). It works without Questie.
func (g *Gatherer) ItemZonesFor(c character.Character, item int64, at questie.Where) (kind string, zones []Zone) {
	var rows []character.SeenGather
	if c.Seen != nil {
		for _, r := range c.Seen.Gather {
			if _, ok := r.Items[item]; ok && seenKind(r.Kind) != "" {
				rows, kind = append(rows, r), seenKind(r.Kind)
			}
		}
	}
	seen := seenZones(c, rows, func(r character.SeenGather) string { return r.Source })
	k, rest := g.ItemZones(item, at)
	if kind == "" {
		kind = k
	}
	zones = mergeZones(seen, rest)
	if len(zones) == 0 {
		return "", nil
	}
	return kind, zones
}
