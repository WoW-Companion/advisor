package questie

import (
	"testing"
	"time"
)

func TestHolidayQuestsFollowTheirEvent(t *testing.T) {
	ix := &Index{Quests: map[int]*Quest{
		1: {ID: 1, Name: "Flickering Flames in Kalimdor", Level: 25, Starters: []Ref{{ID: 10}}},
		2: {ID: 2, Name: "Plain Errand", Level: 24, Starters: []Ref{{ID: 11}}},
		3: {ID: 3, Name: "Gift of Fire", Level: 24, Starters: []Ref{{ID: 10}}},
		4: {ID: 4, Name: "Too Hard", Level: 24, MinLevel: 35, Starters: []Ref{{ID: 11}}},
	}, Npcs: map[int]Entity{10: {Name: "Festival Talespinner"}, 11: {Name: "Elder Mira"}}}
	ix.tagEvents()
	nov := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	jun := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		id   int
		now  time.Time
		log  bool
		want bool
	}{
		{"Midsummer quest in November", 1, nov, false, false},
		{"event-only giver in November", 3, nov, false, false},
		{"Midsummer quest on June 25", 1, jun, false, true},
		{"event-only giver on June 25", 3, jun, false, true},
		{"holiday quest already in the log", 1, nov, true, false}, // not a pick-up: the log shows it
		{"ordinary quest in November", 2, nov, false, true},
		{"required level above the player", 4, jun, false, false},
		{"unknown clock keeps the quest", 1, time.Time{}, false, true},
	}
	for _, c := range cases {
		w := Who{Level: 24, Below: 5, Above: 5, Now: c.now, Done: map[int]bool{}, InLog: map[int]bool{}}
		w.InLog[1] = c.log
		if got := ix.Quests[c.id].Eligible(w); got != c.want {
			t.Errorf("%s: Eligible = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestEventWindows(t *testing.T) {
	winter := eventNamed("Feast of Winter Veil")
	for _, c := range []struct {
		m, d int
		want bool
	}{{12, 25, true}, {1, 1, true}, {1, 3, false}, {12, 14, false}} {
		if got := winter.Open(time.Date(2026, time.Month(c.m), c.d, 0, 0, 0, 0, time.UTC)); got != c.want {
			t.Errorf("winter %d/%d = %v", c.m, c.d, got)
		}
	}
}
