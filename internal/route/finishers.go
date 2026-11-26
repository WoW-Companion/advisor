package route

import (
	"cmp"
	"math"
	"sort"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
)

// Caps on the Plan.lua "finishers" table (docs/plan-format.md, schema 1.4).
const (
	MaxFinisherQuests = 40 // logged quests listed
	MaxFinisherSpawns = 3  // turn-in places per quest
)

// Finisher is one place a logged quest can be handed in: the addon compares it with the player's
// position and the log's ready state live, between reloads.
type Finisher struct {
	QuestID int     `json:"quest_id"`
	Quest   string  `json:"quest"`
	NPC     string  `json:"npc,omitempty"`
	Object  string  `json:"object,omitempty"`
	Map     int     `json:"map"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Place   string  `json:"place"`
}

// finisherPt is one spawn of a quest's finisher, before ranking.
type finisherPt struct {
	object bool
	name   string
	m      int
	x, y   float64
	place  string
	zone   string
}

// finisherSource is a source that knows where a quest is handed in.
type finisherSource interface {
	finisherPts(questID int) []finisherPt
}

// buildFinishers lists, for every quest in the log that can still be handed in, up to MaxFinisherSpawns
// finisher spawns on the player's continent: those nearest the quest's objective area first (the player's
// position for a quest with no placed objective). A spawn with no known place is skipped.
func buildFinishers(c character.Character, src source, listed []*quest, zone string) []Finisher {
	fs, ok := src.(finisherSource)
	if !ok {
		return nil
	}
	cont := ""
	var curMap int
	var px, py float64
	havePos := false
	if pos := c.LastKnownPosition(); pos != nil {
		cont = learn.ContinentOf(pos.Zone)
		if pos.MapID != nil {
			curMap, px, py, havePos = *pos.MapID, pos.X, pos.Y, true
		}
	}
	if cont == "" {
		cont = learn.ContinentOf(zone)
	}
	area := map[int]*spot{}
	names := map[int]string{}
	for _, q := range listed {
		names[q.id] = q.name
		if len(q.objs) > 0 {
			if _, dup := area[q.id]; !dup {
				area[q.id] = q.objs[0]
			}
		}
	}
	var out []Finisher
	seen := map[int]bool{}
	for _, lq := range c.Quests {
		if len(seen) >= MaxFinisherQuests {
			break
		}
		if seen[lq.QuestID] || (lq.Failed != nil && *lq.Failed) {
			continue
		}
		var pts []finisherPt
		for _, p := range fs.finisherPts(lq.QuestID) {
			c2 := learn.ContinentOf(p.zone)
			if p.place == "" || (cont != "" && c2 != "" && c2 != cont) {
				continue
			}
			pts = append(pts, p)
		}
		if len(pts) == 0 {
			continue
		}
		seen[lq.QuestID] = true
		rm, rx, ry, have := curMap, px, py, havePos
		if a := area[lq.QuestID]; a != nil {
			rm, rx, ry, have = a.m, a.x, a.y, true
		}
		dist := func(p finisherPt) float64 {
			if !have {
				return 0
			}
			if p.m != rm {
				return 1e6
			}
			return math.Hypot(p.x-rx, p.y-ry)
		}
		sort.SliceStable(pts, func(i, j int) bool {
			if d := cmp.Compare(dist(pts[i]), dist(pts[j])); d != 0 {
				return d < 0
			}
			if pts[i].m != pts[j].m {
				return pts[i].m < pts[j].m
			}
			if pts[i].x != pts[j].x {
				return pts[i].x < pts[j].x
			}
			return pts[i].y < pts[j].y
		})
		if len(pts) > MaxFinisherSpawns {
			pts = pts[:MaxFinisherSpawns]
		}
		name := cmp.Or(names[lq.QuestID], lq.Title)
		for _, p := range pts {
			f := Finisher{QuestID: lq.QuestID, Quest: name, Map: p.m, X: round1(p.x), Y: round1(p.y), Place: p.place}
			if p.object {
				f.Object = p.name
			} else {
				f.NPC = p.name
			}
			out = append(out, f)
		}
	}
	return out
}

// finisherPts are every spawn of the quest's finishers with an area the dataset knows.
func (s *questieSource) finisherPts(questID int) []finisherPt {
	qq := s.qx.Quests[questID]
	if qq == nil {
		return nil
	}
	var out []finisherPt
	for _, r := range qq.Finishers {
		e, ok := s.entity(r)
		if !ok {
			continue
		}
		for _, sp := range e.Spawns {
			a, ok := s.qx.Areas[sp.Area]
			if !ok || a.UIMap == 0 {
				continue
			}
			out = append(out, finisherPt{object: r.Object, name: e.Name, m: a.UIMap, x: sp.X, y: sp.Y, place: cmp.Or(a.Sub, a.Zone), zone: a.Zone})
		}
	}
	return out
}
