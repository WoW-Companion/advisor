package route

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/unlock"
)

// Limits.
const (
	MaxSteps    = 20 // stops on the route; the rest go to "later"
	MaxTotal    = 25 // stops on the route, locked follow-ups included
	MaxLater    = 30 // stops listed in "later"
	LevelMargin = 2  // a quest above the player's level + this goes to "later"
	// PickupBelow is how far under the player's level a new pick-up may be; MaxPickups is how many new
	// pick-ups the run takes; PickupDetour is how far (map percent, about 300 yards) a giver may be from the
	// route or the player to be "on the way".
	PickupBelow  = 3
	MaxPickups   = 3
	PickupDetour = 6.0
	// DungeonReach is how far above the player's level a dungeon's minimum may be, and how far under
	// the player's level its maximum may be, for its quests to be worth picking up in town.
	DungeonReach = 3
	// DefaultLogCap is the quest log's size; NearlyFull is how many free slots or fewer limit pick-ups.
	DefaultLogCap = 40
	NearlyFull    = 2
	MergeRadius   = 3.0 // objective stops this close (map percent) on one map are one stop
	ClusterSize   = 8.0 // spawns this close (map percent) are one cluster
	pinRadius     = 0.05
)

// FromGame marks a position taken from the game's own quest map point, captured by the addon.
const FromGame = "game"

// FromQuestie marks a position that came from the player's own Questie database rather than the dataset.
const FromQuestie = "from your Questie database"

// Stop kinds.
const (
	KindPickup    = "pickup"
	KindObjective = "objective"
	KindTurnIn    = "turnin"
	KindTravel    = "travel"
)

// NoPos is the X and Y of a stop with no position: the addon reads it as "do not draw". A stop is never
// emitted at 0, 0.
const NoPos = -1

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
	Place    string  `json:"place,omitempty"` // the subzone or zone the stop is in, in words (Plan.lua 1.3)
	// Level is the quest's level (0 when unknown) and Diff its difficulty colour for the player.
	Level int    `json:"level,omitempty"`
	Diff  string `json:"diff,omitempty"`
	// Priority is set on a class quest's stops: they are worth doing first, so they come early.
	Priority bool `json:"priority,omitempty"`
	// Optional is set on a holiday-event quest's stops: the player opted in, and they come after the rest.
	Optional bool `json:"optional,omitempty"`
	// ZoneOnly is a stop with no known position: X and Y are -1 and only Place is known.
	ZoneOnly bool `json:"zone_only,omitempty"`
	// Unplaced marks a stop with no position: X and Y are -1, the "no position" marker. Approx marks a
	// position taken from a subzone's centre, not from the objective's own spawn.
	Unplaced bool `json:"unplaced,omitempty"`
	Approx   bool `json:"approx,omitempty"`
	// Source names where a position came from when it is not the usual NPC or object spawn.
	Source string `json:"source,omitempty"`

	// A trip is a turn-in for a quest with nothing to do, handed in at another NPC or object (Plan.lua 1.5).
	// Finisher is that NPC, FinisherPlace where it is, Instance the dungeon it stands in, when one.
	Trip          bool   `json:"trip,omitempty"`
	Finisher      string `json:"finisher,omitempty"`
	FinisherPlace string `json:"finisher_place,omitempty"`
	Instance      string `json:"instance,omitempty"`

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
	// Errand is the kind of a city errand stop ("train", "bank", "class", "profession", "turnin"); empty on
	// an ordinary stop (Plan.lua 1.7).
	Errand string `json:"errand,omitempty"`
	// Needs are the numbers of the steps that must be done first: a quest's pick-up before its objectives
	// and turn-in, its objectives before the turn-in, a chain's unlocking turn-ins before the follow-up
	// (Plan.lua "after").
	Needs []int `json:"needs"`
}

// FindStop is an objective the planner could not place on the map: something to find, with no x or y.
type FindStop struct {
	N       int    `json:"n"`
	QuestID int    `json:"quest_id,omitempty"`
	Quest   string `json:"quest,omitempty"`
	Label   string `json:"label"`
	Level   int    `json:"level,omitempty"`
	Diff    string `json:"diff,omitempty"`
	// Hint and Zone are on a logged quest with no place on the map, mostly a done quest whose finisher is
	// unknown: Hint is the quest's own objective text, Zone where it is known to be (Plan.lua 1.7).
	Hint string `json:"hint,omitempty"`
	Zone string `json:"zone,omitempty"`
}

