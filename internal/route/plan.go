package route

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/unlock"
)

// Limits.
const (
	MaxSteps    = 20  // stops on the route; the rest go to "later"
	MaxTotal    = 25  // stops on the route, locked follow-ups included
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
	N       int    `json:"n"`
	Kind    string `json:"kind"`
	QuestID int    `json:"quest_id,omitempty"`
	Quest   string `json:"quest,omitempty"`
	// QuestIDs is every quest the stop serves (a merged stop serves several); QuestID is the first.
	QuestIDs []int   `json:"quest_ids,omitempty"`
	Label    string  `json:"label"`
	Map      int     `json:"map"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Done     bool    `json:"done"`

	// Chain-aware plan (Plan.lua 1.1). A locked step is a follow-up the plan's turn-ins unlock.
	Locked     bool   `json:"locked,omitempty"`
	UnlockedBy []int  `json:"unlocked_by,omitempty"` // the turn-ins that unlock it
	Depth      int    `json:"depth,omitempty"`       // links from a planned quest
	Chain      string `json:"chain,omitempty"`       // the quest line's name, when known
	Unlocks    []int  `json:"unlocks,omitempty"`     // on a turn-in: the follow-ups it unlocks
	After      int    `json:"after,omitempty"`       // on a locked step: the step number of its last unlocking turn-in
	Observed   bool   `json:"observed,omitempty"`    // the link or the giver was observed in game
	Giver      string `json:"giver,omitempty"`       // on a locked pick-up: the NPC
	Here       bool   `json:"here,omitempty"`        // on a locked pick-up: the same NPC as the unlocking turn-in
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

// Options are the chain-aware inputs of a plan; the zero value plans from Questie alone.
type Options struct {
	Lines  unlock.Lines   // the dataset's quest lines: a fallback where Questie is silent
	Obs    []unlock.Obs   // unlocks learned in game (the local file)
	Events []unlock.Event // the addon's observation log
	Depth  int            // follow-up links to look ahead; 0 is unlock.DefaultDepth
}

// followSource is a source that can name the follow-ups of the planned quests.
type followSource interface {
	followUps(use []*quest, cur int, opt Options) []*quest
}

// Plan is the ordered route for one character.
type Plan struct {
	Character string       `json:"character"`
	Zone      string       `json:"zone,omitempty"`
	Start     *Point       `json:"start,omitempty"`
	Steps     []Stop       `json:"steps"`
	Later     []Stop       `json:"later,omitempty"`
	Find      []FindStop   `json:"find,omitempty"` // objectives with no known location; not on the route
	Distance  float64      `json:"distance"`       // route length in map percent
	Naive     float64      `json:"naive_distance"` // the quest-by-quest order's length, for comparison
	Total     int          `json:"total"`          // stops on the route, a travel stop included
	Notes     []string     `json:"notes,omitempty"`
	Follow    []FollowNote `json:"follow_ups,omitempty"` // the locked follow-ups on the route, one per quest
	Basis     Basis        `json:"basis"`                // what the plan was built from
	Guides    []Guide      `json:"guides,omitempty"`     // what each listed quest needs once accepted (Plan.lua 1.2)
}

// Basis is the state a plan starts from. "Done" in a plan means done after Captured: a reader judges
// steps against it, and never counts what was already true (a quest in InLog) as progress.
type Basis struct {
	Captured int64 `json:"captured,omitempty"` // capture time, unix seconds; 0 when unknown
	InLog    []int `json:"in_log,omitempty"`   // quest ids in the log when the plan was built, sorted
}

// FollowNote describes one follow-up quest on the route, for the digest's "Unlocks:" line.
type FollowNote struct {
	QuestID    int    `json:"quest_id"`
	Quest      string `json:"quest"`
	NPC        string `json:"npc,omitempty"`
	Here       bool   `json:"here,omitempty"` // the NPC who takes the unlocking turn-in
	Observed   bool   `json:"observed,omitempty"`
	UnlockedBy []int  `json:"unlocked_by"`
}

// UnlockNotes is, for each turn-in quest id, the "Quest (from NPC, here)" texts of the follow-ups it
// unlocks on this route.
func UnlockNotes(p Plan) map[int][]string {
	out := map[int][]string{}
	for _, f := range p.Follow {
		t := f.Quest
		var bits []string
		if f.NPC != "" {
			bits = append(bits, "from "+f.NPC)
		}
		if f.Here {
			bits = append(bits, "here")
		}
		if f.Observed {
			bits = append(bits, "observed")
		}
		if len(bits) > 0 {
			t += " (" + strings.Join(bits, ", ") + ")"
		}
		for _, a := range f.UnlockedBy {
			out[a] = append(out[a], t)
		}
	}
	return out
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

	locked     bool // a follow-up's stop: never merged with an ordinary one
	depth      int
	unlockedBy []int
	chain      string
	observed   bool
	unlocks    []int // on a turn-in: the follow-ups it unlocks
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

	locked     bool // a follow-up the planned turn-ins unlock
	depth      int
	unlockedBy []int
	chainName  string
	observed   bool
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
func Build(c character.Character, qx *questie.Index) Plan { return BuildWith(c, qx, Options{}) }

// BuildWith is Build with the chain-aware inputs: the dataset's quest lines, unlocks learned in
// game and the addon's observation log.
func BuildWith(c character.Character, qx *questie.Index, opt Options) Plan {
	c = unlock.Apply(c, opt.Events)
	if qx == nil {
		return Plan{Character: nameOf(c), Steps: []Stop{}, Notes: []string{"No Questie data (install Questie for your client): a route needs quest coordinates."}}
	}
	if c.Level == nil {
		return Plan{Character: nameOf(c), Steps: []Stop{}, Notes: []string{"The capture has no level, so no quest can be called eligible."}}
	}
	return planWith(c, &questieSource{qx: qx}, opt)
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

func plan(c character.Character, src source) Plan { return planWith(c, src, Options{}) }

func planWith(c character.Character, src source, opt Options) Plan {
	p := Plan{Character: nameOf(c), Steps: []Stop{}}
	if c.CapturedAt != nil {
		p.Basis.Captured = *c.CapturedAt
	}
	for _, lq := range c.Quests {
		p.Basis.InLog = append(p.Basis.InLog, lq.QuestID)
	}
	sort.Ints(p.Basis.InLog)
	pos := c.LastKnownPosition()
	lvl := *c.Level
	var at *questie.Where
	if pos != nil {
		at = &questie.Where{MapID: pos.MapID, Zone: pos.Zone, X: pos.X, Y: pos.Y}
	}

	var later []*spot
	var use, listed []*quest
	var findSpots []*spot
	if !c.QuestsCaptured && len(c.Quests) == 0 {
		p.Notes = append(p.Notes, "The quest log is not in the capture yet, so no pick-ups are planned (a quest you already have would show as one).")
	}
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
		if len(q.chain()) > 0 || len(q.unplaced) > 0 {
			listed = append(listed, q)
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

	// Follow-ups the planned turn-ins unlock, within the stop budget, nearer and shallower first.
	var follows []*quest
	if fs, ok := src.(followSource); ok && known {
		follows = fs.followUps(use, cur, opt)
	}
	var scratch []*spot
	n0, _, _, _ := merge(use, cur, &scratch)
	budget := MaxTotal - min(len(n0), MaxSteps)
	turnOf := map[int]*spot{}
	for _, q := range use {
		if q.turnin != nil && q.turnin.m == cur {
			turnOf[q.id] = q.turnin
		}
	}
	hidden := 0
	var chosen []*quest
	for _, f := range follows {
		cost := len(f.chain())
		ok := f.pickup != nil && f.pickup.m == cur && cost <= budget
		for _, a := range f.unlockedBy {
			ok = ok && turnOf[a] != nil
		}
		if !ok {
			hidden++
			continue
		}
		budget -= cost
		for _, a := range f.unlockedBy {
			turnOf[a].unlocks = append(turnOf[a].unlocks, f.id)
		}
		if f.turnin != nil && f.turnin.m == cur {
			turnOf[f.id] = f.turnin
		}
		for _, sp := range f.chain() {
			sp.locked, sp.depth, sp.unlockedBy, sp.chain, sp.observed = true, f.depth, f.unlockedBy, f.chainName, f.observed
		}
		use = append(use, f)
		chosen = append(chosen, f)
	}
	if hidden > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("%d follow-up quests not planned (the %d-step cap, or no known giver on this map)", hidden, MaxTotal))
	}

	nodes, preds, chains, _ := merge(use, cur, &later)
	prob := &Problem{}
	if p.Start != nil {
		prob.Start = &[2]float64{pos.X, pos.Y}
	}
	for i, s := range nodes {
		prob.Nodes = append(prob.Nodes, Node{X: s.x, Y: s.y, Pred: preds[i]})
	}
	naive := naiveOrder(chains, preds, len(nodes))
	order := Solve(prob, naive)
	order = adjacent(prob, nodes, order)
	p.Naive = round1(prob.Length(naive))
	// The cap counts ordinary stops; a follow-up stays when everything it needs does.
	reg := 0
	for _, n := range nodes {
		if !n.locked {
			reg++
		}
	}
	if reg > MaxSteps {
		var kept []int
		cutSet := map[int]bool{}
		keep := mustSet(nodes, preds)
		room := MaxSteps - len(keep)
		for _, i := range order {
			cut := false
			switch {
			case keep[i]:
			case nodes[i].locked:
				for _, q := range preds[i] {
					cut = cut || cutSet[q]
				}
			case room > 0:
				room--
			default:
				cut = true
			}
			if cut {
				cutSet[i] = true
				later = append(later, nodes[i])
			} else {
				kept = append(kept, i)
			}
		}
		order = kept
	}
	p.Distance = round1(prob.Length(order))
	turnStep := map[int]int{}
	turnLabel := map[int]string{}
	for i, ni := range order {
		if n := nodes[ni]; n.kind == KindTurnIn {
			for _, id := range n.quests {
				turnStep[id], turnLabel[id] = i+1, n.label
			}
		}
	}
	onRoute := map[int]bool{}
	for _, ni := range order {
		for _, id := range nodes[ni].quests {
			onRoute[id] = true
		}
	}
	for _, f := range chosen {
		if !onRoute[f.id] {
			continue
		}
		fn := FollowNote{QuestID: f.id, Quest: f.name, NPC: f.pickup.label, Observed: f.observed, UnlockedBy: f.unlockedBy, Here: true}
		for _, a := range f.unlockedBy {
			if t := turnOf[a]; t == nil || t.label != f.pickup.label {
				fn.Here = false
			}
		}
		p.Follow = append(p.Follow, fn)
	}
	for i, ni := range order {
		st := stopOf(i+1, nodes[ni])
		if st.Locked {
			here := st.Kind == KindPickup && st.Giver != ""
			for _, a := range st.UnlockedBy {
				st.After = max(st.After, turnStep[a])
				here = here && turnLabel[a] == st.Giver
			}
			st.Here = here
		}
		p.Steps = append(p.Steps, st)
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
	p.Guides = guides(append(listed, chosen...), p.Steps, p.Zone, src.mapName)
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
	if s.locked {
		st.Locked, st.Depth, st.Chain, st.Observed = true, s.depth, s.chain, s.observed
		st.UnlockedBy = append([]int(nil), s.unlockedBy...)
		if s.kind == KindPickup {
			st.Giver = s.label
		}
	}
	st.Unlocks = append([]int(nil), s.unlocks...)
	if len(s.quests) > 0 {
		st.QuestID, st.Quest = s.quests[0], strings.Join(s.names, ", ")
		st.QuestIDs = slices.Clone(s.quests)
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
		st.Label = s.label // already an action: "Kill <mob> (0/1)", the quest not repeated
	case KindTurnIn:
		st.Label = with("Turn in ", " to ")
	}
	return st
}

// merge turns the quests' spots on map cur into nodes: identical stops are one. The part of a quest's
// chain from its first stop off the map on waits in *later. preds[i] are the nodes before node i;
// chains[k] are quest k's node indexes in order.
func merge(qs []*quest, cur int, later *[]*spot) (nodes []*spot, preds [][]int, chains [][]int, turnNode map[int]int) {
	turnNode = map[int]int{}
	pickNode := map[int]int{}
	find := func(s *spot) int {
		tol := pinRadius
		if s.kind == KindObjective {
			tol = MergeRadius
		}
		for i, n := range nodes {
			if n.kind == s.kind && n.locked == s.locked && n.depth == s.depth && n.m == s.m && math.Hypot(n.x-s.x, n.y-s.y) <= tol {
				n.quests = append(n.quests, s.quests...)
				n.names = append(n.names, s.names...)
				n.unlocks = append(n.unlocks, s.unlocks...)
				for _, u := range s.unlockedBy {
					if !slices.Contains(n.unlockedBy, u) {
						n.unlockedBy = append(n.unlockedBy, u)
					}
				}
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
		if pick >= 0 {
			pickNode[q.id] = pick
		}
		if fin >= 0 {
			turnNode[q.id] = fin
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
	// A follow-up's pick-up comes after every turn-in that unlocks it.
	for _, q := range qs {
		pk, ok := pickNode[q.id]
		for _, a := range q.unlockedBy {
			if t, tok := turnNode[a]; ok && tok && t != pk && !seen[edge{t, pk}] {
				seen[edge{t, pk}] = true
				edges = append(edges, edge{t, pk})
			}
		}
	}
	preds = make([][]int, len(nodes))
	for _, e := range edges {
		preds[e.b] = append(preds[e.b], e.a)
	}
	return nodes, preds, chains, turnNode
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

// adjacent moves a follow-up's pick-up to just after the turn-in that unlocks it when both are at
// one place, if that keeps every precedence and does not lengthen the route: the NPC who finishes
// the quest hands out the next one.
func adjacent(p *Problem, nodes []*spot, order []int) []int {
	for k := 0; k < len(order); k++ {
		i := order[k]
		n := nodes[i]
		if !n.locked || n.kind != KindPickup {
			continue
		}
		for _, t := range p.Nodes[i].Pred {
			if nodes[t].kind != KindTurnIn || nodes[t].m != n.m || math.Hypot(nodes[t].x-n.x, nodes[t].y-n.y) > pinRadius*20 {
				continue
			}
			from, to := slices.Index(order, i), slices.Index(order, t)
			if to+1 == from {
				break
			}
			rest := slices.Delete(slices.Clone(order), from, from+1)
			to = slices.Index(rest, t)
			cand := slices.Insert(rest, to+1, i)
			if p.Feasible(cand) && p.Length(cand) <= p.Length(order)+1e-9 {
				order = cand
			}
			break
		}
	}
	return order
}
