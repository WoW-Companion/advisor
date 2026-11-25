package route

import (
	"sort"
	"strings"
)

// Guide is what one quest needs once it is accepted: its objectives, its turn-in and where it sits on
// the route. It is data for the addon (Plan.lua 1.2), worked out ahead of time because the game only
// writes the app's files on a reload, and the addon must never ask for one.
type Guide struct {
	QuestID    int         `json:"quest_id"`
	Quest      string      `json:"quest"`
	Class      bool        `json:"class,omitempty"`      // a class quest
	Steps      []int       `json:"steps,omitempty"`      // the route steps that serve the quest, in order
	Objectives []GuideItem `json:"objectives,omitempty"` // what to do once accepted
	TurnIn     *GuideItem  `json:"turn_in,omitempty"`    // where to hand it in; absent when withheld or unknown
	After      string      `json:"after"`                // a short line pointing to the next planned step
}

// GuideItem is one thing to do and where. With no position known only Zone is set.
type GuideItem struct {
	Text string  `json:"text"`
	Map  int     `json:"map,omitempty"`
	X    float64 `json:"x,omitempty"`
	Y    float64 `json:"y,omitempty"`
	Zone string  `json:"zone,omitempty"`
}

// unknownTag is the suffix the planner puts on an objective it could not place.
const unknownTag = " (location unknown)"

func itemOf(s *spot, zone func(int) string, fallback string) GuideItem {
	text := strings.TrimSuffix(s.label, unknownTag)
	if s.kind == KindTurnIn {
		text = stopOf(0, s).Label
	}
	if s.unplaced || s.m == 0 {
		return GuideItem{Text: text, Zone: fallback}
	}
	return GuideItem{Text: text, Map: s.m, X: round1(s.x), Y: round1(s.y), Zone: zone(s.m)}
}

// guides writes one Guide per listed quest, sorted by quest id. steps are the route's steps.
func guides(all []*quest, steps []Stop, planZone string, zone func(int) string) []Guide {
	seen := map[int]bool{}
	var out []Guide
	for _, q := range all {
		if seen[q.id] {
			continue
		}
		seen[q.id] = true
		g := Guide{QuestID: q.id, Quest: q.name, Class: q.must}
		last := -1
		for i, st := range steps {
			for _, id := range st.QuestIDs {
				if id == q.id {
					g.Steps = append(g.Steps, st.N)
					last = i
					break
				}
			}
		}
		for _, o := range q.objs {
			g.Objectives = append(g.Objectives, itemOf(o, zone, planZone))
		}
		for _, o := range q.unplaced {
			g.Objectives = append(g.Objectives, itemOf(o, zone, planZone))
		}
		if q.turnin != nil {
			t := itemOf(q.turnin, zone, planZone)
			g.TurnIn = &t
		}
		// After this: the step that follows the quest's last step; for a quest off the route, the
		// route's first step.
		switch next := last + 1; {
		case next < len(steps):
			g.After = "After this: " + steps[next].Label + "."
		case last < 0 && len(steps) > 0:
			g.After = "After this: " + steps[0].Label + "."
		default:
			g.After = "After this: nothing else is planned here."
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].QuestID < out[j].QuestID })
	return out
}
