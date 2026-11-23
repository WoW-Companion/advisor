// Package unlock finds the follow-up quests that turning in a plan's quests makes available. The
// links come from the player's own Questie data (prerequisites, next-in-chain), from the dataset's
// quest_lines order when Questie is silent, and from unlocks observed in game. Nothing here guesses a
// giver or a position. The rules are in docs/plan-format.md.
package unlock

import (
	"sort"

	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// DefaultDepth is how many links ahead a plan looks.
const DefaultDepth = 2

// Lines gives the quest after questID in its quest line (0 when none) and the line's name.
type Lines func(questID int) (next int, line string)

// FromDataset reads quest_lines from ds; nil when ds is nil.
func FromDataset(ds store.DataStore) Lines {
	if ds == nil {
		return nil
	}
	return func(id int) (int, string) {
		c := quests.Of(ds, int64(id))
		if !c.Found {
			return 0, ""
		}
		return int(c.NextQuestID), c.Line
	}
}

// Follow is a quest the plan's turn-ins unlock.
type Follow struct {
	ID         int
	Depth      int    // 1: unlocked by a planned quest; 2: by a depth-1 follow-up
	UnlockedBy []int  // the turn-ins that unlock it, sorted
	Chain      string // the quest line's name, when known
	Observed   bool   // the link or the start comes from an in-game observation
	Start      *Start // the observed start, when Questie does not give one
}

// Start is a place seen in game where a quest was offered.
type Start struct {
	NPC  string
	Map  int
	X, Y float64
}

// Input is what Find needs.
type Input struct {
	Qx      *questie.Index // may be nil: then only the lines and observations give links, and no quest can be named
	Who     questie.Who    // Done and InLog are the facts at capture (with observed events applied)
	Planned []int          // quests whose turn-in the plan makes
	Lines   Lines          // may be nil
	Obs     []Obs          // learned unlocks, may be nil
	Depth   int            // 0: DefaultDepth
	OnMap   int            // keep a follow-up only when its giver stands on this map; 0: any
	Skip    func(name string) bool
}

// Obs is an unlock learned in game: after A was turned in, B was offered by NPC at the place.
type Obs struct {
	From, To int
	NPC      string
	Map      int
	X, Y     float64
	Count    int
	Last     int64
}

func copyBool(m map[int]bool) map[int]bool {
	out := make(map[int]bool, len(m))
	for k := range m {
		out[k] = true
	}
	return out
}

func startMaps(qx *questie.Index, q *questie.Quest) map[int]bool {
	out := map[int]bool{}
	if qx == nil {
		return out
	}
	for _, r := range q.Starters {
		var e questie.Entity
		var ok bool
		if r.Object {
			e, ok = qx.Objects[r.ID]
		} else {
			e, ok = qx.Npcs[r.ID]
		}
		if !ok {
			continue
		}
		for _, sp := range e.Spawns {
			if a, ok := qx.Areas[sp.Area]; ok && a.UIMap != 0 {
				out[a.UIMap] = true
			}
		}
	}
	return out
}

// Find lists the follow-ups, shallowest first, then by quest id. A follow-up is kept only when every
// prerequisite is met by the planned turn-ins (or is done), its level fits the same band as an
// available quest, it is not exclusive with a planned or done quest, it is not a breadcrumb for a
// quest already done or planned, and its start is known.
func Find(in Input) []Follow {
	depthMax := in.Depth
	if depthMax <= 0 {
		depthMax = DefaultDepth
	}
	if in.Qx == nil {
		return nil
	}
	// preds[B] are the quests whose turn-in can unlock B.
	preds := map[int]map[int]bool{}
	add := func(a, b int) {
		if a == 0 || b == 0 || a == b {
			return
		}
		if preds[b] == nil {
			preds[b] = map[int]bool{}
		}
		preds[b][a] = true
	}
	for id, q := range in.Qx.Quests {
		for _, p := range q.PreSingle {
			add(p, id)
		}
		for _, p := range q.PreGroup {
			add(p, id)
		}
		add(id, q.Next)
	}
	chainOf := map[int]string{}
	lineLink := map[[2]int]bool{}
	obsOf := map[[2]int]Obs{}
	for _, o := range in.Obs {
		add(o.From, o.To)
		obsOf[[2]int{o.From, o.To}] = o
	}
	frontier := append([]int(nil), in.Planned...)
	sort.Ints(frontier)
	seenLine := map[int]bool{}
	var scanLines func(ids []int)
	scanLines = func(ids []int) {
		if in.Lines == nil {
			return
		}
		for _, a := range ids {
			if seenLine[a] {
				continue
			}
			seenLine[a] = true
			if next, line := in.Lines(a); next != 0 {
				add(a, next)
				lineLink[[2]int{a, next}] = true
				if line != "" {
					chainOf[next] = line
					if chainOf[a] == "" {
						chainOf[a] = line
					}
				}
			}
		}
	}
	depth := map[int]int{}
	t := copyBool(in.Who.Done) // turned in by the time the quest is picked up
	for _, id := range in.Planned {
		depth[id] = 0
		t[id] = true
	}
	var out []Follow
	for d := 1; d <= depthMax; d++ {
		scanLines(frontier)
		cands := map[int]bool{}
		for b, ps := range preds {
			for _, a := range frontier {
				if ps[a] {
					cands[b] = true
				}
			}
		}
		ids := make([]int, 0, len(cands))
		for b := range cands {
			ids = append(ids, b)
		}
		sort.Ints(ids)
		var next []int
		for _, b := range ids {
			if _, planned := depth[b]; planned || in.Who.Done[b] || in.Who.InLog[b] {
				continue
			}
			q := in.Qx.Quests[b]
			if q == nil || (in.Skip != nil && in.Skip(q.Name)) {
				continue
			}
			var by []int
			for a := range preds[b] {
				if _, ok := depth[a]; ok {
					by = append(by, a)
				}
			}
			sort.Ints(by)
			f := Follow{ID: b, Depth: d, UnlockedBy: by}
			// The start: Questie's giver, else an observed one.
			cq := q
			maps := startMaps(in.Qx, q)
			for _, a := range by {
				if o, ok := obsOf[[2]int{a, b}]; ok && o.Map != 0 {
					if len(maps) == 0 {
						f.Start = &Start{NPC: o.NPC, Map: o.Map, X: o.X, Y: o.Y}
						f.Observed = true
						maps[o.Map] = true
						cp := *q
						cp.Starters = []questie.Ref{{ID: -1}}
						cq = &cp
					}
					break
				}
			}
			if len(maps) == 0 || (in.OnMap != 0 && !maps[in.OnMap]) {
				continue
			}
			// A quest with prerequisites that is available without the planned turn-ins is a plain
			// pick-up. One with none is a follow-up only through a chain, line or observed link.
			now := in.Who
			if len(q.PreSingle)+len(q.PreGroup) > 0 && cq.Eligible(now) {
				continue
			}
			w := in.Who
			w.Done = t
			if !cq.Eligible(w) {
				continue
			}
			f.Chain = chainOf[b]
			if f.Chain == "" {
				for _, a := range by {
					if chainOf[a] != "" {
						f.Chain = chainOf[a]
						break
					}
				}
			}
			for _, a := range by {
				if _, ok := obsOf[[2]int{a, b}]; ok && !lineLink[[2]int{a, b}] && !questieLink(in.Qx, a, b) {
					f.Observed = true
				}
			}
			out = append(out, f)
			next = append(next, b)
		}
		for _, f := range out {
			if f.Depth == d {
				depth[f.ID] = d
				t[f.ID] = true
			}
		}
		frontier = next
		if len(next) == 0 {
			break
		}
	}
	return out
}

func questieLink(qx *questie.Index, a, b int) bool {
	q := qx.Quests[b]
	if q == nil {
		return false
	}
	for _, p := range append(append([]int(nil), q.PreSingle...), q.PreGroup...) {
		if p == a {
			return true
		}
	}
	if qa := qx.Quests[a]; qa != nil && qa.Next == b {
		return true
	}
	return false
}
