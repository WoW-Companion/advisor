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
)

// Limits of the parked list.
const (
	TooHighMargin = 3 // a quest more than this many levels above the player is parked
	GreyGap       = 8 // a quest this many levels or more below the player is grey
	// FullLogShare is the share of the quest log (3 in 4: 30 of 40) from which a grey quest is "worth abandoning".
	fullLogNum, fullLogDen = 3, 4
)

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
	cost     int
}

// Parked is a logged quest that is in no step and no trip, and why.
type Parked struct {
	QuestID int    `json:"quest_id"`
	Quest   string `json:"quest"`
	Why     string `json:"why"`
	Kind    string `json:"kind"`
}

// entranceSource is a source that knows which zone a dungeon's entrance stands in.
type entranceSource interface {
	// entranceOf is the dungeon's name and the map of the zone its entrance is in (0: unknown), for an instance map.
	entranceOf(m int) (name string, zone int)
}

type sharedInput struct {
	listed []*quest
	later  []*spot
	legs   []Stop
	cur    int
	known  bool
	lvl    int
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
		park := func(kind, why string) {
			p.Parked = append(p.Parked, Parked{QuestID: q.id, Quest: q.name, Why: why, Kind: kind})
		}
		switch {
		case q.event || q.festival:
			park(ParkSeasonal, "holiday quest")
			continue
		case q.level > in.lvl+TooHighMargin:
			park(ParkTooHigh, fmt.Sprintf("level %d", q.level))
			continue
		case q.level > 0 && q.level <= in.lvl-GreyGap && !ready:
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
			t.Travel, t.cost = "Here, once the run is done", 0
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
		t.cost = travelCost(t.Travel, arr)
	}
	sort.SliceStable(trips, func(i, j int) bool {
		a, b := trips[i], trips[j]
		return a.Ready > b.Ready || (a.Ready == b.Ready && (a.XP > b.XP ||
			(a.XP == b.XP && (a.cost < b.cost || (a.cost == b.cost && a.Map < b.Map)))))
	})
	for i, t := range trips {
		t.Order = i + 1
		p.Trips = append(p.Trips, *t)
	}
	if len(trips) > 0 && trips[0].Map != in.cur {
		t := trips[0]
		a := arrivals[t.Map]
		e := &Exit{Label: t.Travel, Map: t.Map, X: NoPos, Y: NoPos, Via: viaOf(t.Travel, a)}
		if a.Known {
			e.X, e.Y = a.X, a.Y
		}
		p.Exit = e
	}
	p.Total = len(p.Steps)
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

// travelCost ranks the way to a zone: a known flight first, then a walk, then a crossing by zeppelin or boat.
func travelCost(travel string, a Arrival) int {
	switch viaOf(travel, a) {
	case "flight":
		return 1
	case "walk":
		return 2
	}
	return 3
}
