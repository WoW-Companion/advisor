package route

import (
	"cmp"
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// Limits.
const (
	MaxSteps    = 20  // stops on the route; the rest go to "later"
	MaxLater    = 30  // stops listed in "later"
	LevelMargin = 1   // a quest above the player's level + this goes to "later"
	MergeRadius = 3.0 // objective stops this close (map percent) on one map are one stop
	ClusterSize = 8.0 // spawns this close (map percent) are one cluster
	pinRadius   = 0.05
)

// Stop kinds.
const (
	KindPickup    = "pickup"
	KindObjective = "objective"
	KindTurnIn    = "turnin"
	KindTravel    = "travel"
)

// Stop is one place on the route.
type Stop struct {
	N       int     `json:"n"`
	Kind    string  `json:"kind"`
	QuestID int     `json:"quest_id,omitempty"`
	Quest   string  `json:"quest,omitempty"`
	Label   string  `json:"label"`
	Map     int     `json:"map"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Done    bool    `json:"done"`
}

// FindStop is an objective the planner could not place on the map: something to find, with no x or y.
type FindStop struct {
	N       int    `json:"n"`
	QuestID int    `json:"quest_id,omitempty"`
	Quest   string `json:"quest,omitempty"`
	Label   string `json:"label"`
}

// Point is a place on a map, in map percent.
type Point struct {
	Map int     `json:"map"`
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
}

// Plan is the ordered route for one character.
type Plan struct {
	Character string     `json:"character"`
	Zone      string     `json:"zone,omitempty"`
	Start     *Point     `json:"start,omitempty"`
	Steps     []Stop     `json:"steps"`
	Later     []Stop     `json:"later,omitempty"`
	Find      []FindStop `json:"find,omitempty"` // objectives with no known location; not on the route
	Distance  float64    `json:"distance"`       // route length in map percent
	Naive     float64    `json:"naive_distance"` // the quest-by-quest order's length, for comparison
	Total     int        `json:"total"`          // stops on the route, a travel stop included
	Notes     []string   `json:"notes,omitempty"`
}

// spot is a stop before ordering; quests and names list the quests merged into it.
type spot struct {
	kind     string
	m        int
	x, y     float64
	quests   []int
	names    []string
	label    string // the NPC or mob
	must     bool   // part of a class quest: always on the route, never cut to "later"
	unplaced bool   // an objective with no known location: never routed, listed under Find
}

// quest is the input for one quest: where it starts, what to do, where it ends.
type quest struct {
	id       int
	name     string
	level    int
	pickup   *spot
	objs     []*spot
	unplaced []*spot // unfinished objectives with no location
	turnin   *spot
	must     bool // a class quest: its stops are must-include
}

// source supplies what the planner needs of the world.
type source interface {
	quests(c character.Character, at *questie.Where) []*quest
	mapName(m int) string
}

func nameOf(c character.Character) string { return cmp.Or(c.Name, c.Key) }

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// Build plans c's route from the player's own QuestieDB. A nil index or a character with no level
// gives an empty plan with a note saying why.
func Build(c character.Character, qx *questie.Index) Plan {
	if qx == nil {
		return Plan{Character: nameOf(c), Steps: []Stop{}, Notes: []string{"No Questie data (install Questie for your client): a route needs quest coordinates."}}
	}
	if c.Level == nil {
		return Plan{Character: nameOf(c), Steps: []Stop{}, Notes: []string{"The capture has no level, so no quest can be called eligible."}}
	}
	return plan(c, &questieSource{qx: qx})
}

func (q *quest) chain() []*spot {
	var out []*spot
	if q.pickup != nil {
		out = append(out, q.pickup)
	}
	out = append(out, q.objs...)
	if q.turnin != nil {
		out = append(out, q.turnin)
	}
	return out
}

func plan(c character.Character, src source) Plan {
	p := Plan{Character: nameOf(c), Steps: []Stop{}}
	pos := c.LastKnownPosition()
	lvl := *c.Level
	var at *questie.Where
	if pos != nil {
		at = &questie.Where{MapID: pos.MapID, Zone: pos.Zone, X: pos.X, Y: pos.Y}
	}

	var later []*spot
	var use []*quest
	var findSpots []*spot
	for _, q := range src.quests(c, at) {
		// The invariant: a turn-in is routed only when every unfinished objective has a stop before it.
		// An objective with no location is listed under Find and its quest's turn-in is withheld.
		if len(q.unplaced) > 0 {
			q.turnin = nil
			findSpots = append(findSpots, q.unplaced...)
		}
		if q.must {
			for _, sp := range q.chain() {
				sp.must = true
			}
		}
		if q.level > lvl+LevelMargin && !q.must {
			later = append(later, q.chain()...)
		} else if len(q.chain()) > 0 {
			use = append(use, q)
		}
	}

	// The map the route is on: the player's, else the one with the most stops.
	cur, known := 0, false
	if pos != nil && pos.MapID != nil {
		cur, known = *pos.MapID, true
		p.Start = &Point{Map: cur, X: round1(pos.X), Y: round1(pos.Y)}
	} else {
		count := map[int]int{}
		for _, q := range use {
			for _, s := range q.chain() {
				count[s.m]++
			}
		}
		for m, n := range count {
			if !known || n > count[cur] || (n == count[cur] && m < cur) {
				cur, known = m, true
			}
		}
	}
	if known {
		p.Zone = src.mapName(cur)
	}

	nodes, preds, chains := merge(use, cur, &later)
	prob := &Problem{}
	if p.Start != nil {
		prob.Start = &[2]float64{pos.X, pos.Y}
	}
	for i, s := range nodes {
		prob.Nodes = append(prob.Nodes, Node{X: s.x, Y: s.y, Pred: preds[i]})
	}
	naive := naiveOrder(chains, preds, len(nodes))
	order := Solve(prob, naive)
	p.Naive = round1(prob.Length(naive))
	if len(order) > MaxSteps {
		var kept []int
		var cut []*spot
		keep := mustSet(nodes, preds)
		room := MaxSteps - len(keep)
		for _, i := range order {
			if keep[i] {
				kept = append(kept, i)
			} else if room > 0 {
				kept, room = append(kept, i), room-1
			} else {
				cut = append(cut, nodes[i])
			}
		}
		later = append(later, cut...)
		order = kept
	}
	p.Distance = round1(prob.Length(order))
	for i, ni := range order {
		p.Steps = append(p.Steps, stopOf(i+1, nodes[ni]))
	}

	// Stops on other maps wait; the map with the most of them is named as the next place to travel to.
	counts := map[int]int{}
	for _, s := range later {
		if s.m != cur {
			counts[s.m]++
		}
	}
	next, best := 0, 0
	for m, n := range counts {
		if n > best || (n == best && m < next) {
			next, best = m, n
		}
	}
	if best > 0 {
		for _, s := range later {
			if s.m == next {
				p.Steps = append(p.Steps, Stop{N: len(p.Steps) + 1, Kind: KindTravel, Label: "Travel to " + src.mapName(next), Map: next, X: round1(s.x), Y: round1(s.y)})
				break
			}
		}
	}
	p.Total = len(p.Steps)
	for i, s := range findSpots {
		p.Find = append(p.Find, FindStop{N: i + 1, QuestID: s.quests[0], Quest: strings.Join(s.names, ", "), Label: s.label})
	}
	sort.SliceStable(later, func(i, j int) bool { return (later[i].m == cur) && (later[j].m != cur) })
	for i, s := range later {
		if i >= MaxLater {
			p.Notes = append(p.Notes, fmt.Sprintf("%d more later stops not listed", len(later)-MaxLater))
			break
		}
		p.Later = append(p.Later, stopOf(i+1, s))
	}
	return p
}

// mustSet is the nodes that must stay on the route: the must-include ones and everything they need first.
func mustSet(nodes []*spot, preds [][]int) map[int]bool {
	keep := map[int]bool{}
	var add func(i int)
	add = func(i int) {
		if keep[i] {
			return
		}
		keep[i] = true
		for _, p := range preds[i] {
			add(p)
		}
	}
	for i, n := range nodes {
		if n.must {
			add(i)
		}
	}
	return keep
}

func stopOf(n int, s *spot) Stop {
	st := Stop{N: n, Kind: s.kind, Map: s.m, X: round1(s.x), Y: round1(s.y)}
	if len(s.quests) > 0 {
		st.QuestID, st.Quest = s.quests[0], strings.Join(s.names, ", ")
	}
	with := func(prefix, sep string) string {
		l := prefix + strings.Join(s.names, ", ")
		if s.label != "" {
			l += sep + s.label
		}
		return l
	}
	switch s.kind {
	case KindPickup:
		st.Label = with("Pick up ", " from ")
	case KindObjective:
		st.Label = with("", ": ")
	case KindTurnIn:
		st.Label = with("Turn in ", " to ")
	}
	return st
}

// merge turns the quests' spots on map cur into nodes: identical stops are one. The part of a quest's
// chain from its first stop off the map on waits in *later. preds[i] are the nodes before node i;
// chains[k] are quest k's node indexes in order.
func merge(qs []*quest, cur int, later *[]*spot) (nodes []*spot, preds [][]int, chains [][]int) {
	find := func(s *spot) int {
		tol := pinRadius
		if s.kind == KindObjective {
			tol = MergeRadius
		}
		for i, n := range nodes {
			if n.kind == s.kind && n.m == s.m && math.Hypot(n.x-s.x, n.y-s.y) <= tol {
				n.quests = append(n.quests, s.quests...)
				n.names = append(n.names, s.names...)
				n.must = n.must || s.must
				return i
			}
		}
		cp := *s
		nodes = append(nodes, &cp)
		return len(nodes) - 1
	}
	type edge struct{ a, b int }
	seen := map[edge]bool{}
	var edges []edge
	for _, q := range qs {
		ch := q.chain()
		cut := len(ch)
		for i, s := range ch {
			if s.m != cur {
				cut = i
				break
			}
		}
		*later = append(*later, ch[cut:]...)
		var idx []int
		var pick, fin = -1, -1
		var objs []int
		for _, s := range ch[:cut] {
			i := find(s)
			idx = append(idx, i)
			switch s.kind {
			case KindPickup:
				pick = i
			case KindTurnIn:
				fin = i
			default:
				objs = append(objs, i)
			}
		}
		link := func(a, b int) {
			if e := (edge{a, b}); a >= 0 && b >= 0 && a != b && !seen[e] {
				seen[e] = true
				edges = append(edges, e)
			}
		}
		for _, o := range objs {
			link(pick, o)
			link(o, fin)
		}
		if len(objs) == 0 {
			link(pick, fin)
		}
		chains = append(chains, idx)
	}
	preds = make([][]int, len(nodes))
	for _, e := range edges {
		preds[e.b] = append(preds[e.b], e.a)
	}
	return nodes, preds, chains
}

// naiveOrder is the quest-by-quest order: each quest's stops in turn (the stops a merged stop needs
// first are visited first), a merged stop at its first use.
func naiveOrder(chains [][]int, preds [][]int, n int) []int {
	var order []int
	done := make([]bool, n)
	var visit func(i int)
	visit = func(i int) {
		if done[i] {
			return
		}
		done[i] = true
		for _, q := range preds[i] {
			visit(q)
		}
		order = append(order, i)
	}
	for _, ch := range chains {
		for _, i := range ch {
			visit(i)
		}
	}
	return order
}
