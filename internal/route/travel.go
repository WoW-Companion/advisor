package route

import (
	"cmp"
	"container/heap"
	"math"
	"slices"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
)

// The travel graph: zones are nodes; the edges are zone borders (on foot or by mount), the faction's
// zeppelins and boats, and the flight paths the character has opened. Every edge has an estimated time in
// minutes. The data is written from public game facts, by hand.

// Minutes of each kind of edge.
const (
	walkMinutes   = 8  // across one zone border
	cityMinutes   = 2  // between a city and the zone around it
	flightMinutes = 5  // between two known flight points on one continent
	unknownLeg    = 15 // two zones the graph cannot join
	unknownCross  = 25 // two zones on different continents the graph cannot join
)

type edgeKind int

const (
	edgeWalk edgeKind = iota
	edgeFlight
	edgeTransport
)

type edge struct {
	to      string // lower-case zone name
	minutes int
	kind    edgeKind
	via     string // "zeppelin" or "boat" for a transport
	label   string // a transport's wording, "Zeppelin from Undercity to Grom'gol"
	toName  string // the display name of the zone reached
}

type transport struct {
	faction  string // "H", "A" or "" for both
	from, to string // zone names
	via      string
	label    string // the leg both ways is "<Via> from <A> to <B>"; this is the forward one
	back     string
	minutes  int
}

// transports are the zeppelin, boat and tram routes between zones.
var transports = []transport{
	{"H", "Undercity", "Orgrimmar", "zeppelin", "Zeppelin from Undercity to Orgrimmar", "Zeppelin from Orgrimmar to Undercity", 10},
	{"H", "Undercity", "Stranglethorn Vale", "zeppelin", "Zeppelin from Undercity to Grom'gol", "Zeppelin from Grom'gol to Undercity", 10},
	{"H", "Orgrimmar", "Stranglethorn Vale", "zeppelin", "Zeppelin from Orgrimmar to Grom'gol", "Zeppelin from Grom'gol to Orgrimmar", 10},
	{"", "The Barrens", "Stranglethorn Vale", "boat", "Boat from Ratchet to Booty Bay", "Boat from Booty Bay to Ratchet", 8},
	{"A", "Wetlands", "Darkshore", "boat", "Boat from Menethil Harbor to Auberdine", "Boat from Auberdine to Menethil Harbor", 8},
	{"A", "Wetlands", "Dustwallow Marsh", "boat", "Boat from Menethil Harbor to Theramore Isle", "Boat from Theramore Isle to Menethil Harbor", 8},
	{"A", "Darkshore", "Teldrassil", "boat", "Boat from Auberdine to Rut'theran Village", "Boat from Rut'theran Village to Auberdine", 8},
	{"A", "Ironforge", "Stormwind City", "tram", "Deeprun Tram from Ironforge to Stormwind", "Deeprun Tram from Stormwind to Ironforge", 5},
}

