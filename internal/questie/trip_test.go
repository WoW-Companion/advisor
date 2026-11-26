package questie

import "testing"

func tripFixture() *Index {
	sp := func(area int) []Spawn { return []Spawn{{Area: area, X: 30, Y: 40}} }
	return &Index{
		Npcs: map[int]Entity{1: {Name: "Giver", Spawns: sp(901)}, 2: {Name: "Vincent", Spawns: sp(209)}, 3: {Name: "Sputtervalve", Spawns: sp(903)}, 4: {Name: "Wolf", Spawns: sp(901)}},
		Areas: map[int]Area{
			901: {UIMap: 7001, Zone: "Town"},
			902: {UIMap: 7002, Zone: "Silverpine Forest"},
			209: {UIMap: 33, Sub: "Shadowfang Keep", Zone: "Shadowfang Keep"},
			903: {UIMap: 7003, Sub: "Ratchet", Zone: "The Barrens"},
		},
		Dungeons: []Dungeon{{Area: 209, Parent: 902, Name: "Shadowfang Keep"}},
		Quests: map[int]*Quest{
			1: {ID: 1, Starters: []Ref{{ID: 1}}, Finishers: []Ref{{ID: 2}}},
			2: {ID: 2, Starters: []Ref{{ID: 1}}, Finishers: []Ref{{ID: 3}}},
			3: {ID: 3, Starters: []Ref{{ID: 1}}, Finishers: []Ref{{ID: 3}}, Creatures: []Ref{{ID: 4}}},
			4: {ID: 4, Starters: []Ref{{ID: 1}}, Finishers: []Ref{{ID: 1}}},
		},
	}
}

func TestTripOf(t *testing.T) {
	ix := tripFixture()
	for _, tc := range []struct {
		name        string
		quest       int
		ok          bool
		place, inst string
		zone        string
	}{
		{"inside a dungeon", 1, true, "Shadowfang Keep (Silverpine Forest)", "Shadowfang Keep", "Silverpine Forest"},
		{"town delivery", 2, true, "Ratchet (The Barrens)", "", "The Barrens"},
		{"has an objective", 3, false, "", "", ""},
		{"same giver and finisher", 4, false, "", "", ""},
		{"unknown quest", 99, false, "", "", ""},
	} {
		got, ok := ix.TripOf(tc.quest, nil)
		if ok != tc.ok || (ok && (got.Place() != tc.place || got.Instance != tc.inst || got.Zone != tc.zone)) {
			t.Errorf("%s: %+v ok=%v", tc.name, got, ok)
		}
	}
}
