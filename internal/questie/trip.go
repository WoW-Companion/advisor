package questie

import "cmp"

// Trip is a quest that is handed in somewhere other than where it was given, and that asks for
// nothing countable: "find this NPC", "deliver this", "speak to". It is a trip, not a quick stop.
type Trip struct {
	Finisher string // the NPC or object that takes the quest in
	Object   bool
	Sub      string // the finisher's subzone, when the area has one
	Zone     string
	Instance string // the dungeon the finisher stands in; "" when it is in the open world
	Map      int
	X, Y     float64
}

// Place is where the finisher is in words: the subzone with its zone ("Ratchet (The Barrens)"), or
// the zone alone.
func (t Trip) Place() string {
	if t.Sub != "" && t.Zone != "" && t.Sub != t.Zone {
		return t.Sub + " (" + t.Zone + ")"
	}
	return cmp.Or(t.Sub, t.Zone)
}

// TripOf classifies a quest: it has no counted objectives (no creature, item, kill-credit or area
// objective) and a giver the data knows, none of whose is also a finisher. at, when set, picks the finisher spawn on the
// player's map first. ok is false for any other quest, or when no finisher has a known place.
func (ix *Index) TripOf(questID int, at *Where) (Trip, bool) {
	if ix == nil {
		return Trip{}, false
	}
	q := ix.Quests[questID]
	if q == nil || len(q.Starters) == 0 || len(q.Finishers) == 0 || len(q.Creatures) > 0 || len(q.Items) > 0 || len(q.Credits) > 0 || len(q.Triggers) > 0 {
		return Trip{}, false
	}
	for _, f := range q.Finishers {
		for _, s := range q.Starters {
			if f == s {
				return Trip{}, false
			}
		}
	}
	var best Trip
	found, onMap, bestDist := false, false, 0.0
	for _, r := range q.Finishers {
		e, ok := ix.entity(r)
		if !ok {
			continue
		}
		for _, s := range e.Spawns {
			a, ok := ix.Areas[s.Area]
			if !ok || cmp.Or(a.Sub, a.Zone) == "" {
				continue
			}
			t := Trip{Finisher: e.Name, Object: r.Object, Sub: a.Sub, Zone: a.Zone, Map: a.UIMap, X: s.X, Y: s.Y}
			if d := ix.DungeonOfArea(s.Area); d != nil {
				t.Instance = d.Name
				if t.Zone == "" || t.Zone == d.Name {
					if p, ok := ix.Areas[d.Parent]; ok {
						t.Zone = cmp.Or(p.Zone, p.Sub)
					}
				}
			}
			here := at != nil && ix.onMap(s.Area, *at)
			dist := 0.0
			if here {
				dist = (s.X-at.X)*(s.X-at.X) + (s.Y-at.Y)*(s.Y-at.Y)
			}
			if !found || (here && (!onMap || dist < bestDist)) {
				best, found, onMap, bestDist = t, true, here, dist
			}
		}
	}
	return best, found
}

// DungeonOfArea is the dungeon whose own area, or one of its alternatives, is area.
func (ix *Index) DungeonOfArea(area int) *Dungeon {
	for i := range ix.Dungeons {
		for _, a := range ix.Dungeons[i].Areas() {
			if a == area {
				return &ix.Dungeons[i]
			}
		}
	}
	return nil
}
