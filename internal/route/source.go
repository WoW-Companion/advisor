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
)

// questieSource reads quests and coordinates from the player's own QuestieDB. What it reads is used
// at run time for the player's local plan only; nothing here is stored in the repo.
type questieSource struct{ qx *questie.Index }

type pt struct {
	m    int
	x, y float64
	n    int // spawns in the cluster
	name string
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
				out = append(out, pt{m: a.UIMap, x: sp.X, y: sp.Y, n: 1, name: e.Name})
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
			out = append(out, pt{m: p.m, name: p.name})
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
		return &spot{kind: kind, m: p.m, x: p.x, y: p.y, quests: []int{q.ID}, names: []string{q.Name}, label: p.name}
	}
	objectives := func(q *questie.Quest, done []string, ax, ay float64) []*spot {
		var out []*spot
		for _, r := range q.Creatures {
			e, ok := s.entity(r)
			if !ok {
				continue
			}
			skip := false
			for _, d := range done {
				if e.Name != "" && strings.Contains(d, strings.ToLower(e.Name)) {
					skip = true
				}
			}
			if skip {
				continue
			}
			cs := clusters(s.points([]questie.Ref{r}))
			if len(cs) == 0 {
				continue
			}
			// Spawns off the current map only count when none is on it; the biggest nearby cluster
			// wins, a cluster under half the biggest's size is not worth a trip.
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
			p, _ := near(cands, curMap, ax, ay, true)
			out = append(out, &spot{kind: KindObjective, m: p.m, x: p.x, y: p.y, quests: []int{q.ID}, names: []string{q.Name}, label: e.Name})
		}
		return out
	}

	inLog := map[int]bool{}
	for _, lq := range c.Quests {
		inLog[lq.QuestID] = true
		qq := s.qx.Quests[lq.QuestID]
		if qq == nil || (lq.Failed != nil && *lq.Failed) {
			continue
		}
		q := &quest{id: qq.ID, name: qq.Name, level: qq.Level}
		if lq.Level != nil {
			q.level = *lq.Level
		}
		complete := lq.Complete != nil && *lq.Complete
		var done []string
		open := 0
		for _, o := range lq.Objectives {
			if o.Done != nil && *o.Done {
				done = append(done, strings.ToLower(o.Text))
			} else {
				open++
			}
		}
		q.turnin = spotFrom(KindTurnIn, qq, qq.Finishers, x, y, haveXY)
		if !complete && open > 0 {
			ax, ay := x, y
			if q.turnin != nil && !haveXY {
				ax, ay = q.turnin.x, q.turnin.y
			}
			q.objs = objectives(qq, done, ax, ay)
		}
		out = append(out, q)
	}

	// Quests not in the log that could be picked up now, whose giver stands on the player's map.
	if at != nil && curMap != 0 {
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
			q := &quest{id: qq.ID, name: qq.Name, level: qq.Level, pickup: pick}
			q.objs = objectives(qq, nil, pick.x, pick.y)
			q.turnin = spotFrom(KindTurnIn, qq, qq.Finishers, pick.x, pick.y, true)
			out = append(out, q)
		}
	}
	return out
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
