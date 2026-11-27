package route

import (
	"cmp"
	"slices"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// MaxFlights caps the Plan.lua "flights" table (docs/plan-format.md, additive keys, no schema bump).
const MaxFlights = 60

// Flight is a flight master the addon can suggest for a far stop: where it stands, its name, the
// flight node (the place the flight path is named after) and whether the character has opened it.
type Flight struct {
	Map   int     `json:"map"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Name  string  `json:"name"`
	Node  string  `json:"node"`
	Known bool    `json:"known"`
}

// flightMasterSource is a source that can list the flight masters of the data it plans from.
type flightMasterSource interface {
	flightMasters(c character.Character, maps map[int]bool) []Flight
}

// flightMasters are the taxi NPCs of Questie's data that stand on one of maps and serve the character's side.
func (s *questieSource) flightMasters(c character.Character, maps map[int]bool) []Flight {
	ids := make([]int, 0, 64)
	for id, e := range s.qx.Npcs {
		if e.IsFlightMaster() && sideServed(e.Faction, c.Faction) {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	var out []Flight
	for _, id := range ids {
		e := s.qx.Npcs[id]
		for _, sp := range e.Spawns {
			a, ok := s.qx.Areas[sp.Area]
			if !ok || a.UIMap == 0 || !maps[a.UIMap] {
				continue
			}
			out = append(out, Flight{Map: a.UIMap, X: round1(sp.X), Y: round1(sp.Y), Name: e.Name, Node: cmp.Or(a.Sub, a.Zone, s.mapName(a.UIMap))})
			break
		}
	}
	return out
}

// sideServed reports whether an NPC with Questie's friendlyToFaction serves the character's faction.
func sideServed(npcSide, faction string) bool {
	if npcSide == "" || npcSide == "AH" {
		return true
	}
	return faction == "" || strings.HasPrefix(strings.ToUpper(faction), npcSide)
}

// flightList is the plan's flight masters: the run's map and the maps of its other zones and trips,
// known ones (the character's captured flight paths) first-class and unknown ones from the data.
func flightList(c character.Character, src source, p *Plan, cur int) []Flight {
	maps := map[int]bool{}
	if cur != 0 {
		maps[cur] = true
	}
	for _, z := range p.Zones {
		maps[z.Map] = true
	}
	for _, t := range p.Trips {
		maps[t.Map] = true
	}
	seen := c.Seen.FlightsFor(c.Faction)
	known := map[int64]bool{}
	for _, f := range seen {
		known[f.NPC] = true
	}
	var out []Flight
	if fs, ok := src.(flightMasterSource); ok {
		for _, f := range fs.flightMasters(c, maps) {
			out = append(out, f)
		}
	}
	// A known flight path marks the data's master at the same place, or stands alone when the data has none.
	for _, f := range seen {
		if !maps[f.Map] {
			continue
		}
		node := cmp.Or(f.Name, src.mapName(f.Map))
		if ps, ok := src.(placeSource); ok {
			node = cmp.Or(ps.flightPlace(f.NPC), node)
		}
		hit := false
		for i := range out {
			if out[i].Map == f.Map && (out[i].Name == f.Name || (round1(f.X) == out[i].X && round1(f.Y) == out[i].Y)) {
				out[i].Known, hit = true, true
			}
		}
		if !hit {
			out = append(out, Flight{Map: f.Map, X: round1(f.X), Y: round1(f.Y), Name: f.Name, Node: node, Known: true})
		}
	}
	slices.SortStableFunc(out, func(a, b Flight) int {
		return cmp.Or(cmp.Compare(a.Map, b.Map), cmp.Compare(a.Name, b.Name))
	})
	if len(out) > MaxFlights {
		out = out[:MaxFlights]
	}
	return out
}

// linkSteps fills each step's Needs: the earlier steps of its quests it cannot be done before. A quest's
// pick-up comes before its objectives, both before its turn-in; a locked follow-up waits for the turn-ins
// that unlock it. Numbers are steps of the same list.
func linkSteps(steps []Stop) {
	ids := func(s Stop) []int {
		if len(s.QuestIDs) > 0 {
			return s.QuestIDs
		}
		if s.QuestID != 0 {
			return []int{s.QuestID}
		}
		return nil
	}
	for i := range steps {
		s := &steps[i]
		s.Needs = []int{}
		mine := ids(*s)
		for j := range steps {
			o := steps[j]
			if j == i || o.Kind == KindErrand || o.Kind == KindTravel {
				continue
			}
			share := false
			for _, id := range ids(o) {
				if slices.Contains(mine, id) {
					share = true
				}
			}
			need := false
			switch s.Kind {
			case KindObjective:
				need = share && o.Kind == KindPickup
			case KindTurnIn:
				need = share && (o.Kind == KindPickup || o.Kind == KindObjective)
			case KindPickup:
				if s.Locked {
					for _, id := range ids(o) {
						need = need || (o.Kind == KindTurnIn && slices.Contains(s.UnlockedBy, id))
					}
				}
			}
			if need {
				s.Needs = append(s.Needs, o.N)
			}
		}
		slices.Sort(s.Needs)
	}
}
