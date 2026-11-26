package route

import (
	"math"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/questie"
)

// NearbyRadius is how far (map percent) from an NPC a quest giver still counts as "nearby".
const NearbyRadius = 12.0

// Here is what the plan knows about one NPC stop: what can be done there, what each quest involving
// the NPC still needs and where, and quests available nearby that are not on the route. It is data for
// the addon (Plan.lua 1.3): the addon shows it when the player stands at the NPC.
type Here struct {
	NPC    string      `json:"npc"`
	NPCID  int         `json:"npc_id,omitempty"`
	Map    int         `json:"map"`
	X      float64     `json:"x"`
	Y      float64     `json:"y"`
	Place  string      `json:"place,omitempty"`
	Steps  []int       `json:"steps"`         // the route steps at this NPC
	Can    []string    `json:"can,omitempty"` // "turn in", "pick up", "train", "buy and sell", "repair"
	Quests []HereQuest `json:"quests,omitempty"`
	Nearby []Nearby    `json:"nearby,omitempty"`
}

// HereQuest is one quest involving the NPC: its role here and what is still missing.
type HereQuest struct {
	QuestID int         `json:"quest_id"`
	Quest   string      `json:"quest"`
	Role    string      `json:"role"` // "turnin" or "pickup"
	Text    string      `json:"text"` // the line to show
	Missing []GuideItem `json:"missing,omitempty"`
}

// Nearby is a quest available close to the NPC that is not on the route, with its guidance.
type Nearby struct {
	QuestID  int     `json:"quest_id"`
	Quest    string  `json:"quest"`
	Giver    string  `json:"giver"`
	Map      int     `json:"map"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Guidance Guide   `json:"guidance"`
}

// npcSource is a source that knows an NPC's service flags.
type npcSource interface {
	npc(id int) (questie.Entity, bool)
}

// missingText is "Kill Wolf (Windsong Lake)": an open objective with where it is done.
func missingText(it GuideItem) string {
	where := it.Place
	if where == "" {
		where = it.Zone
	}
	switch {
	case where == "":
		return it.Text
	case it.Map == 0:
		return it.Text + " (somewhere in " + where + ")"
	}
	return it.Text + " (" + where + ")"
}

func hereQuestOf(q *quest, role string, zone string, mapName func(int) string) HereQuest {
	hq := HereQuest{QuestID: q.id, Quest: q.name, Role: role}
	for _, o := range q.objs {
		hq.Missing = append(hq.Missing, itemOf(o, mapName, zone))
	}
	for _, o := range q.unplaced {
		hq.Missing = append(hq.Missing, itemOf(o, mapName, zone))
	}
	var parts []string
	for _, m := range hq.Missing {
		parts = append(parts, missingText(m))
	}
	switch {
	case role == "pickup" && len(parts) == 0:
		hq.Text = "Pick up " + q.name + "."
	case role == "pickup":
		hq.Text = "Pick up " + q.name + ", then: " + strings.Join(parts, "; ") + "."
	case len(parts) == 0:
		hq.Text = "Turn in " + q.name + ": nothing else is needed."
	default:
		hq.Text = "Turn in " + q.name + " once you have done: " + strings.Join(parts, "; ") + "."
	}
	return hq
}

// buildHere writes one block per NPC the route stops at (a pick-up or turn-in), nearest steps first.
// all are the listed quests: those with a known giver near the NPC and no step on the route are the
// nearby quests, each with its guidance.
func buildHere(p Plan, all []*quest, src source) []Here {
	byID := map[int]*quest{}
	for _, q := range all {
		if _, ok := byID[q.id]; !ok {
			byID[q.id] = q
		}
	}
	onRoute := map[int]bool{}
	for _, s := range p.Steps {
		for _, id := range s.QuestIDs {
			onRoute[id] = true
		}
	}
	type key struct {
		npc int
		m   int
	}
	var out []Here
	idx := map[key]int{}
	// The stop's spot carries the NPC id; a Stop does not, so re-find the spot through the quests.
	spotOf := func(s Stop, q *quest) *spot {
		for _, sp := range []*spot{q.pickup, q.turnin} {
			if sp != nil && sp.kind == s.Kind && sp.m == s.Map && math.Hypot(sp.x-s.X, sp.y-s.Y) <= 0.1 {
				return sp
			}
		}
		return nil
	}
	for _, s := range p.Steps {
		if s.Kind != KindPickup && s.Kind != KindTurnIn {
			continue
		}
		for _, id := range s.QuestIDs {
			q := byID[id]
			if q == nil {
				continue
			}
			sp := spotOf(s, q)
			if sp == nil || sp.npc == 0 {
				continue
			}
			// Here is only what can be done at the spot now: a turn-in that is not ready is a step, not a line here.
			if s.Kind == KindTurnIn && !sp.ready {
				continue
			}
			k := key{sp.npc, sp.m}
			i, ok := idx[k]
			if !ok {
				i = len(out)
				idx[k] = i
				out = append(out, Here{NPC: sp.label, NPCID: sp.npc, Map: sp.m, X: round1(sp.x), Y: round1(sp.y), Place: sp.place})
			}
			h := &out[i]
			if len(h.Steps) == 0 || h.Steps[len(h.Steps)-1] != s.N {
				h.Steps = append(h.Steps, s.N)
			}
			role := "turn in"
			if s.Kind == KindPickup {
				role = "pick up"
			}
			addUnique(&h.Can, role)
			h.Quests = append(h.Quests, hereQuestOf(q, s.Kind, p.Zone, src.mapName))
		}
	}
	ns, _ := src.(npcSource)
	for i := range out {
		h := &out[i]
		if ns != nil {
			if f, ok := ns.npc(h.NPCID); ok {
				fl := f.Flags
				for _, c := range []struct {
					bit  int
					word string
				}{{flagTrainer, "train"}, {flagVendor, "buy and sell"}, {flagRepair, "repair"}} {
					if fl&c.bit != 0 {
						h.Can = append(h.Can, c.word)
					}
				}
			}
		}
		for _, q := range all {
			if q.pickup == nil || onRoute[q.id] || q.pickup.m != h.Map || q.pickup.npc == h.NPCID {
				continue
			}
			if math.Hypot(q.pickup.x-h.X, q.pickup.y-h.Y) > NearbyRadius {
				continue
			}
			dup := false
			for _, n := range h.Nearby {
				dup = dup || n.QuestID == q.id
			}
			if dup {
				continue
			}
			for _, g := range p.Guides {
				if g.QuestID == q.id {
					h.Nearby = append(h.Nearby, Nearby{QuestID: q.id, Quest: q.name, Giver: q.pickup.label, Map: q.pickup.m,
						X: round1(q.pickup.x), Y: round1(q.pickup.y), Guidance: g})
					break
				}
			}
		}
		sort.Slice(h.Nearby, func(a, b int) bool { return h.Nearby[a].QuestID < h.Nearby[b].QuestID })
	}
	return out
}

func addUnique(list *[]string, s string) {
	for _, v := range *list {
		if v == s {
			return
		}
	}
	*list = append(*list, s)
}
