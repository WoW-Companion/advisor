package questie

import (
	"cmp"
	"fmt"
	"math"
	"math/bits"
	"sort"
	"strings"
)

// Where is the character's last known place: the uiMapID, or the zone name without one; X and Y are map percent.
type Where struct {
	MapID *int
	Zone  string
	X, Y  float64
}

type Place struct {
	ID   int    // the NPC's or object's id
	Name string // the NPC or object
	Sub  string // subzone, when the area has one
	Zone string
	X, Y float64
	Dist float64 // distance from the character in map percent; -1 when unknown
}

func (p Place) String() string {
	s := p.Name
	if loc := cmp.Or(p.Sub, p.Zone); loc != "" {
		s += ", " + loc
	}
	return s + fmt.Sprintf(" (%.1f, %.1f)", p.X, p.Y)
}

type Who struct {
	Level           int
	ClassID, RaceID int // 0 when not captured: that rule is not checked
	Done, InLog     map[int]bool
	Below, Above    int
}

// Offer is a quest that can be picked up, with its nearest giver.
type Offer struct {
	Quest *Quest
	Giver Place
	Diff  int
	Class bool // a class quest the character's class can take
}

func (ix *Index) entity(r Ref) (Entity, bool) {
	if r.Object {
		e, ok := ix.Objects[r.ID]
		return e, ok
	}
	e, ok := ix.Npcs[r.ID]
	return e, ok
}

func (ix *Index) onMap(area int, w Where) bool {
	a, ok := ix.Areas[area]
	if !ok {
		return false
	}
	if w.MapID != nil {
		return a.UIMap == *w.MapID
	}
	return w.Zone != "" && (strings.EqualFold(a.Zone, w.Zone) || strings.EqualFold(a.Sub, w.Zone))
}

func (ix *Index) place(e Entity, s Spawn, w Where) Place {
	a := ix.Areas[s.Area]
	p := Place{Name: e.Name, Sub: a.Sub, Zone: a.Zone, X: s.X, Y: s.Y, Dist: -1}
	if ix.onMap(s.Area, w) {
		p.Dist = math.Hypot(s.X-w.X, s.Y-w.Y)
	}
	return p
}

// nearest is the closest spawn of any of the refs on the character's map; with no position it is
// the first spawn found. ok is false when none is on the map.
func (ix *Index) nearest(refs []Ref, w Where) (Place, bool) {
	best, found := Place{Dist: -1}, false
	consider := func(p Place) {
		if !found || (p.Dist >= 0 && (best.Dist < 0 || p.Dist < best.Dist)) {
			best, found = p, true
		}
	}
	for _, r := range refs {
		e, ok := ix.entity(r)
		if !ok {
			continue
		}
		for _, s := range e.Spawns {
			if ix.onMap(s.Area, w) {
				pl := ix.place(e, s, w)
				pl.ID = r.ID
				consider(pl)
			}
		}
	}
	return best, found
}

func anyIn(ids []int, set map[int]bool) bool {
	for _, id := range ids {
		if set[id] {
			return true
		}
	}
	return false
}

func allIn(ids []int, set map[int]bool) bool {
	for _, id := range ids {
		if !set[id] {
			return false
		}
	}
	return true
}

// MaxClassQuestClasses is the most classes a quest's mask may name for it to count as a class quest.
const MaxClassQuestClasses = 3

// ClassQuest reports whether the quest is limited to one class or a few (its class mask names at
// most MaxClassQuestClasses of them), not open to everyone.
func (q *Quest) ClassQuest() bool {
	return q.Classes != 0 && bits.OnesCount64(q.Classes) <= MaxClassQuestClasses
}

// Eligible reports whether the quest could be picked up now: level, race, class, prerequisites,
// not done, not in the log, not a placeholder, no requirement this reader cannot check.
func (q *Quest) Eligible(w Who) bool {
	if q.Name == "" || q.Restricted || len(q.Starters) == 0 || q.Parent != 0 {
		return false
	}
	if w.Done[q.ID] || w.InLog[q.ID] {
		return false
	}
	if q.Level < w.Level-w.Below || q.Level > w.Level+w.Above || q.MinLevel > w.Level || (q.MaxLevel > 0 && w.Level > q.MaxLevel) {
		return false
	}
	if q.Races != 0 && w.RaceID > 0 && q.Races&(1<<(w.RaceID-1)) == 0 {
		return false
	}
	if q.Classes != 0 && w.ClassID > 0 && q.Classes&(1<<(w.ClassID-1)) == 0 {
		return false
	}
	if len(q.PreSingle) > 0 && !anyIn(q.PreSingle, w.Done) {
		return false
	}
	if !allIn(q.PreGroup, w.Done) {
		return false
	}
	if anyIn(q.Exclusive, w.Done) || anyIn(q.Exclusive, w.InLog) {
		return false
	}
	for _, id := range []int{q.Next, q.Until} {
		if id != 0 && (w.Done[id] || w.InLog[id]) {
			return false
		}
	}
	return q.Disabled == 0 || !w.InLog[q.Disabled]
}

