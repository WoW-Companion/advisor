package route

import (
	"cmp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// hearthMinutes is the cost of the hearthstone leg: the cast and the loading screen.
const hearthMinutes = 1

// bindSource is a source that can place a bind name the graph does not know: a town or flight point by
// name, or the zone of an innkeeper.
type bindSource interface {
	// bindZone is the zone a bind name stands in; ok is false when no place matches, or the name is in
	// more than one zone (a wrong teleport is worse than none).
	bindZone(name string) (zone string, ok bool)
}

// hearthState is the hearthstone as a route resource for one plan.
type hearthState struct {
	name string // the bind as captured
	zone string // the zone it is in (display name)
	wait int    // seconds until the stone is ready, at the start of the plan
}

// placeBind resolves a bind name to a zone: (a) a city or zone of the travel graph, (b) a flight point
// or town the place data knows, (c) an innkeeper's subzone, zone or subname. ok is false when none matches.
func placeBind(g *graph, src source, name string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(name))
	if k == "" {
		return "", false
	}
	if cities[k] {
		return g.names[k], true
	}
	if cities[k+" city"] {
		return g.names[k+" city"], true
	}
	if z := g.names[k]; z != "" {
		return z, true
	}
	if bs, ok := src.(bindSource); ok {
		return bs.bindZone(name)
	}
	return "", false
}

// bindZone places a bind name by the data's flight masters (b) and then its innkeepers (c).
func (s *questieSource) bindZone(name string) (string, bool) {
	want := strings.TrimSpace(name)
	ids := make([]int, 0, 64)
	for id, e := range s.qx.Npcs {
		if e.IsFlightMaster() || e.IsInnkeeper() {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	pick := func(match func(id int) (int, bool)) (string, bool) {
		zones := map[string]bool{}
		for _, id := range ids {
			if m, ok := match(id); ok {
				zones[s.mapName(m)] = true
			}
		}
		delete(zones, unknownPlace)
		if len(zones) != 1 {
			return "", false
		}
		for z := range zones {
			return z, true
		}
		return "", false
	}
	// (b) a flight point's town: the subzone a flight master stands in.
	if z, _ := pick(func(id int) (int, bool) {
		e := s.qx.Npcs[id]
		if !e.IsFlightMaster() {
			return 0, false
		}
		for _, sp := range e.Spawns {
			if a, ok := s.qx.Areas[sp.Area]; ok && a.UIMap != 0 && strings.EqualFold(a.Sub, want) {
				return a.UIMap, true
			}
		}
		return 0, false
	}); z != "" {
		return z, true
	}
	// (c) an innkeeper whose spawn subzone or zone, or whose subname, is the bind.
	return pick(func(id int) (int, bool) {
		e := s.qx.Npcs[id]
		if !e.IsInnkeeper() {
			return 0, false
		}
		for _, sp := range e.Spawns {
			a, ok := s.qx.Areas[sp.Area]
			if !ok || a.UIMap == 0 {
				continue
			}
			if strings.EqualFold(a.Sub, want) || strings.EqualFold(a.Zone, want) || strings.EqualFold(e.Sub, want) {
				return a.UIMap, true
			}
		}
		return 0, false
	})
}

// hearthFor is the hearthstone for the plan, when the character has one that is bound somewhere the
// advisor can place. The wait is the capture's, aged to now. A usable stone with an unplaceable bind
// sets the plan's basis note.
func hearthFor(p *Plan, c character.Character, g *graph, src source, now int64) *hearthState {
	wait, usable := c.HearthWait(cmp.Or(now, derefInt64(c.CapturedAt)))
	if !usable {
		return nil
	}
	zone, ok := placeBind(g, src, c.Hearth)
	if !ok {
		p.Basis.Hearth = "the hearthstone is bound to " + strings.TrimSpace(c.Hearth) + ", which the advisor could not place"
		return nil
	}
	return &hearthState{name: strings.TrimSpace(c.Hearth), zone: zone, wait: wait}
}

func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// noHearth is the minutes of a hearth leg that cannot be taken.
const noHearth = 1 << 30

// tripCost is the ordering cost of a route: travel minutes plus the value-weighted time before each trip.
// leg hp (-1: none) is taken with the hearthstone, costing hb[trip] minutes.
func tripCost(ord []int, d0 []int, d [][]int, w []float64, hp int, hb []int) float64 {
	t, total, prev := 0, 0.0, -1
	for pos, i := range ord {
		leg := d0[i]
		if prev >= 0 {
			leg = d[prev][i]
		}
		if pos == hp {
			leg = hb[i]
		}
		t += leg
		total += float64(leg) + float64(t)*w[i]
		prev = i
	}
	return total
}

// bestHearthLeg is the position in ord where spending the hearthstone lowers the cost the most (-1: none)
// and the cost with it. Only a leg that starts once the stone is ready (the minutes before it, in seconds,
// reach wait) and that the hearth makes strictly shorter qualifies.
func bestHearthLeg(ord []int, d0 []int, d [][]int, w []float64, wait int, hb []int) (int, float64) {
	best, bestCost := -1, tripCost(ord, d0, d, w, -1, hb)
	run, prev := 0, -1
	for pos, i := range ord {
		leg := d0[i]
		if prev >= 0 {
			leg = d[prev][i]
		}
		if run*60 >= wait && hb[i] < leg {
			if c := tripCost(ord, d0, d, w, pos, hb); c < bestCost-1e-9 {
				best, bestCost = pos, c
			}
		}
		run += leg
		prev = i
	}
	return best, bestCost
}

// orderWithHearth orders the trips as orderTrips does, and also tries the orders that make the hearth leg
// worth taking (one candidate per trip the stone could bring the player to). It returns the cheapest order
// and the position of the hearth leg in it (-1: not used); the plain order wins a tie.
func orderWithHearth(d0 []int, d [][]int, w []float64, wait int, hb []int) ([]int, int) {
	ord := orderTrips(d0, d, w)
	hp, cost := bestHearthLeg(ord, d0, d, w, wait, hb)
	n := len(d0)
	for j := 0; j < n; j++ {
		if hb[j] >= noHearth {
			continue
		}
		d0j, dj := append([]int(nil), d0...), make([][]int, n)
		d0j[j] = min(d0j[j], hb[j])
		for a := range d {
			dj[a] = append([]int(nil), d[a]...)
			if a != j {
				dj[a][j] = min(dj[a][j], hb[j])
			}
		}
		cand := orderTrips(d0j, dj, w)
		if cp, c := bestHearthLeg(cand, d0, d, w, wait, hb); cp >= 0 && c < cost-1e-9 {
			ord, hp, cost = cand, cp, c
		}
	}
	return ord, hp
}

// hearthText is the wording of a hearth leg: "Hearth to Tarren Mill", then the rest of the hop.
func hearthText(name, rest string) string {
	if rest == "" {
		return "Hearth to " + name
	}
	rest = lowerFirst(rest)
	if strings.HasPrefix(rest, "Walk or ride") {
		rest = "w" + rest[1:]
	}
	return "Hearth to " + name + ", then " + rest
}
