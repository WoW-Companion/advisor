package route

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/unlock"
)

// questieSource reads quests and coordinates from the player's own QuestieDB. What it reads is used
// at run time for the player's local plan only; nothing here is stored in the repo.
type questieSource struct {
	qx     *questie.Index
	follow func(use []*quest, cur int, opt Options) []*quest // set by quests
}

type pt struct {
	m    int
	x, y float64
	n    int // spawns in the cluster
	name string
	id   int    // the entity's id; objects are 0, so a stop never says an object is an NPC
	area string // the subzone's name, else the zone's: where the point is, in words
}

func (s *questieSource) mapName(m int) string {
	best := ""
	ids := make([]int, 0, len(s.qx.Areas))
	for id := range s.qx.Areas {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		a := s.qx.Areas[id]
		if a.UIMap == m && a.Zone != "" && (best == "" || a.Sub == "") {
			best = a.Zone
		}
	}
	if best == "" {
		return fmt.Sprintf("map %d", m)
	}
	return best
}

func (s *questieSource) entity(r questie.Ref) (questie.Entity, bool) {
	if r.Object {
		e, ok := s.qx.Objects[r.ID]
		return e, ok
	}
	e, ok := s.qx.Npcs[r.ID]
	return e, ok
}

// points are the spawns of the refs, as map points.
func (s *questieSource) points(refs []questie.Ref) []pt {
	var out []pt
	for _, r := range refs {
		e, ok := s.entity(r)
		if !ok {
			continue
		}
		for _, sp := range e.Spawns {
			if a, ok := s.qx.Areas[sp.Area]; ok && a.UIMap != 0 {
				place := a.Sub
				if place == "" {
					place = a.Zone
				}
				id := r.ID
				if r.Object {
					id = 0
				}
				out = append(out, pt{m: a.UIMap, x: sp.X, y: sp.Y, n: 1, name: e.Name, id: id, area: place})
			}
		}
	}
	return out
}

// near is the point of pts closest to (x, y), preferring map m; ok is false when pts is empty.
func near(pts []pt, m int, x, y float64, haveXY bool) (pt, bool) {
	var best pt
	found := false
	for _, p := range pts {
		better := !found
		if found {
			switch {
			case (p.m == m) != (best.m == m):
				better = p.m == m
			case haveXY:
				better = math.Hypot(p.x-x, p.y-y) < math.Hypot(best.x-x, best.y-y)
			}
		}
		if better {
			best, found = p, true
		}
	}
	return best, found
}

// clusters groups points of one map that lie within ClusterSize of a cluster's first point, and
// returns each cluster's centroid with its size, biggest first.
func clusters(pts []pt) []pt {
	var out []pt
	var firsts []pt
	for _, p := range pts {
		hit := -1
		for i, f := range firsts {
			if f.m == p.m && math.Hypot(f.x-p.x, f.y-p.y) <= ClusterSize {
				hit = i
				break
			}
		}
		if hit < 0 {
			firsts = append(firsts, p)
			out = append(out, pt{m: p.m, name: p.name, id: p.id, area: p.area})
			hit = len(out) - 1
		}
		c := &out[hit]
		c.x, c.y, c.n = c.x+p.x, c.y+p.y, c.n+1
	}
	for i := range out {
		out[i].x /= float64(out[i].n)
		out[i].y /= float64(out[i].n)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].n > out[j].n })
	return out
}