// borders are pairs of zones that touch, or a city and the zone it stands in (city pairs are marked by
// the first name being a city).
var borders = [][2]string{
	// Eastern Kingdoms
	{"Undercity", "Tirisfal Glades"}, {"Tirisfal Glades", "Silverpine Forest"}, {"Tirisfal Glades", "Western Plaguelands"},
	{"Silverpine Forest", "Hillsbrad Foothills"}, {"Hillsbrad Foothills", "Arathi Highlands"}, {"Hillsbrad Foothills", "Alterac Mountains"},
	{"Arathi Highlands", "Wetlands"}, {"Arathi Highlands", "The Hinterlands"}, {"The Hinterlands", "Western Plaguelands"},
	{"Western Plaguelands", "Eastern Plaguelands"}, {"Wetlands", "Dun Morogh"}, {"Wetlands", "Loch Modan"},
	{"Dun Morogh", "Loch Modan"}, {"Ironforge", "Dun Morogh"}, {"Loch Modan", "Badlands"}, {"Badlands", "Searing Gorge"},
	{"Searing Gorge", "Burning Steppes"}, {"Burning Steppes", "Redridge Mountains"}, {"Redridge Mountains", "Elwynn Forest"},
	{"Redridge Mountains", "Duskwood"}, {"Redridge Mountains", "Swamp of Sorrows"}, {"Elwynn Forest", "Westfall"},
	{"Elwynn Forest", "Duskwood"}, {"Stormwind City", "Elwynn Forest"}, {"Westfall", "Duskwood"},
	{"Duskwood", "Stranglethorn Vale"}, {"Duskwood", "Deadwind Pass"}, {"Deadwind Pass", "Swamp of Sorrows"},
	{"Swamp of Sorrows", "Blasted Lands"}, {"Eversong Woods", "Ghostlands"}, {"Silvermoon City", "Eversong Woods"},
	{"Ghostlands", "Eastern Plaguelands"},
	// Kalimdor
	{"Orgrimmar", "Durotar"}, {"Durotar", "The Barrens"}, {"The Barrens", "Mulgore"}, {"The Barrens", "Stonetalon Mountains"},
	{"The Barrens", "Ashenvale"}, {"The Barrens", "Dustwallow Marsh"}, {"The Barrens", "Thousand Needles"},
	{"Thunder Bluff", "Mulgore"}, {"Mulgore", "Thousand Needles"}, {"Stonetalon Mountains", "Ashenvale"},
	{"Stonetalon Mountains", "Desolace"}, {"Ashenvale", "Desolace"}, {"Ashenvale", "Azshara"}, {"Ashenvale", "Darkshore"},
	{"Ashenvale", "Felwood"}, {"Darnassus", "Teldrassil"}, {"Desolace", "Feralas"}, {"Feralas", "Thousand Needles"},
	{"Thousand Needles", "Tanaris"}, {"Tanaris", "Un'Goro Crater"}, {"Un'Goro Crater", "Silithus"}, {"Felwood", "Winterspring"},
	{"Felwood", "Moonglade"}, {"Winterspring", "Moonglade"}, {"Tanaris", "Dustwallow Marsh"},
}

var cities = map[string]bool{"undercity": true, "orgrimmar": true, "thunder bluff": true, "darnassus": true,
	"ironforge": true, "stormwind city": true, "silvermoon city": true}

// graph is the travel graph for one character at one moment.
type graph struct {
	adj   map[string][]edge
	names map[string]string // lower-case zone name -> display name
}

func (g *graph) add(a, b string, e edge, back edge) {
	ka, kb := strings.ToLower(a), strings.ToLower(b)
	g.names[ka], g.names[kb] = a, b
	e.to, e.toName = kb, b
	back.to, back.toName = ka, a
	g.adj[ka] = append(g.adj[ka], e)
	g.adj[kb] = append(g.adj[kb], back)
}

// newGraph builds the graph for the character's side and the flight paths they know.
func newGraph(c character.Character) *graph {
	g := &graph{adj: map[string][]edge{}, names: map[string]string{}}
	for _, b := range borders {
		m := walkMinutes
		if cities[strings.ToLower(b[0])] {
			m = cityMinutes
		}
		g.add(b[0], b[1], edge{minutes: m, kind: edgeWalk}, edge{minutes: m, kind: edgeWalk})
	}
	side := sideLetter(c)
	for _, t := range transports {
		if t.faction != "" && side != "" && t.faction != side {
			continue
		}
		g.add(t.from, t.to,
			edge{minutes: t.minutes, kind: edgeTransport, via: t.via, label: t.label},
			edge{minutes: t.minutes, kind: edgeTransport, via: t.via, label: t.back})
	}
	return g
}

func sideLetter(c character.Character) string {
	switch playerSide(c) {
	case "H", "A":
		return playerSide(c)
	}
	return ""
}

// leg is one step of a path.
type leg struct {
	from, to string // display names of the zones
	e        edge
}

type pathItem struct {
	node string
	cost int
}
type pathHeap []pathItem

func (h pathHeap) Len() int           { return len(h) }
func (h pathHeap) Less(i, j int) bool { return h[i].cost < h[j].cost }
func (h pathHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *pathHeap) Push(x any)        { *h = append(*h, x.(pathItem)) }
func (h *pathHeap) Pop() any          { o := *h; x := o[len(o)-1]; *h = o[:len(o)-1]; return x }

