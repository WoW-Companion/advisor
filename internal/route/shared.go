package route

import (
	"cmp"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// Parked kinds (Plan.lua 1.6).
const (
	ParkTooHigh  = "toohigh"
	ParkGrey     = "grey"
	ParkDungeon  = "dungeon"
	ParkSeasonal = "seasonal"
	ParkHidden   = "hidden" // the player hid it
	ParkElite    = "elite"  // an elite quest, needs a group
)

// Limits of the parked list.
const (
	// FullLogShare is the share of the quest log (3 in 4: 30 of 40) from which a grey quest is "worth abandoning".
	fullLogNum, fullLogDen = 3, 4
)

// GreyLevel is the highest quest level that is grey (gives next to no XP and no colour) for a player of level p, 0 when none is: the
// game's gray level, which the Warcraft Wiki gives for quests as for creatures. Under level 5 no level is grey; to level 39 it is
// p-5-p/10, to level 59 p-1-p/5, and from level 60 on p-9 (a gap of 6 at level 10, 7 at level 20, 8 at level 30, 11 at level 50, 9 at
// level 60), not one gap for every level.
func GreyLevel(p int) int {
	switch {
	case p < 5:
		return 0
	case p < 40:
		return p - 5 - p/10
	case p < 60:
		return p - 1 - p/5
	}
	return p - 9
}

// Exit is the last line of the run here: how to leave for the next trip. It is nil when the next trip
// is in the same zone.
type Exit struct {
	Label string  `json:"label"`
	Map   int     `json:"map"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Via   string  `json:"via"` // "flight", "zeppelin", "boat" or "walk"
}

// Trip is the logged quests that wait in one zone outside the run here, in suggested travel order.
type Trip struct {
	Order    int    `json:"order"`
	Map      int    `json:"map"`
	Zone     string `json:"zone,omitempty"`
	Ready    int    `json:"ready"`   // quests ready to turn in there
	Todo     int    `json:"todo"`    // quests with work left there
	Pickups  int    `json:"pickups"` // quests not yet in the log that can be picked up there (not in QuestIDs)
	QuestIDs []int  `json:"quest_ids"`
	Reason   string `json:"reason"`
	Travel   string `json:"travel"`
	Instance string `json:"instance,omitempty"`
	XP       int    `json:"xp,omitempty"`
	ETA      int    `json:"eta"` // minutes of travel from the previous stop on the route (the exit for the first trip)
	via      string
}

// Parked is a logged quest that is in no step and no trip, and why.
type Parked struct {
	QuestID int    `json:"quest_id"`
	Quest   string `json:"quest"`
	Why     string `json:"why"`
	Kind    string `json:"kind"`
	Level   int    `json:"level,omitempty"`
	Diff    string `json:"diff,omitempty"`
}

// entranceSource is a source that knows which zone a dungeon's entrance stands in.
type entranceSource interface {
	// entranceOf is the dungeon's name and the map of the zone its entrance is in (0: unknown), for an instance map.
	entranceOf(m int) (name string, zone int)
}

// dungeonSource is a source that knows which dungeon a quest leads to, and the zone its entrance is in.
type dungeonSource interface {
	// dungeonOf is the dungeon's name with its entrance zone, and the entrance zone's map (0: unknown).
	dungeonOf(questID int) (label string, zone int)
}

type sharedInput struct {
	listed []*quest
	later  []*spot
	legs   []Stop
	cur    int
	known  bool
	lvl    int
	reach  int // the level reach: -1 is off
}

// shapeSharedPlan fills Exit, Trips and Parked from the quest log: every logged quest is in the run
// here (a step), in exactly one trip, or parked. The steps are the run; nothing in them is a journey.
func shapeSharedPlan(p *Plan, c character.Character, src source, opt Options, in sharedInput) {
	inLog := map[int]bool{}
	for _, id := range p.Basis.InLog {
		inLog[id] = true
	}
	inSteps := map[int]bool{}
	for _, s := range p.Steps {
		inSteps[s.QuestID] = true
		for _, id := range s.QuestIDs {
			inSteps[id] = true
		}
	}
	logCap := cmp.Or(opt.LogCap, DefaultLogCap)
	crowded := len(c.Quests)*fullLogDen >= logCap*fullLogNum

	byMap := map[int]*Trip{}
	var trips []*Trip
	seen := map[int]bool{}
	for _, q := range in.listed {
		if !inLog[q.id] || inSteps[q.id] || seen[q.id] {
			continue
		}
		seen[q.id] = true
		m, ready := nextAction(q)
		if m == 0 {
			continue // no known place: it stays under find
		}
		instance, entrance := "", 0
		if isInstance(src, m) {
			if es, ok := src.(entranceSource); ok {
				instance, entrance = es.entranceOf(m)
			}
			instance = cmp.Or(instance, travelName(src, m, ""))
		}
		if q.turnin != nil && q.turnin.trip != nil && q.turnin.trip.Instance != "" {
			instance = cmp.Or(instance, q.turnin.trip.Instance)
		}
		// A quest the dungeon table links to an instance names it and its zone, whatever the map says.
		if ds, ok := src.(dungeonSource); ok {
			if label, zone := ds.dungeonOf(q.id); label != "" {
				if instance == "" || instance == "another area" || !strings.Contains(instance, "(") {
					instance = label
				}
				if entrance == 0 {
					entrance = zone
				}
			}
		}
		if instance == "another area" {
			instance = "a dungeon"
		}
		park := func(kind, why string) {
			p.Parked = append(p.Parked, parkedOf(q, in.lvl, kind, why))
		}
		switch {
		case q.event || q.festival:
			park(ParkSeasonal, "holiday quest")
			continue
		case tooHigh(q.level, in.lvl, in.reach) && !ready && !deliveryOnly(q):
			// Difficulty only matters for work not yet done: a ready turn-in is free XP at any level.
			park(ParkTooHigh, doableAround(q.level, in.reach))
			continue
		case q.level > 0 && q.level <= GreyLevel(in.lvl) && !ready:
			why := "grey at your level"
			if crowded {
				why += ", worth abandoning"
			}
			park(ParkGrey, why)
			continue
		case instance != "" && !opt.GroupPlan:
			park(ParkDungeon, "needs a group: "+instance)
			continue
		}
		if instance != "" && entrance != 0 {
			m = entrance
		}
		t := byMap[m]
		if t == nil {
			t = &Trip{Map: m, Zone: src.mapName(m), Instance: instance, QuestIDs: []int{}}
			byMap[m] = t
			trips = append(trips, t)
		}
		t.QuestIDs = append(t.QuestIDs, q.id)
		t.XP += opt.XP[q.id]
		if ready {
			t.Ready++
		} else {
			t.Todo++
		}
	}
	sort.Slice(p.Parked, func(i, j int) bool { return p.Parked[i].QuestID < p.Parked[j].QuestID })
	for _, s := range in.later {
		if s.kind == KindPickup && !inLog[s.quests[0]] {
			if t := byMap[s.m]; t != nil {
				t.Pickups += len(s.quests)
			}
		}
	}

	legOf := map[int]Stop{}
	for _, l := range in.legs {
		legOf[l.Map] = l
	}
	zoneOf := map[int]*ZonePlan{}
	for i := range p.Zones {
		zoneOf[p.Zones[i].Map] = &p.Zones[i]
	}
	arrivals := map[int]Arrival{}
	for _, t := range trips {
		slices.Sort(t.QuestIDs)
		t.Reason = reasonOf(t)
		if t.Map == in.cur {
			t.Travel = "Here, once the run is done"
			continue
		}
		arr, ok := Arrival{}, false
		if z := zoneOf[t.Map]; z != nil {
			arr, ok = z.Arrival, true
			t.Travel = z.Travel
		}
		if !ok {
			arr = arrivalFor(c, src, t.Map, nil, p.Start)
			t.Travel = travelLabel(c, src, t.Map, arr, in.cur, in.known)
		}
		arrivals[t.Map] = arr
		if l, ok := legOf[t.Map]; ok {
			t.Travel = l.Label
		}
		t.via = viaOf(t.Travel, arr)
	}
	orderAndRoute(p, c, src, trips, arrivals, legOf, in)
	for i, t := range trips {
		t.Order = i + 1
		p.Trips = append(p.Trips, *t)
	}
	if len(trips) > 0 && trips[0].Map != in.cur {
		t := trips[0]
		a := arrivals[t.Map]
		e := &Exit{Label: t.Travel, Map: t.Map, X: NoPos, Y: NoPos, Via: t.via}
		if a.Known {
			e.X, e.Y = a.X, a.Y
		}
		p.Exit = e
	}
	findHomeless(p, c, src, in.listed)
	addErrands(p, c, src, opt, in)
	p.Total = len(p.Steps)
	linkSteps(p.Steps)
	p.Flights = flightList(c, src, p, in.cur)
}

// findHomeless gives every logged quest that is in no step, trip, parked entry or find entry a find entry
// with the quest's own words: a done quest whose finisher is unknown, or one the data has never heard of.
func findHomeless(p *Plan, c character.Character, src source, listed []*quest) {
	placed := map[int]bool{}
	for _, s := range p.Steps {
		placed[s.QuestID] = true
		for _, id := range s.QuestIDs {
			placed[id] = true
		}
	}
	for _, k := range p.Parked {
		placed[k.QuestID] = true
	}
	for _, f := range p.Find {
		placed[f.QuestID] = true
	}
	hints := map[int]string{}
	for _, q := range listed {
		hints[q.id] = q.hint
	}
	if qs, ok := src.(*questieSource); ok {
		for _, lq := range c.Quests {
			if qq := qs.qx.Quests[lq.QuestID]; qq != nil && qq.Objective != "" {
				hints[lq.QuestID] = qq.Objective
			}
		}
	}
	for _, lq := range c.Quests {
		if placed[lq.QuestID] {
			continue
		}
		hasTrip := false
		for _, t := range p.Trips {
			hasTrip = hasTrip || slices.Contains(t.QuestIDs, lq.QuestID)
		}
		if hasTrip {
			continue
		}
		placed[lq.QuestID] = true
		hint := hints[lq.QuestID]
		for _, o := range lq.Objectives {
			hint = cmp.Or(hint, o.Text)
		}
		hint = cmp.Or(hint, "Find who takes "+cmp.Or(lq.Title, "this quest"))
		p.Find = append(p.Find, FindStop{N: len(p.Find) + 1, QuestID: lq.QuestID, Quest: lq.Title, Label: hint, Hint: hint})
	}
}

// nextAction is the map where the quest's next action happens, and whether it is a ready turn-in:
// a turn-in is at the finisher, open objectives at their own place. 0 when nothing is placed.
func nextAction(q *quest) (m int, ready bool) {
	if q.turnin != nil && q.turnin.ready {
		return q.turnin.m, true
	}
	for _, o := range q.objs {
		if o.m != 0 {
			return o.m, false
		}
	}
	if q.turnin != nil {
		return q.turnin.m, false
	}
	for _, o := range q.unplaced {
		if o.m != 0 {
			return o.m, false
		}
	}
	return 0, false
}

func reasonOf(t *Trip) string {
	var parts []string
	if t.Ready > 0 {
		parts = append(parts, fmt.Sprintf("%d ready to turn in", t.Ready))
	}
	if t.Todo > 0 {
		parts = append(parts, fmt.Sprintf("%d to do", t.Todo))
	}
	return strings.Join(parts, ", ")
}

// viaOf is how the player gets to the zone: by flight, by zeppelin or boat across continents, or on foot.
func viaOf(travel string, a Arrival) string {
	switch {
	case strings.HasPrefix(travel, "Zeppelin"):
		return "zeppelin"
	case strings.HasPrefix(travel, "Boat"):
		return "boat"
	case a.Kind == ArriveFlight || strings.HasPrefix(travel, "Fly"):
		return "flight"
	}
	return "walk"
}