func (s *questieSource) quests(c character.Character, at *questie.Where) []*quest {
	var curMap int
	var x, y float64
	haveXY := at != nil
	if at != nil {
		x, y = at.X, at.Y
		if at.MapID != nil {
			curMap = *at.MapID
		}
	}
	var out []*quest
	spotFrom := func(kind string, q *questie.Quest, refs []questie.Ref, ax, ay float64, haveA bool) *spot {
		p, ok := near(s.points(refs), curMap, ax, ay, haveA)
		if !ok {
			return nil
		}
		return &spot{kind: kind, m: p.m, x: p.x, y: p.y, quests: []int{q.ID}, names: []string{q.Name}, label: p.name, npc: p.id, place: p.area}
	}
	// best is the spot for an objective whose spawns are pts: spawns off the current map only count
	// when none is on it; the biggest nearby cluster wins, a cluster under half the biggest's size is
	// not worth a trip.
	best := func(pts []pt, ax, ay float64) (pt, bool) {
		cs := clusters(pts)
		if len(cs) == 0 {
			return pt{}, false
		}
		var cands []pt
		for _, k := range cs {
			if k.m == curMap {
				cands = append(cands, k)
			}
		}
		if len(cands) == 0 {
			cands = cs
		}
		cands = cands[:1+len(filterBig(cands[1:], cands[0].n))]
		return near(cands, curMap, ax, ay, true)
	}
	// objectives resolves q's objectives to spots; an objective with no location is returned as an
	// unplaced spot. done are the lower-cased texts of finished objectives; open the texts of the
	// unfinished ones (nil: unknown, as for a quest not yet in the log).
	objectives := func(q *questie.Quest, done, open []string, ax, ay float64) (found, unplaced []*spot) {
		isDone := func(name string) bool {
			for _, d := range done {
				if name != "" && strings.Contains(d, strings.ToLower(name)) {
					return true
				}
			}
			return false
		}
		textOf := func(name string) string {
			for _, o := range open {
				if name != "" && strings.Contains(strings.ToLower(o), strings.ToLower(name)) {
					return o
				}
			}
			return name
		}
		covered := map[string]bool{}
		add := func(verb, name string, refs []questie.Ref) {
			if name == "" || isDone(name) {
				return
			}
			// A quest in the log lists what is still open: an objective it does not mention is finished
			// (or not this quest's to do), and is not a step.
			if len(open) > 0 && textOf(name) == name {
				return
			}
			covered[strings.ToLower(name)] = true
			mk := func() *spot {
				return &spot{kind: KindObjective, quests: []int{q.ID}, names: []string{q.Name}, label: objectiveLabel(verb, textOf(name))}
			}
			if p, ok := best(s.points(refs), ax, ay); ok {
				sp := mk()
				sp.m, sp.x, sp.y, sp.place = p.m, p.x, p.y, p.area
				found = append(found, sp)
				return
			}
			sp := mk()
			sp.unplaced, sp.label = true, sp.label+" (location unknown)"
			unplaced = append(unplaced, sp)
		}
		for _, r := range q.Creatures {
			if e, ok := s.entity(r); ok {
				add(creatureVerb(e), e.Name, []questie.Ref{r})
			}
		}
		for _, id := range q.Items {
			it, ok := s.qx.Items[id]
			if !ok {
				continue
			}
			var refs []questie.Ref
			for _, n := range it.Npcs {
				refs = append(refs, questie.Ref{ID: n})
			}
			for _, o := range it.Objects {
				refs = append(refs, questie.Ref{Object: true, ID: o})
			}
			add("Collect", it.Name, refs)
		}
		for _, c := range q.Credits {
			var refs []questie.Ref
			for _, n := range c.NPCs {
				refs = append(refs, questie.Ref{ID: n})
			}
			add("", c.Name, refs)
		}
		// An unfinished objective the quest's data does not explain has no place to go.
		for _, o := range open {
			hit := false
			for name := range covered {
				hit = hit || strings.Contains(strings.ToLower(o), name)
			}
			if !hit {
				unplaced = append(unplaced, &spot{kind: KindObjective, unplaced: true, quests: []int{q.ID}, names: []string{q.Name}, label: objectiveLabel("", o) + " (location unknown)"})
			}
		}
		return found, unplaced
	}

	inLog := map[int]bool{}
	for _, lq := range c.Quests {
		inLog[lq.QuestID] = true
		qq := s.qx.Quests[lq.QuestID]
		if qq == nil || (lq.Failed != nil && *lq.Failed) {
			continue
		}
		q := &quest{id: qq.ID, name: qq.Name, level: qq.Level, must: classQuestOf(c, qq)}
		if lq.Level != nil {
			q.level = *lq.Level
		}
		ready := character.Quest(lq).Ready()
		complete := ready
		var done, open []string
		for _, o := range lq.Objectives {
			if o.ObjectiveDone() {
				done = append(done, strings.ToLower(o.Text))
			} else {
				open = append(open, o.Text)
			}
		}
		q.turnin = spotFrom(KindTurnIn, qq, qq.Finishers, x, y, haveXY)
		if !complete && (len(open) > 0 || lq.Observed) {
			ax, ay := x, y
			if q.turnin != nil && !haveXY {
				ax, ay = q.turnin.x, q.turnin.y
			}
			q.objs, q.unplaced = objectives(qq, done, open, ax, ay)
		}
		// A turn-in is for a quest that is done. When the capture does not say so and no objective step
		// explains what is left, the quest is listed under Find and its turn-in withheld.
		if !ready && len(q.objs) == 0 && len(q.unplaced) == 0 {
			q.turnin = nil
			q.unplaced = []*spot{{kind: KindObjective, unplaced: true, quests: []int{qq.ID}, names: []string{qq.Name}, label: "Finish " + qq.Name + " (location unknown)"}}
		}
		out = append(out, q)
	}

	// Quests not in the log that could be picked up now, whose giver stands on the player's map.
	// Without the quest log nothing says what is already in it, so no pick-up is offered.
	if at != nil && curMap != 0 && (c.QuestsCaptured || len(c.Quests) > 0) {
		who := advice.WhoOf(c)
		ids := make([]int, 0, len(s.qx.Quests))
		for id := range s.qx.Quests {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		for _, id := range ids {
			qq := s.qx.Quests[id]
			if inLog[id] || !qq.Eligible(who) || items.Unobtainable(qq.Name) {
				continue
			}
			pick := spotFrom(KindPickup, qq, qq.Starters, x, y, true)
			if pick == nil || pick.m != curMap {
				continue
			}
			q := &quest{id: qq.ID, name: qq.Name, level: qq.Level, pickup: pick, must: classQuestOf(c, qq)}
			q.objs, q.unplaced = objectives(qq, nil, nil, pick.x, pick.y)
			q.turnin = spotFrom(KindTurnIn, qq, qq.Finishers, pick.x, pick.y, true)
			out = append(out, q)
		}
	}
	s.follow = func(use []*quest, cur int, opt Options) []*quest {
		who := advice.WhoOf(c)
		tspot := map[int]*spot{}
		var planned []int
		for _, q := range use {
			if q.turnin != nil {
				planned = append(planned, q.id)
				tspot[q.id] = q.turnin
			}
		}
		found := unlock.Find(unlock.Input{Qx: s.qx, Who: who, Planned: planned, Lines: opt.Lines, Obs: opt.Obs,
			Depth: opt.Depth, OnMap: cur, Skip: items.Unobtainable})
		type ranked struct {
			q    *quest
			dist float64
		}
		var rs []ranked
		for _, f := range found {
			qq := s.qx.Quests[f.ID]
			if qq == nil || qq.Level > *c.Level+LevelMargin {
				continue
			}
			ax, ay, haveA := x, y, haveXY
			for _, a := range f.UnlockedBy {
				if t := tspot[a]; t != nil {
					ax, ay, haveA = t.x, t.y, true
					break
				}
			}
			pick := spotFrom(KindPickup, qq, qq.Starters, ax, ay, haveA)
			if pick == nil && f.Start != nil {
				pick = &spot{kind: KindPickup, m: f.Start.Map, x: f.Start.X, y: f.Start.Y, quests: []int{qq.ID}, names: []string{qq.Name}, label: f.Start.NPC}
			}
			if pick == nil || pick.m != cur {
				continue
			}
			q := &quest{id: qq.ID, name: qq.Name, level: qq.Level, pickup: pick, must: classQuestOf(c, qq),
				locked: true, depth: f.Depth, unlockedBy: f.UnlockedBy, chainName: f.Chain, observed: f.Observed}
			q.objs, q.unplaced = objectives(qq, nil, nil, pick.x, pick.y)
			if len(q.unplaced) == 0 {
				q.turnin = spotFrom(KindTurnIn, qq, qq.Finishers, pick.x, pick.y, true)
			}
			if q.turnin != nil {
				tspot[q.id] = q.turnin
			}
			rs = append(rs, ranked{q, math.Hypot(pick.x-ax, pick.y-ay)})
		}
		sort.SliceStable(rs, func(i, j int) bool {
			if rs[i].q.depth != rs[j].q.depth {
				return rs[i].q.depth < rs[j].q.depth
			}
			return rs[i].dist < rs[j].dist
		})
		var out []*quest
		for _, r := range rs {
			out = append(out, r.q)
		}
		return out
	}
	return out
}

func (s *questieSource) followUps(use []*quest, cur int, opt Options) []*quest {
	if s.follow == nil {
		return nil
	}
	return s.follow(use, cur, opt)
}

// classQuestOf reports whether qq is a class quest for c's class: those stops are must-include.
func classQuestOf(c character.Character, qq *questie.Quest) bool {
	return advice.WhoOf(c).ClassID > 0 && qq.ClassQuest()
}

// filterBig keeps the leading clusters at least half the size of the biggest.
func filterBig(cs []pt, biggest int) []pt {
	var out []pt
	for _, c := range cs {
		if c.n*2 >= biggest {
			out = append(out, c)
		}
	}
	return out
}

// npcFlags are the bits of an NPC's npcFlags the planner reads.
const (
	flagQuestGiver = 0x2
	flagTrainer    = 0x10
	flagVendor     = 0x80
	flagRepair     = 0x1000
)

// friendly reports whether e is an NPC to talk to, not a mob to fight: Questie names a faction it is
// friendly to, or it gives quests.
func friendly(e questie.Entity) bool { return e.Faction != "" || e.Flags&flagQuestGiver != 0 }

// creatureVerb is the verb for a creature objective: a friendly NPC is talked to, never killed.
func creatureVerb(e questie.Entity) string {
	if friendly(e) {
		return "Talk to"
	}
	return "Kill"
}

// npc is the entity of an NPC id (the stop's NPC), for the "here" blocks.
func (s *questieSource) npc(id int) (questie.Entity, bool) {
	e, ok := s.qx.Npcs[id]
	return e, ok
}