// route is the fastest path between two zones: its legs and minutes. ok is false when the graph cannot
// join them.
func (g *graph) route(from, to string, flights map[string][]flightPoint) (legs []leg, minutes int, ok bool) {
	src, dst := strings.ToLower(strings.TrimSpace(from)), strings.ToLower(strings.TrimSpace(to))
	if src == "" || dst == "" {
		return nil, 0, false
	}
	if src == dst {
		return nil, 0, true
	}
	if _, known := g.adj[src]; !known && len(flights[src]) == 0 {
		return nil, 0, false
	}
	// Flight edges: to any zone with an opened flight point on the same continent; quicker from a zone
	// that has one too, since the player is then already at a flight master.
	neighbours := func(n string) []edge {
		out := g.adj[n]
		if learn.ContinentOf(n) == "" {
			return out
		}
		out = append([]edge(nil), out...)
		for z, pts := range flights {
			if z == n || len(pts) == 0 || learn.ContinentOf(z) != learn.ContinentOf(n) {
				continue
			}
			m := flightMinutes
			if len(flights[n]) == 0 {
				m += 2
			}
			out = append(out, edge{to: z, toName: pts[0].zone, minutes: m, kind: edgeFlight})
		}
		return out
	}
	dist := map[string]int{src: 0}
	prev := map[string]leg{}
	h := &pathHeap{{src, 0}}
	for h.Len() > 0 {
		it := heap.Pop(h).(pathItem)
		if it.cost > dist[it.node] {
			continue
		}
		if it.node == dst {
			break
		}
		for _, e := range neighbours(it.node) {
			d := it.cost + e.minutes
			if old, seen := dist[e.to]; seen && old <= d {
				continue
			}
			dist[e.to] = d
			prev[e.to] = leg{from: it.node, to: e.to, e: e}
			heap.Push(h, pathItem{e.to, d})
		}
	}
	d, reached := dist[dst]
	if !reached {
		return nil, 0, false
	}
	for n := dst; n != src; n = prev[n].from {
		l := prev[n]
		l.from, l.to = g.display(l.from, flights), g.display(l.to, flights)
		legs = append([]leg{l}, legs...)
	}
	return legs, d, true
}

func (g *graph) display(n string, flights map[string][]flightPoint) string {
	if s := g.names[n]; s != "" {
		return s
	}
	if pts := flights[n]; len(pts) > 0 {
		return pts[0].zone
	}
	return n
}

// flightPoint is a flight master the character has opened.
type flightPoint struct {
	zone  string
	label string // "Tarren Mill", or the flight master's name when the place is not known
	full  string // "Tarren Mill (Ghan Redhoof)": the place with the flight master, for a flight from nowhere
	x, y  float64
}

// flightPoints groups the character's known flight points by lower-case zone name.
func flightPoints(c character.Character, src source) map[string][]flightPoint {
	out := map[string][]flightPoint{}
	fl := c.Seen.FlightsFor(c.Faction)
	for _, f := range fl {
		zone := src.mapName(f.Map)
		if zone == "" {
			continue
		}
		place := ""
		if ps, ok := src.(placeSource); ok {
			place = ps.flightPlace(f.NPC)
		}
		label := cmp.Or(place, f.Name, zone)
		full := label
		if place != "" && f.Name != "" {
			full = place + " (" + f.Name + ")"
		}
		k := strings.ToLower(zone)
		out[k] = append(out[k], flightPoint{zone: zone, label: label, full: full, x: f.X, y: f.Y})
	}
	return out
}

// nearestPoint is the flight point of the zone nearest (x, y); the first one when no position is given.
func nearestPoint(pts []flightPoint, near *[2]float64) flightPoint {
	best := pts[0]
	if near == nil {
		return best
	}
	for _, p := range pts {
		if math.Hypot(p.x-near[0], p.y-near[1]) < math.Hypot(best.x-near[0], best.y-near[1]) {
			best = p
		}
	}
	return best
}

