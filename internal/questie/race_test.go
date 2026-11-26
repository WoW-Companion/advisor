package questie

import "testing"

func TestRaceBit(t *testing.T) {
	for _, tc := range []struct {
		race, faction string
		bit           uint64
		ok            bool
	}{
		{"Skyborne", "Horde", 8589934592, true},
		{"Skyborne", "Alliance", 4294967296, true},
		{"WindshaperSkyborne", "Horde", 1 << 33, true},
		{"High Order Skyborne", "Alliance", 1 << 32, true},
		{"Skyborn", "Horde", 1 << 33, true},
		{"Skyborne", "", 0, false},
		{"NightElf", "Alliance", 1 << 3, true},
		{"Pandaren", "Horde", 1 << 23, true},
		{"Mystery", "Horde", 0, false},
	} {
		if bit, ok := RaceBit(tc.race, tc.faction); bit != tc.bit || ok != tc.ok {
			t.Errorf("RaceBit(%q,%q) = %d,%v", tc.race, tc.faction, bit, ok)
		}
	}
}

func TestEligibleRaceMask(t *testing.T) {
	horde, _, _ := RaceMasks("Skyborne", "Horde")
	base := Who{Level: 9, Below: 2, Above: 2, RaceMask: horde}
	_, side, _ := RaceMasks("Mystery", "Horde")
	fallback := Who{Level: 9, Below: 2, Above: 2, SideMask: side}
	q := func(races uint64) *Quest {
		return &Quest{ID: 1, Name: "Errand", Level: 9, Races: races, Starters: []Ref{{ID: 1}}}
	}
	for _, tc := range []struct {
		name  string
		w     Who
		races uint64
		want  bool
	}{
		{"horde skyborne, alliance human only", base, 1, false},
		{"horde skyborne, horde skyborne bit above 32 bits", base, BitSkyborneHorde, true},
		{"horde skyborne, alliance skyborne bit", base, BitSkyborneAlliance, false},
		{"horde skyborne, no mask", base, 0, true},
		{"fallback horde, alliance only", fallback, 1, false},
		{"fallback horde, orc", fallback, 2, true},
		{"fallback horde, horde skyborne bit", fallback, BitSkyborneHorde, true},
	} {
		if got := q(tc.races).Eligible(tc.w); got != tc.want {
			t.Errorf("%s: Eligible = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestHostileGiver(t *testing.T) {
	ix := &Index{Npcs: map[int]Entity{1: {Faction: "A"}, 2: {Faction: "AH"}, 3: {}}}
	for _, tc := range []struct {
		side    string
		starter Ref
		want    bool
	}{
		{"H", Ref{ID: 1}, true},
		{"A", Ref{ID: 1}, false},
		{"H", Ref{ID: 2}, false},
		{"H", Ref{ID: 3}, false},
		{"", Ref{ID: 1}, false},
		{"H", Ref{ID: 1, Object: true}, false},
	} {
		if got := ix.Hostile(&Quest{Starters: []Ref{tc.starter}}, Who{Side: tc.side}); got != tc.want {
			t.Errorf("%+v side %q: %v, want %v", tc.starter, tc.side, got, tc.want)
		}
	}
}
