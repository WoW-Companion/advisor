package route

import (
	"cmp"
	"math"
	"slices"
	"sort"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
)

// Arrival kinds.
const (
	ArriveFlight = "flight"
	ArriveWalk   = "walk"
)

// Arrival is where the player arrives in a zone they are not in.
type Arrival struct {
	Kind  string  `json:"kind"`            // ArriveFlight or ArriveWalk
	NPC   int64   `json:"npc,omitempty"`   // a flight master's NPC id
	Name  string  `json:"name,omitempty"`  // the flight master, or the side the player walks in from
	Place string  `json:"place,omitempty"` // the subzone of a flight master, when known
	Map   int     `json:"map"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Known bool    `json:"known"` // X and Y are a known position
}

// ZonePlan is the one connected route through a zone the player is not in: it starts at Arrival and
// visits Order, the `n` of the zone's stops in Plan.Later.
type ZonePlan struct {
	Map     int     `json:"map"`
	Zone    string  `json:"zone,omitempty"`
	Arrival Arrival `json:"arrival"`
	Order   []int   `json:"order"`
	Travel  string  `json:"travel"` // the route step that goes there
}

// entrance is where the road from one map crosses into a zone.
type entrance struct {
	From string
	Map  int
	X, Y float64
}

// zoneEntrances are the walking entrances to a zone, by the map the player comes from. Positions are
// approximate map percent at the border road. The first entry is used when the player's own map is not
// listed. A zone not listed has no known entrance: its walk arrival has no position.
var zoneEntrances = map[int][]entrance{
	1442: { // Stonetalon Mountains
		{From: "The Barrens", Map: 1413, X: 66, Y: 94},
		{From: "Ashenvale", Map: 1440, X: 60, Y: 5},
	},
}

// placeSource names where a flight master stands.
type placeSource interface {
	flightPlace(npc int64) string
}

// hubs are the crossings between continents: faction, from, to.
var hubs = map[[3]string]string{
	{"Horde", "Eastern Kingdoms", "Kalimdor"}:    "Zeppelin to Orgrimmar",
	{"Horde", "Kalimdor", "Eastern Kingdoms"}:    "Zeppelin to Undercity",
	{"Alliance", "Eastern Kingdoms", "Kalimdor"}: "Boat to Auberdine",
	{"Alliance", "Kalimdor", "Eastern Kingdoms"}: "Boat to Menethil Harbor",
}

func kindRank(k string) int {
	switch k {
	case KindPickup:
		return 0
	case KindObjective:
		return 1
	}
	return 2
}

// zoneRoutes plans one connected route for every zone other than cur that holds later stops. It
// returns the later stops regrouped (the player's zone first, then each zone's stops in route order,
// the zone with the most stops first) and each zone's plan; Order holds indexes into the returned list
// until the caller numbers them.
func zoneRoutes(c character.Character, src source, later []*spot, cur int, curKnown bool, from *Point) (list []*spot, zones []ZonePlan, idx map[int][]int) {
	byMap := map[int][]*spot{}
	var maps []int
	for _, s := range later {
		if s.m == cur {
			list = append(list, s)
			continue
		}
		if _, ok := byMap[s.m]; !ok {
			maps = append(maps, s.m)
		}
		byMap[s.m] = append(byMap[s.m], s)
	}
	sort.SliceStable(maps, func(i, j int) bool {
		a, b := len(byMap[maps[i]]), len(byMap[maps[j]])
		return a > b || (a == b && maps[i] < maps[j])
	})
	idx = map[int][]int{}
	for _, m := range maps {
		spots := byMap[m]
		if isInstance(src, m) {
			for _, s := range spots {
				idx[m] = append(idx[m], len(list))
				list = append(list, s)
			}
			continue
		}
		arr := arrivalFor(c, src, m, spots, from)
		ordered := chainOrder(spots, arr)
		for _, s := range ordered {
			idx[m] = append(idx[m], len(list))
			list = append(list, s)
		}
		zones = append(zones, ZonePlan{Map: m, Zone: src.mapName(m), Arrival: arr, Travel: travelLabel(c, src, m, arr, cur, curKnown)})
	}
	return list, zones, idx
}

func isInstance(src source, m int) bool {
	if is, ok := src.(interface{ isInstance(int) bool }); ok {
		return is.isInstance(m)
	}
	return false
}

// arrivalFor is where the player arrives in zone m: the known flight master nearest the zone's stops,
// else the entrance on the player's side, else the zone alone (no position).
func arrivalFor(c character.Character, src source, m int, spots []*spot, from *Point) Arrival {
	var cx, cy float64
	n := 0
	for _, s := range spots {
		if !s.zoneOnly {
			cx, cy, n = cx+s.x, cy+s.y, n+1
		}
	}
	if n > 0 {
		cx, cy = cx/float64(n), cy/float64(n)
	}
	var best *character.SeenFlight
	fl := c.Seen.FlightsFor(c.Faction)
	for i := range fl {
		f := &fl[i]
		if f.Map != m {
			continue
		}
		if best == nil || (n > 0 && math.Hypot(f.X-cx, f.Y-cy) < math.Hypot(best.X-cx, best.Y-cy)) {
			best = f
		}
	}
	if best != nil {
		a := Arrival{Kind: ArriveFlight, NPC: best.NPC, Name: best.Name, Map: m, X: round1(best.X), Y: round1(best.Y), Known: true}
		if ps, ok := src.(placeSource); ok {
			a.Place = ps.flightPlace(best.NPC)
		}
		return a
	}
	a := Arrival{Kind: ArriveWalk, Map: m}
	if es := zoneEntrances[m]; len(es) > 0 {
		e := es[0]
		for _, x := range es {
			if from != nil && x.Map == from.Map {
				e = x
			}
		}
		a.Name, a.X, a.Y, a.Known = e.From, e.X, e.Y, true
	}
	return a
}

// chainOrder orders one zone's stops as a single path from the arrival point: every quest's pick-up
// before its objectives before its turn-in. Stops with no position come last.
func chainOrder(spots []*spot, arr Arrival) []*spot {
	var placed, rest []*spot // rest: no position
	for _, s := range spots {
		if s.zoneOnly {
			rest = append(rest, s)
		} else {
			placed = append(placed, s)
		}
	}
	prob := &Problem{}
	if arr.Known {
		prob.Start = &[2]float64{arr.X, arr.Y}
	}
	for i, s := range placed {
		var pred []int
		for j, o := range placed {
			if i != j && kindRank(o.kind) < kindRank(s.kind) && sharesQuest(o, s) {
				pred = append(pred, j)
			}
		}
		prob.Nodes = append(prob.Nodes, Node{X: s.x, Y: s.y, Pred: pred})
	}
	var out []*spot
	for _, i := range Solve(prob) {
		out = append(out, placed[i])
	}
	// A stop with no position goes before the first stop that needs it, else last.
	sort.SliceStable(rest, func(i, j int) bool { return kindRank(rest[i].kind) < kindRank(rest[j].kind) })
	for _, s := range rest {
		at := len(out)
		for k, o := range out {
			if kindRank(o.kind) > kindRank(s.kind) && sharesQuest(o, s) {
				at = k
				break
			}
		}
		out = slices.Insert(out, at, s)
	}
	return out
}

func sharesQuest(a, b *spot) bool {
	for _, q := range a.quests {
		if slices.Contains(b.quests, q) {
			return true
		}
	}
	return false
}

// travelLabel is the route step that goes to zone m. A flight is suggested only for a known flight
// master; otherwise the player walks or rides.
func travelLabel(c character.Character, src source, m int, a Arrival, cur int, curKnown bool) string {
	zone := src.mapName(m)
	hub := ""
	if curKnown {
		hub = hubs[[3]string{c.Faction, learn.ContinentOf(src.mapName(cur)), learn.ContinentOf(zone)}]
	}
	if a.Kind == ArriveFlight {
		where := cmp.Or(a.Place, a.Name, zone)
		if a.Place != "" && a.Name != "" {
			where = a.Place + " (" + a.Name + ")"
		}
		if hub != "" {
			return hub + ", then fly to " + where
		}
		return "Fly to " + where + ", " + zone
	}
	walk := "walk or ride to " + zone
	if a.Name != "" {
		walk += " from " + a.Name
	}
	if hub != "" {
		return hub + ", then " + walk
	}
	return "W" + walk[1:]
}