// legsText is the plain wording of a path: "Zeppelin from Undercity to Grom'gol, then ride to Stranglethorn
// Vale", "Fly Tarren Mill → Hammerfall". Walking legs in a row are one "walk or ride" leg, and a path that is
// only walking says so. near is where the player stands in the first zone, when known; arrive is where the
// last flight should land (the flight point nearest the work).
func legsText(legs []leg, flights map[string][]flightPoint, near, arrive *[2]float64) (text, via string) {
	var parts []string
	i := 0
	for i < len(legs) {
		l := legs[i]
		switch l.e.kind {
		case edgeTransport:
			parts = append(parts, l.e.label)
			if via == "" {
				via = l.e.via
				if via == "tram" {
					via = "walk"
				}
			}
			// The port is somewhere in the zone: the last transport ends with a ride.
			if i == len(legs)-1 {
				parts = append(parts, "ride to "+l.to)
			}
			i++
		case edgeFlight:
			to := nearestPoint(flights[strings.ToLower(l.to)], arrive)
			if i != len(legs)-1 {
				to = nearestPoint(flights[strings.ToLower(l.to)], nil)
			}
			if pts := flights[strings.ToLower(l.from)]; len(pts) > 0 {
				parts = append(parts, "Fly "+nearestPoint(pts, near).label+" → "+to.label)
			} else {
				parts = append(parts, "Fly to "+to.full+", "+l.to)
			}
			if via == "" {
				via = "flight"
			}
			i++
		default:
			j := i
			for j < len(legs) && legs[j].e.kind == edgeWalk {
				j++
			}
			parts = append(parts, "walk or ride to "+legs[j-1].to)
			if via == "" {
				via = "walk"
			}
			i = j
		}
	}
	if len(parts) == 0 {
		return "", ""
	}
	// Only "walk or ride" parts of the first kind keep the capital; after the first, "then".
	for k := range parts {
		if k > 0 {
			parts[k] = "then " + lowerFirst(parts[k])
		}
	}
	text = strings.Join(parts, ", ")
	if parts[0] != "" && !strings.HasPrefix(parts[0], "Fly ") {
		text = strings.ToUpper(text[:1]) + text[1:]
	}
	return text, via
}

func lowerFirst(s string) string {
	if strings.HasPrefix(s, "Fly ") || s == "" {
		return strings.ToLower(s[:1]) + s[1:]
	}
	// Keep proper names: only the leading transport word is lower-cased. The Deeprun Tram is a name, so it stays whole.
	for _, w := range []string{"Zeppelin ", "Boat "} {
		if strings.HasPrefix(s, w) {
			return strings.ToLower(w[:1]) + w[1:] + s[len(w):]
		}
	}
	return s
}

// hop is the estimated minutes and wording between two trip zones.
type hop struct {
	minutes int
	text    string
	via     string
	ok      bool
}

func (g *graph) hop(from, to string, flights map[string][]flightPoint, near, arrive *[2]float64) hop {
	legs, m, ok := g.route(from, to, flights)
	if !ok {
		cf, ct := learn.ContinentOf(from), learn.ContinentOf(to)
		m = unknownLeg
		if len(flights[strings.ToLower(to)]) > 0 {
			m = flightMinutes + 2
		}
		if cf != "" && ct != "" && cf != ct {
			m = unknownCross
		}
		return hop{minutes: m}
	}
	text, via := legsText(legs, flights, near, arrive)
	return hop{minutes: m, text: text, via: via, ok: true}
}

// valueWeight is how much a minute of waiting costs for a trip: each ready turn-in and each 2,000 XP add to it.
func valueWeight(ready, xp int) float64 {
	return float64(ready)*0.5 + float64(xp)/2000
}