// Point is a place on a map, in map percent.
type Point struct {
	Map int     `json:"map"`
	X   float64 `json:"x"`
	Y   float64 `json:"y"`
	At  int64   `json:"at,omitempty"` // when the save the plan was built from was captured (unix seconds)
}

// Options are the chain-aware inputs of a plan; the zero value plans from Questie alone.
type Options struct {
	Lines  unlock.Lines   // the dataset's quest lines: a fallback where Questie is silent
	Obs    []unlock.Obs   // unlocks learned in game (the local file)
	Events []unlock.Event // the addon's observation log
	Depth  int            // follow-up links to look ahead; 0 is unlock.DefaultDepth

	// Dungeons are the dungeons' level ranges: a pick-up in town for a dungeon the player fits is a
	// current step. XP is the quest XP table (nil: no XP is shown). LogCap is the quest log's size
	// (0 is DefaultLogCap).
	Dungeons []DungeonLevel
	Places   *Places // the dataset's UI map and instance names, for maps Questie does not name
	XP       map[int]int
	LogCap   int
	// GroupPlan is set when the player has a plan to run dungeons with a group: their quests become trips
	// to the entrance zone. Otherwise a dungeon quest is parked.
	GroupPlan bool
	// PickupCap is how many new pick-ups the run takes (0 is MaxPickups, negative none); AnyPickup lifts the "on the way
	// or in the hub" rule, for a caller that wants every pick-up of the map.
	PickupCap int
	AnyPickup bool

	// Data is the dataset, for the class trainer's spells in a city errand (nil: none). Trainable counts
	// the spells the character can train now; nil reads them from Data.
	Data      store.DataStore
	Trainable func(c character.Character) int
	// Others are the player's captured characters, for the mail errands; BagSlots is the bag size (0: not
	// captured, so no sell-junk errand).
	Others   []character.Character
	BagSlots int
}

