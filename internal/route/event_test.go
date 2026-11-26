package route

import (
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/questie"
)

func eventIsle() *questie.Index {
	at := func(x, y float64) []questie.Spawn { return []questie.Spawn{{Area: 901, X: x, Y: y}} }
	return &questie.Index{
		Quests: map[int]*questie.Quest{
			60: {ID: 60, Name: "Far Boars", Level: 9, Starters: []questie.Ref{{ID: 1}}, Finishers: []questie.Ref{{ID: 1}}},
			61: {ID: 61, Name: "Near Flames", Level: 9, Event: "midsummer", Starters: []questie.Ref{{ID: 2}}, Finishers: []questie.Ref{{ID: 2}}},
		},
		Npcs: map[int]questie.Entity{
			1: {Name: "Elder Far", Faction: "AH", Flags: flagQuestGiver, Spawns: at(90, 90)},
			2: {Name: "Festival Test", Faction: "AH", Flags: flagQuestGiver, Spawns: at(49, 49)},
		},
		Objects: map[int]questie.Entity{},
		Areas:   map[int]questie.Area{901: {UIMap: 7001, Zone: "Testisle"}},
	}
}

func TestEventQuestsJoinTheRouteOnlyWhenOptedInAndRunning(t *testing.T) {
	for _, tc := range []struct {
		name string
		when time.Time
		set  string
		want bool
	}{
		{"off in event", time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC), "", false},
		{"on in event", time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC), "on", true},
		{"on after event", time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC), "on", false},
	} {
		c := hero()
		at := tc.when.Unix()
		c.CapturedAt, c.Seasonal = &at, tc.set
		p := Build(c, eventIsle())
		flames, boars := stepIndex(p, KindPickup, 61), stepIndex(p, KindPickup, 60)
		if (flames >= 0) != tc.want || boars < 0 {
			t.Errorf("%s: flames at %d, boars at %d: %+v", tc.name, flames, boars, p.Steps)
			continue
		}
		if tc.want && (flames < boars || !p.Steps[flames].Optional || p.Steps[boars].Optional) {
			t.Errorf("%s: the optional event steps must come after the progress ones: %+v", tc.name, p.Steps)
		}
		for _, s := range p.Later {
			if s.QuestID == 61 {
				t.Errorf("%s: event quest in later", tc.name)
			}
		}
	}
}