// orderTrips orders trips as a route from start: the cost is the total travel time, plus, for every trip,
// the time spent before reaching it weighted by its value, so a very valuable trip can go first. It starts
// from nearest-neighbour (nearest first, from every start) and improves with 2-opt; trips are 8 to 10,
// so this is cheap. d[i][j] is the minutes between trips, d0[i] from the start.
func orderTrips(d0 []int, d [][]int, w []float64) []int {
	n := len(d0)
	cost := func(ord []int) float64 {
		t, total, prev := 0, 0.0, -1
		for _, i := range ord {
			leg := d0[i]
			if prev >= 0 {
				leg = d[prev][i]
			}
			t += leg
			total += float64(leg) + float64(t)*w[i]
			prev = i
		}
		return total
	}
	var best []int
	bestCost := math.Inf(1)
	for first := 0; first < n; first++ {
		ord := []int{first}
		used := map[int]bool{first: true}
		for len(ord) < n {
			last, pick := ord[len(ord)-1], -1
			for j := 0; j < n; j++ {
				if used[j] {
					continue
				}
				if pick < 0 || d[last][j] < d[last][pick] {
					pick = j
				}
			}
			ord = append(ord, pick)
			used[pick] = true
		}
		for improved := true; improved; {
			improved = false
			for i := 0; i < n-1; i++ {
				for j := i + 1; j < n; j++ {
					cand := append([]int(nil), ord...)
					for a, b := i, j; a < b; a, b = a+1, b-1 {
						cand[a], cand[b] = cand[b], cand[a]
					}
					if c := cost(cand); c < cost(ord)-1e-9 {
						ord, improved = cand, true
					}
				}
			}
		}
		if c := cost(ord); c < bestCost-1e-9 {
			best, bestCost = ord, c
		}
	}
	return best
}

// orderAndRoute orders the trips as a travel route from the run's exit (in place), then words every trip's
// travel from the one before it and sets its minutes. A pair of zones the graph cannot join keeps the older
// wording and a flat estimate.
func orderAndRoute(p *Plan, c character.Character, src source, trips []*Trip, arrivals map[int]Arrival, legOf map[int]Stop, in sharedInput) {
	if len(trips) == 0 {
		return
	}
	slices.SortStableFunc(trips, func(a, b *Trip) int { return cmp.Compare(a.Map, b.Map) })
	g := newGraph(c)
	flights := flightPoints(c, src)
	start := src.mapName(in.cur)
	var near *[2]float64
	if p.Start != nil && p.Start.Map == in.cur {
		near = &[2]float64{p.Start.X, p.Start.Y}
	}
	arriveAt := func(t *Trip) *[2]float64 {
		if a := arrivals[t.Map]; a.Known {
			return &[2]float64{a.X, a.Y}
		}
		return nil
	}
	zone := func(t *Trip) string { return cmp.Or(t.Zone, src.mapName(t.Map)) }
	n := len(trips)
	d0 := make([]int, n)
	d := make([][]int, n)
	w := make([]float64, n)
	for i, t := range trips {
		w[i] = valueWeight(t.Ready, t.XP) + 0.05*float64(t.Todo)
		d0[i] = tripMinutes(g, start, t, in.cur, flights, near, arriveAt(t))
		d[i] = make([]int, n)
		for j, u := range trips {
			switch {
			case i == j:
			case t.Map == u.Map:
			default:
				d[i][j] = g.hop(zone(t), zone(u), flights, nil, arriveAt(u)).minutes
			}
		}
	}
	ord := orderTrips(d0, d, w)
	sorted := make([]*Trip, 0, n)
	for _, i := range ord {
		sorted = append(sorted, trips[i])
	}
	copy(trips, sorted)
	prev := start
	for i, t := range trips {
		from, fromNear := prev, near
		if i > 0 {
			fromNear = nil
		}
		h := g.hop(from, zone(t), flights, fromNear, arriveAt(t))
		if t.Map == in.cur {
			t.ETA = 0
		} else {
			t.ETA = h.minutes
			if h.ok && h.text != "" {
				// A ready turn-in's leg keeps what it hands in: "<route>: turn in Quest 1, Quest 2".
				suffix := ""
				if l, ok := legOf[t.Map]; ok {
					if _, after, found := strings.Cut(l.Label, ": "); found {
						suffix = ": " + after
					}
				}
				t.Travel, t.via = h.text+suffix, h.via
			}
		}
		prev = zone(t)
	}
}

// tripMinutes is the minutes from the start zone to a trip, 0 for a trip in the run's own map.
func tripMinutes(g *graph, start string, t *Trip, cur int, flights map[string][]flightPoint, near, arrive *[2]float64) int {
	if t.Map == cur {
		return 0
	}
	return g.hop(start, cmp.Or(t.Zone, start), flights, near, arrive).minutes
}