// DungeonLevel is a dungeon's level range; Map is its instance id (0 when unknown).
type DungeonLevel struct {
	Map      int
	Name     string
	Min, Max int
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
	Here      []Here       `json:"here,omitempty"`       // what can be done at each NPC stop (Plan.lua 1.3)
	Finishers []Finisher   `json:"finishers,omitempty"`  // where each logged quest is handed in (Plan.lua 1.4)
	Zones     []ZonePlan   `json:"zones,omitempty"`      // one connected route per other zone with stops (Plan.lua 1.5)
	Exit      *Exit        `json:"exit,omitempty"`       // the run's last line: the way to the next trip (Plan.lua 1.6)
	Trips     []Trip       `json:"trips,omitempty"`      // logged quests outside the run, by destination zone (Plan.lua 1.6)
	Parked    []Parked     `json:"parked,omitempty"`     // logged quests set aside, and why (Plan.lua 1.6)
	Errands   []Errand     `json:"errands,omitempty"`    // which steps are city errands (Plan.lua 1.7)
	Flights   []Flight     `json:"flights,omitempty"`    // flight masters of the run's zones, known or not (Plan.lua "flights")
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
	label    string        // the NPC or mob
	must     bool          // part of a class quest: always on the route, never cut to "later"
	event    bool          // only holiday-event quests use this stop: optional, cut before any other
	tags     []string      // on a pick-up: per quest in names, the dungeon and XP in brackets; "" for none
	town     bool          // a dungeon quest's pick-up in the player's town: first on the route
	dungeon  string        // with town: the dungeon, to keep its pick-ups together
	xp       int           // with town: the quest's XP, for the order
	endObj   bool          // placed from the quest's triggerEnd (an exploration objective)
	areaObj  bool          // placed by an area objective of the player's Questie database, not an NPC or object
	game     bool          // placed at the quest map point the client gave for the quest
	approx   bool          // placed at a subzone's centre named in the quest text, not at the objective's own spot
	zoneOnly bool          // no position anywhere: the zone is all that is known, so no coordinates are shown
	trip     *questie.Trip // a turn-in that is a journey: nothing to do, handed in elsewhere (never merged with an ordinary one)
	unplaced bool          // an objective with no known location: never routed, listed under Find
	npc      int           // the NPC's id, on a pick-up or turn-in; 0 when unknown
	ready    bool          // a turn-in for a quest in the log that is done: it can be handed in now
	level    int           // the quest's level; 0 when unknown
	place    string        // where it is in words: the subzone, else the zone; empty when unknown

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
	must     bool   // a class quest: its stops are must-include
	elite    bool   // the quest is flagged elite: it needs a group
	done     bool   // the log says every objective is done
	hint     string // Questie's own objective text, for a quest with no place
	event    bool   // a holiday-event quest the player opted in to: optional, after the rest

	// Pick-up facts (an unstarted quest): the dungeon it leads to with its level range, the quest's
	// XP from the table (0: unknown) and whether it is a holiday or festival quest.
	dungeon  string
	dMin     int
	dMax     int
	xp       int
	festival bool

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

// flightSource is a source that knows which maps the player can fly to, and names a travel target.
type flightSource interface {
	flights(c character.Character) (known map[int]bool, haveData bool)
	travelName(m int, place string) string
}

// FlyMinGap is how far below the player's level a ready quest may be and still be worth a trip.
const FlyMinGap = 10

// FlyLegs is the most fly-and-turn-in legs a plan adds.
const FlyLegs = 2

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
	return planWith(c, &questieSource{qx: qx, opt: opt}, opt)
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
	var use, listed, pickups []*quest
	var findSpots []*spot
	if !c.QuestsCaptured && len(c.Quests) == 0 {
		p.Notes = append(p.Notes, "The quest log is not in the capture yet, so no pick-ups are planned (a quest you already have would show as one).")
	}
	reach := c.Reach()
	for _, q := range src.quests(c, at) {
		for _, sp := range q.chain() {
			sp.level = q.level
		}
		for _, sp := range q.unplaced {
			sp.level = q.level
		}
		if c.IsHidden(q.id) {
			if inLogSet(p.Basis.InLog, q.id) {
				p.Parked = append(p.Parked, parkedOf(q, lvl, ParkHidden, "Hidden by you"))
			}
			continue
		}
		if q.pickup == nil && !q.locked && !q.must && !deliveryOnly(q) {
			if q.elite && !c.ElitesIncluded() {
				p.Parked = append(p.Parked, parkedOf(q, lvl, ParkElite, "Elite: needs a group"))
				continue
			}
			if tooHigh(q.level, lvl, reach) {
				p.Parked = append(p.Parked, parkedOf(q, lvl, ParkTooHigh, doableAround(q.level, reach)))
				continue
			}
		}
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
		if q.event {
			for _, sp := range q.chain() {
				sp.event = true
			}
		}
		if len(q.chain()) > 0 || len(q.unplaced) > 0 {
			listed = append(listed, q)
		}
		if q.pickup != nil && !q.locked {
			pickups = append(pickups, q) // decided below, with the quest log's room
		} else if tooHigh(q.level, lvl, reach) && !q.must && !(q.turnin != nil && q.turnin.ready) && !deliveryOnly(q) {
			later = append(later, q.chain()...)
		} else if len(q.chain()) > 0 {
			use = append(use, q)
		}
	}
	use = append(use, choosePickups(c, lvl, reach, opt, pickups, use, at, &later, &p.Notes)...)

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
	order = groupByPlace(prob, nodes, order)
	order = prioritize(prob, nodes, order)
	order = townFirst(prob, nodes, order)
	order = readyFirst(prob, nodes, order, cur)
	order = eventsLast(nodes, preds, order)
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
		// Holiday-event stops get only the room the others leave.
		others := 0
		for i, n := range nodes {
			if !n.locked && !n.event && !keep[i] {
				others++
			}
		}
		eventRoom := max(0, room-others)
		room -= eventRoom
		for _, i := range order {
			cut := false
			switch {
			case keep[i]:
			case nodes[i].locked:
				for _, q := range preds[i] {
					cut = cut || cutSet[q]
				}
			case nodes[i].event && eventRoom > 0:
				eventRoom--
			case !nodes[i].event && room > 0:
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
		st.Diff = DiffOf(st.Level, lvl)
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

	// A ready turn-in on another map is worth a trip when its town can be reached: one leg per town
	// names every turn-in there. The generic travel step then goes to the map with the most other stops.
	// Since Plan.lua 1.6 the legs are not steps: the run here is the stops on this map, and each leg is
	// the travel text of its zone's trip.
	legs := flyLegs(c, src, later, cur, lvl)
	// Every other zone with stops gets one connected route that starts where the player arrives.
	var zoneIdx map[int][]int
	later, p.Zones, zoneIdx = zoneRoutes(c, src, later, cur, known, p.Start)
	p.Total = len(p.Steps)
	p.Guides = guides(append(listed, chosen...), p.Steps, p.Zone, src.mapName)
	p.Here = buildHere(p, append(listed, chosen...), src)
	p.Finishers = buildFinishers(c, src, listed, p.Zone)
	for i, s := range findSpots {
		p.Find = append(p.Find, FindStop{N: i + 1, QuestID: s.quests[0], Quest: strings.Join(s.names, ", "), Label: s.label, Level: s.level, Diff: DiffOf(s.level, lvl)})
	}
	for i := range p.Zones {
		z := &p.Zones[i]
		z.Order = []int{}
		for _, k := range zoneIdx[z.Map] {
			if k < MaxLater {
				z.Order = append(z.Order, k+1)
			}
		}
	}
	for i, s := range later {
		if i >= MaxLater {
			p.Notes = append(p.Notes, fmt.Sprintf("%d more later stops not listed", len(later)-MaxLater))
			break
		}
		p.Later = append(p.Later, stopOf(i+1, s))
		p.Later[len(p.Later)-1].Diff = DiffOf(s.level, lvl)
	}
	shapeSharedPlan(&p, c, src, opt, sharedInput{reach: reach, listed: append(listed, chosen...), later: later, legs: legs, cur: cur, known: known, lvl: lvl})
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
	st := Stop{N: n, Kind: s.kind, Map: s.m, X: round1(s.x), Y: round1(s.y), Place: s.place, Priority: s.must, Optional: s.event}
	st.Level = s.level
	if s.zoneOnly {
		st.X, st.Y, st.ZoneOnly, st.Unplaced = NoPos, NoPos, true, true
	}
	st.Approx = s.approx
	if s.game {
		st.Source = FromGame
	}
	if s.areaObj {
		st.Source = FromQuestie
	}
	if s.locked {
		st.Locked, st.Depth, st.Chain, st.Observed = true, s.depth, s.chain, s.observed
		st.UnlockedBy = append([]int(nil), s.unlockedBy...)
		if s.kind == KindPickup {
			st.Giver = s.label
		}
	}
	if t := s.trip; t != nil && s.kind == KindTurnIn {
		st.Trip, st.Finisher, st.FinisherPlace, st.Instance = true, t.Finisher, t.Place(), t.Instance
	}
	st.Unlocks = append([]int(nil), s.unlocks...)
	if len(s.quests) > 0 {
		st.QuestID, st.Quest = s.quests[0], strings.Join(s.names, ", ")
		st.QuestIDs = slices.Clone(s.quests)
	}
	with := func(prefix, sep string) string {
		names := make([]string, len(s.names))
		for i, n := range s.names {
			names[i] = n
			if i < len(s.tags) && s.tags[i] != "" {
				names[i] += " (" + s.tags[i] + ")"
			}
		}
		l := prefix + strings.Join(names, ", ")
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
		if s.areaObj {
			st.Label += " (" + FromQuestie + ")"
		}
	case KindTurnIn:
		st.Label = with("Turn in ", " to ")
		if t := s.trip; t != nil {
			st.Label = tripLabel(*t, strings.Join(s.names, ", "))
		}
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
			if n.kind == s.kind && (n.trip == nil) == (s.trip == nil) && (s.kind != KindObjective || n.label == s.label) && n.locked == s.locked && n.town == s.town && n.depth == s.depth && n.m == s.m && math.Hypot(n.x-s.x, n.y-s.y) <= tol {
				n.quests = append(n.quests, s.quests...)
				n.tags = append(padTags(n.tags, len(n.names)), padTags(s.tags, len(s.names))...)
				n.names = append(n.names, s.names...)
				n.unlocks = append(n.unlocks, s.unlocks...)
				for _, u := range s.unlockedBy {
					if !slices.Contains(n.unlockedBy, u) {
						n.unlockedBy = append(n.unlockedBy, u)
					}
				}
				n.must = n.must || s.must
				n.event = n.event && s.event
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

// GroupRadius is how close (map percent) a pick-up or turn-in must be to another stop, or to the player,
// to be done together with it.
const GroupRadius = 3.0

// groupByPlace pulls a pick-up or turn-in forward to the first stop (or the player's start) within
// GroupRadius of it, when every precedence still holds and the route grows by no more than the detour to it can cost (twice GroupRadius):
// what can be done at one place is done together, and a turn-in the player will pass is not left to the end.
func groupByPlace(p *Problem, nodes []*spot, order []int) []int {
	near := func(a, b int) bool {
		return nodes[a].m == nodes[b].m && math.Hypot(nodes[a].x-nodes[b].x, nodes[a].y-nodes[b].y) <= GroupRadius
	}
	for pass := 0; pass < len(order); pass++ {
		moved := false
		for j := 1; j < len(order); j++ {
			n := order[j]
			if k := nodes[n].kind; k != KindPickup && k != KindTurnIn {
				continue
			}
			at := -2 // insert after the stop at this index of order; -1 is the front
			if p.Start != nil && math.Hypot(p.Start[0]-nodes[n].x, p.Start[1]-nodes[n].y) <= GroupRadius {
				at = -1
			}
			for k := 0; at == -2 && k < j-1; k++ {
				if near(order[k], n) {
					at = k
				}
			}
			if at == -2 || at+1 == j {
				continue
			}
			rest := slices.Delete(slices.Clone(order), j, j+1)
			cand := slices.Insert(rest, at+1, n)
			if p.Feasible(cand) && p.Length(cand) <= p.Length(order)+2*GroupRadius {
				order, moved = cand, true
			}
		}
		if !moved {
			break
		}
	}
	return order
}

// eventsLast moves the stops only holiday-event quests use after every other stop, keeping their
// order. A stop another quest's stop needs first stays where it is.
func eventsLast(nodes []*spot, preds [][]int, order []int) []int {
	late := map[int]bool{}
	for i, n := range nodes {
		late[i] = n.event
	}
	for j, n := range nodes {
		if !n.event {
			for _, p := range preds[j] {
				late[p] = false
			}
		}
	}
	var first, last []int
	for _, i := range order {
		if late[i] {
			last = append(last, i)
		} else {
			first = append(first, i)
		}
	}
	return append(first, last...)
}

// prioritize puts the objectives of class quests (and what they need first) early: after whatever can
// be done at the player's current spot, before the rest. Every precedence is kept; the rest keep their order.
func prioritize(p *Problem, nodes []*spot, order []int) []int {
	pri := map[int]bool{}
	var add func(i int)
	add = func(i int) {
		if pri[i] {
			return
		}
		pri[i] = true
		for _, q := range p.Nodes[i].Pred {
			add(q)
		}
	}
	found := false
	for i, n := range nodes {
		if n.must && n.kind == KindObjective && !n.locked {
			add(i)
			found = true
		}
	}
	if !found {
		return order
	}
	// What can be done at the player's current spot goes first, in its order.
	inHead := map[int]bool{}
	var out, rest []int
	if p.Start != nil {
		for _, i := range order {
			n := nodes[i]
			ok := math.Hypot(p.Start[0]-n.x, p.Start[1]-n.y) <= GroupRadius && !n.zoneOnly
			for _, q := range p.Nodes[i].Pred {
				ok = ok && inHead[q]
			}
			if ok {
				inHead[i] = true
				out = append(out, i)
			}
		}
	}
	for _, i := range order {
		switch {
		case inHead[i]:
		case pri[i]:
			out = append(out, i)
		default:
			rest = append(rest, i)
		}
	}
	// A priority stop's own precedence may sit in the head or the rest; the closure kept them in pri.
	out = append(out, rest...)
	if !p.Feasible(out) {
		return order
	}
	return out
}

// QuestOrder is the quest ids in route order: each quest where its first stop is. The panel's Next
// quests and the answers' facts follow it, so the route, the panel and the AI agree.
func QuestOrder(p Plan) []int {
	var out []int
	seen := map[int]bool{}
	for _, s := range p.Steps {
		for _, id := range s.QuestIDs {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
		if len(s.QuestIDs) == 0 && s.QuestID != 0 && !seen[s.QuestID] {
			seen[s.QuestID] = true
			out = append(out, s.QuestID)
		}
	}
	return out
}

// travelName is the place a travel step names: never a bare map number.
func travelName(src source, m int, place string) string {
	if fs, ok := src.(flightSource); ok {
		return fs.travelName(m, place)
	}
	return cmp.Or(src.mapName(m), place, "another area")
}

// readyFirst puts the ready turn-ins on the player's map first, nearest first: what can be handed in
// where the player stands comes before any pick-up or objective. Every precedence is kept.
func readyFirst(p *Problem, nodes []*spot, order []int, cur int) []int {
	var head []int
	isHead := map[int]bool{}
	near := func(n *spot) bool {
		return p.Start != nil && math.Hypot(n.x-p.Start[0], n.y-p.Start[1]) <= NearReady
	}
	for _, i := range order {
		n := nodes[i]
		// A ready turn-in goes first; so does a delivery with nothing to do before it when it is close by.
		if n.kind == KindTurnIn && (n.ready || (n.trip != nil && near(n))) && n.m == cur && !n.zoneOnly && len(p.Nodes[i].Pred) == 0 {
			head = append(head, i)
			isHead[i] = true
		}
	}
	if len(head) == 0 {
		return order
	}
	var out []int
	px, py, have := 0.0, 0.0, p.Start != nil
	if have {
		px, py = p.Start[0], p.Start[1]
	}
	left := slices.Clone(head)
	for len(left) > 0 {
		bi := 0
		if have {
			for k, i := range left {
				if math.Hypot(nodes[i].x-px, nodes[i].y-py) < math.Hypot(nodes[left[bi]].x-px, nodes[left[bi]].y-py) {
					bi = k
				}
			}
		}
		i := left[bi]
		out = append(out, i)
		px, py, have = nodes[i].x, nodes[i].y, true
		left = slices.Delete(left, bi, bi+1)
	}
	for _, i := range order {
		if !isHead[i] {
			out = append(out, i)
		}
	}
	if !p.Feasible(out) {
		return order
	}
	return out
}

// flyLegs are the fly-and-turn-in legs: ready turn-ins on other maps, grouped by town, for the towns
// the player can reach. Without flight data the towns holding the most ready turn-ins qualify.
func flyLegs(c character.Character, src source, later []*spot, cur, lvl int) []Stop {
	type town struct {
		m     int
		spots []*spot
	}
	by := map[int]*town{}
	var towns []*town
	for _, s := range later {
		if s.kind != KindTurnIn || !s.ready || s.m == cur || s.zoneOnly || (s.trip != nil && s.trip.Instance != "") || (s.level > 0 && s.level < lvl-FlyMinGap) {
			continue
		}
		t := by[s.m]
		if t == nil {
			t = &town{m: s.m}
			by[s.m] = t
			towns = append(towns, t)
		}
		t.spots = append(t.spots, s)
	}
	var known map[int]bool
	haveData := false
	if fs, ok := src.(flightSource); ok {
		known, haveData = fs.flights(c)
	}
	// A flight never crosses the sea: a town on another continent than the player's is reached by the zeppelin, boat or portal.
	here := learn.ContinentOf(src.mapName(cur))
	var keep []*town
	for _, t := range towns {
		if there := learn.ContinentOf(src.mapName(t.m)); here != "" && there != "" && here != there {
			continue
		}
		if !haveData || known[t.m] {
			keep = append(keep, t)
		}
	}
	sort.SliceStable(keep, func(i, j int) bool {
		if len(keep[i].spots) != len(keep[j].spots) {
			return len(keep[i].spots) > len(keep[j].spots)
		}
		return keep[i].m < keep[j].m
	})
	if len(keep) > FlyLegs {
		keep = keep[:FlyLegs]
	}
	var out []Stop
	for _, t := range keep {
		var names []string
		var ids []int
		for _, s := range t.spots {
			for k, id := range s.quests {
				if !slices.Contains(ids, id) {
					ids = append(ids, id)
					names = append(names, s.names[k])
				}
			}
		}
		first := t.spots[0]
		out = append(out, Stop{Kind: KindTravel, Label: "Fly to " + travelName(src, t.m, first.place) + ": turn in " + strings.Join(names, ", "),
			Map: t.m, X: round1(first.x), Y: round1(first.y), QuestID: ids[0], Quest: strings.Join(names, ", "), QuestIDs: ids, Place: first.place})
	}
	return out
}

func padTags(t []string, n int) []string {
	for len(t) < n {
		t = append(t, "")
	}
	return t
}

// dungeonFits reports whether the dungeon's level range suits a player of level lvl: it opens at most
// DungeonReach levels above them, and closes at most DungeonReach levels below them.
func dungeonFits(q *quest, lvl int) bool {
	return q.dungeon != "" && q.dMin > 0 && q.dMax >= q.dMin && q.dMin <= lvl+DungeonReach && lvl <= q.dMax+DungeonReach
}

// choosePickups decides which pick-ups in the player's town are current steps; the rest go to later.
// A pick-up is worth taking when the quest is within the level margin, a class quest, or leads to a
// dungeon the player fits (it is then tagged with the dungeon and XP, and goes first on the route).
// Holiday and festival quests never outrank these: they wait unless nothing else is planned. A log
// that is full or nearly full limits the pick-ups to its free slots, best first. others is how many
// quests are already planned.
func choosePickups(c character.Character, lvl, reach int, opt Options, pickups []*quest, planned []*quest, at *questie.Where, later *[]*spot, notes *[]string) []*quest {
	others := len(planned)
	var want, fest []*quest
	for _, q := range pickups {
		fit := dungeonFits(q, lvl)
		switch {
		case tooHigh(q.level, lvl, reach) && !q.must && !fit:
			*later = append(*later, q.chain()...)
		case q.level > 0 && q.level < lvl-PickupBelow && !q.must && !fit:
			*later = append(*later, q.chain()...)
		case q.festival && !q.must:
			fest = append(fest, q)
		default:
			if fit {
				tag := q.dungeon
				if q.xp > 0 {
					tag += ", " + thousands(q.xp) + " XP"
				}
				q.pickup.town, q.pickup.dungeon, q.pickup.xp, q.pickup.tags = true, q.dungeon, q.xp, []string{tag}
			}
			want = append(want, q)
		}
	}
	want = earnPlace(opt, want, planned, at, later)
	if len(want) == 0 && others == 0 {
		want, fest = fest, nil // nothing else on the map: a festival quest is better than none
	}
	for _, q := range fest {
		*later = append(*later, q.chain()...)
	}
	logCap := cmp.Or(opt.LogCap, DefaultLogCap)
	if free := max(logCap-len(c.Quests), 0); free <= NearlyFull && len(want) > 0 {
		sort.SliceStable(want, func(i, j int) bool {
			a, b := want[i], want[j]
			if fa, fb := a.pickup.town, b.pickup.town; fa != fb {
				return fa
			}
			if a.must != b.must {
				return a.must
			}
			if a.xp != b.xp {
				return a.xp > b.xp
			}
			return a.level > b.level
		})
		if len(want) > free {
			for _, q := range want[free:] {
				*later = append(*later, q.chain()...)
			}
			want = want[:free]
		}
		*notes = append(*notes, fmt.Sprintf("The quest log is nearly full (%d of %d): %d pick-ups planned at most.", len(c.Quests), logCap, free))
	}
	return want
}

// thousands writes n with a comma between thousands: 2100 is "2,100".
func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// townFirst puts the dungeon pick-ups of the player's town at the head of the route, a dungeon's
// together, the best XP first. A pick-up needs nothing before it, so every precedence holds.
func townFirst(p *Problem, nodes []*spot, order []int) []int {
	var head, rest []int
	for _, i := range order {
		if nodes[i].town && len(p.Nodes[i].Pred) == 0 {
			head = append(head, i)
		} else {
			rest = append(rest, i)
		}
	}
	if len(head) == 0 {
		return order
	}
	best := map[string]int{}
	for _, i := range head {
		best[nodes[i].dungeon] = max(best[nodes[i].dungeon], nodes[i].xp)
	}
	sort.SliceStable(head, func(a, b int) bool {
		na, nb := nodes[head[a]], nodes[head[b]]
		if na.dungeon != nb.dungeon {
			if best[na.dungeon] != best[nb.dungeon] {
				return best[na.dungeon] > best[nb.dungeon]
			}
			return na.dungeon < nb.dungeon
		}
		return na.xp > nb.xp
	})
	return append(head, rest...)
}

// earnPlace keeps the new pick-ups that earn a place in the run: those on the way (a giver within
// PickupDetour of the player or of a stop of a quest already in the log) or in the zone's hub (within
// NearbyRadius of the player), at most MaxPickups. Class quests and the dungeon quests of a town are exempt.
// The rest wait in later.
func earnPlace(opt Options, want []*quest, planned []*quest, at *questie.Where, later *[]*spot) []*quest {
	var out []*quest
	taken := 0
	for _, q := range want {
		if q.must || q.pickup.town {
			out = append(out, q)
			continue
		}
		onWay := false
		if at != nil && at.MapID != nil && q.pickup.m == *at.MapID {
			d := math.Hypot(q.pickup.x-at.X, q.pickup.y-at.Y)
			onWay = d <= NearbyRadius
		}
		for _, pq := range planned {
			for _, s := range pq.chain() {
				if s.m == q.pickup.m && math.Hypot(s.x-q.pickup.x, s.y-q.pickup.y) <= PickupDetour {
					onWay = true
				}
			}
		}
		if !(onWay || opt.AnyPickup) || taken >= cmp.Or(opt.PickupCap, MaxPickups) {
			*later = append(*later, q.chain()...)
			continue
		}
		taken++
		out = append(out, q)
	}
	return out
}

// NearReady is how close (map percent, about 250 yards) a ready turn-in or delivery must be to the player,
// or to the run's start, to be done first.
const NearReady = 5.0

// DiffOf is the game's difficulty colour of a quest of level q for a player of level p: "red" 5 or more
// levels above, "orange" 3 to 4, "yellow" within 2, "green" down to the level above GreyLevel, then "grey". Empty
// when the level is unknown.
func DiffOf(q, p int) string {
	if q <= 0 || p <= 0 {
		return ""
	}
	switch d := q - p; {
	case d >= 5:
		return "red"
	case d >= 3:
		return "orange"
	case d >= -2:
		return "yellow"
	case q > GreyLevel(p):
		return "green"
	}
	return "grey"
}

// tooHigh is whether a quest of level q is past the player's reach; reach -1 turns the limit off.
func tooHigh(q, lvl, reach int) bool { return reach >= 0 && q > lvl+reach }

// doableAround is the text of a parked too-high quest.
func doableAround(q, reach int) string {
	return fmt.Sprintf("level %d · doable around %d", q, q-max(reach, 0))
}

// deliveryOnly is a quest with nothing to do but hand in or speak with someone: the level limit never
// applies to it.
func deliveryOnly(q *quest) bool {
	if q.done || q.turnin != nil && q.turnin.ready {
		return true
	}
	return len(q.objs) == 0 && len(q.unplaced) == 0 && q.turnin != nil
}

func inLogSet(inLog []int, id int) bool { _, ok := slices.BinarySearch(inLog, id); return ok }

// parkedOf is a parked entry for a quest.
func parkedOf(q *quest, lvl int, kind, why string) Parked {
	return Parked{QuestID: q.id, Quest: q.name, Why: why, Kind: kind, Level: q.level, Diff: DiffOf(q.level, lvl)}
}
