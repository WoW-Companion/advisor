package vendors

import (
	"slices"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// seenVendor is one npc's observations merged across the characters: the newest sighting's place and side,
// and every item any of them saw it sell (Ext items, which also cost tokens or items, left out).
type seenVendor struct {
	v     character.SeenVendor
	items map[int64]bool
	zone  string
	sub   string
}

// fill gives the found NPC what the dataset and QuestieDB did not: its name, side and place. Those sources
// win where they have a value; the observed place fills a missing zone or missing coordinates.
func (s *seenVendor) fill(f *Found) {
	f.Observed = true
	if f.Name == "" {
		f.Name = s.v.Name
	}
	if f.Faction == "" && s.v.Faction != "" {
		f.Faction = factionName(strings.ToUpper(s.v.Faction[:1]))
		if f.Faction == "" {
			f.Faction = s.v.Faction
		}
	}
	if f.Zone == "" {
		f.Zone, f.Subzone = s.zone, s.sub
	}
	if !f.HasXY && s.v.Map != 0 && (s.v.X != 0 || s.v.Y != 0) {
		f.X, f.Y, f.HasXY = s.v.X, s.v.Y, true
	}
}

// mapNames is a UI map's zone and subzone name from QuestieDB's areas; empty when it has none.
func mapNames(qx *questie.Index, uiMap int) (zone, sub string) {
	if qx == nil || uiMap == 0 {
		return "", ""
	}
	best := -1
	for id, a := range qx.Areas {
		if a.UIMap != uiMap || a.Zone == "" {
			continue
		}
		if best < 0 || a.Sub == "" && qx.Areas[best].Sub != "" || (a.Sub == "") == (qx.Areas[best].Sub == "") && id < best {
			best = id
		}
	}
	if best < 0 {
		return "", ""
	}
	a := qx.Areas[best]
	return a.Zone, a.Sub
}

// observedVendors merges the observed vendors by npc id, adds those that answer the keywords (their items by
// name or profession reagent, or their name) to the candidates and records the item names they sold.
func observedVendors(ds store.DataStore, qx *questie.Index, terms []string, q Query, cand map[int64]*Found, get func(int64) *Found, sold map[int64]map[string]bool) map[int64]*seenVendor {
	if len(q.Observed) == 0 {
		return nil
	}
	out := map[int64]*seenVendor{}
	for _, v := range q.Observed {
		if v.NPC == 0 {
			continue
		}
		s := out[v.NPC]
		if s == nil {
			s = &seenVendor{items: map[int64]bool{}}
			out[v.NPC] = s
		}
		if s.v.NPC == 0 || v.T >= s.v.T {
			s.v = v
		}
		for _, it := range v.Items {
			if !it.Ext {
				s.items[it.ItemID] = true
			}
		}
	}
	reagent := map[string]map[int64]bool{}
	for _, t := range terms {
		if line := professionWords[t]; line != "" && ds != nil {
			reagent[t] = reagentItems(ds, line)
		}
	}
	itemName := map[int64]string{}
	nameOf := func(id int64) string {
		n, ok := itemName[id]
		if !ok && ds != nil {
			if it, err := ds.Get("items", id); err == nil {
				n = str(it["name"])
			}
			itemName[id] = n
		}
		return n
	}
	for id, s := range out {
		s.zone, s.sub = mapNames(qx, s.v.Map)
		hit := len(terms) == 0 && q.Zone != ""
		ids := make([]int64, 0, len(s.items))
		for it := range s.items {
			ids = append(ids, it)
		}
		slices.Sort(ids)
		var f *Found
		match := func(t string) {
			if f == nil {
				f = get(id)
			}
			f.matched[t] = true
		}
		name := strings.ToLower(s.v.Name)
		for _, t := range terms {
			if strings.Contains(name, t) {
				match(t)
			}
			for _, it := range ids {
				n := nameOf(it)
				if reagent[t][it] || n != "" && strings.Contains(strings.ToLower(n), t) {
					match(t)
					if n != "" {
						if sold[id] == nil {
							sold[id] = map[string]bool{}
						}
						sold[id][n] = true
					}
				}
			}
		}
		if f == nil && hit {
			f = get(id)
		}
		if f == nil {
			f = cand[id]
		}
		if f != nil {
			if sold[id] == nil {
				sold[id] = map[string]bool{}
			}
			for _, it := range ids {
				if n := nameOf(it); n != "" && len(sold[id]) < MaxSells {
					sold[id][n] = true
				}
			}
		}
	}
	return out
}