// Available lists the quests the character could pick up, whose giver stands on their map: best
// level fit first, then nearest the character. A character with no position gets none (nothing can
// be called nearby).
func (ix *Index) Available(w Who, at Where, skip func(name string) bool) []Offer {
	if at.MapID == nil && at.Zone == "" {
		return nil
	}
	var out []Offer
	for _, q := range ix.Quests {
		if !q.Eligible(w) || (skip != nil && skip(q.Name)) {
			continue
		}
		p, ok := ix.nearest(q.Starters, at)
		if !ok {
			continue
		}
		d := q.Level - w.Level
		if d < 0 {
			d = -d
		}
		out = append(out, Offer{q, p, d, w.ClassID > 0 && q.ClassQuest()})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		switch {
		case a.Class != b.Class:
			return a.Class
		case a.Diff != b.Diff:
			return a.Diff < b.Diff
		case (a.Giver.Dist < 0) != (b.Giver.Dist < 0):
			return b.Giver.Dist < 0
		case a.Giver.Dist != b.Giver.Dist:
			return a.Giver.Dist < b.Giver.Dist
		}
		return a.Quest.ID < b.Quest.ID
	})
	return out
}

// Locate is where to go next for a quest in the log: the turn-in when complete, else the nearest
// objective, else the turn-in. ok is false when nothing is on the character's map.
func (ix *Index) Locate(questID int, complete bool, at Where) (p Place, objective, ok bool) {
	q := ix.Quests[questID]
	if q == nil {
		return Place{}, false, false
	}
	if !complete {
		if p, ok := ix.nearest(q.Creatures, at); ok {
			return p, true, true
		}
	}
	p, ok = ix.nearest(q.Finishers, at)
	return p, false, ok
}

// Describe names the NPCs or objects in refs with the zone of each one's first known spawn, such as
// "Elder Mira (Tidepool Cove)". It needs no character position.
func (ix *Index) Describe(refs []Ref) string {
	var out []string
	for _, r := range refs {
		e, ok := ix.entity(r)
		if !ok || e.Name == "" {
			continue
		}
		s := e.Name
		if len(e.Spawns) > 0 {
			a := ix.Areas[e.Spawns[0].Area]
			if loc := cmp.Or(a.Sub, a.Zone); loc != "" {
				s += " (" + loc + ")"
			}
		}
		out = append(out, s)
		if len(out) == 3 {
			break
		}
	}
	return strings.Join(out, " or ")
}

// FindByName returns the ids of quests named name: the exact (case-insensitive) matches, else those
// whose name contains it. Ids are ascending.
func (ix *Index) FindByName(name string) []int {
	name = strings.ToLower(strings.TrimSpace(name))
	var exact, part []int
	if name == "" {
		return nil
	}
	for id, q := range ix.Quests {
		n := strings.ToLower(q.Name)
		switch {
		case n == name:
			exact = append(exact, id)
		case strings.Contains(n, name):
			part = append(part, id)
		}
	}
	if len(exact) > 0 {
		part = exact
	}
	sort.Ints(part)
	return part
}

// NpcPlace is where the NPC stands: its spawn nearest the character on the character's map, else
// its first spawn anywhere. ok is false when Questie holds no such NPC or no spawn for it.
func (ix *Index) NpcPlace(id int, at Where) (Place, bool) {
	if ix == nil {
		return Place{}, false
	}
	e, ok := ix.Npcs[id]
	if !ok || len(e.Spawns) == 0 {
		return Place{}, false
	}
	if p, ok := ix.nearest([]Ref{{ID: id}}, at); ok {
		return p, true
	}
	p := ix.place(e, e.Spawns[0], at)
	p.ID = id
	return p, true
}

// NpcFaction is the NPC's "friendly to" field: "A", "H" or "AH"; empty when unknown.
func (ix *Index) NpcFaction(id int) string {
	if ix == nil {
		return ""
	}
	return ix.Npcs[id].Faction
}
