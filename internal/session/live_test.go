package session

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/store"
)

// A session ends at an inn or flight point of the character's own side or a neutral one, never the other side's.
func TestLiveAnchorsServeTheCharactersSide(t *testing.T) {
	at := func(name, side string, flags int) questie.Entity {
		return questie.Entity{Name: name, Faction: side, Flags: flags, Spawns: []questie.Spawn{{Area: 1, X: 50, Y: 50}}}
	}
	qx := &questie.Index{
		Areas: map[int]questie.Area{1: {UIMap: 1, Zone: "Synthetic Vale"}},
		Npcs: map[int]questie.Entity{1: at("Alliance Keeper", "A", flagInnkeeper), 2: at("Horde Keeper", "H", flagInnkeeper),
			3: at("Neutral Keeper", "AH", flagInnkeeper), 4: at("Flight Master", "", flagFlight)},
	}
	for _, tc := range []struct{ side, want string }{{"Horde", "Alliance Keeper"}, {"Alliance", "Horde Keeper"}} {
		c := character.Character{Faction: tc.side}
		in := Live(c, store.New(store.ReaderSource{}), qx, route.Plan{}, nil)
		if len(in.Anchors) != 3 {
			t.Errorf("%s: %d anchors, want the own side's, the neutral inn and the flight point", tc.side, len(in.Anchors))
		}
		for _, a := range in.Anchors {
			if strings.Contains(a.Name, tc.want) {
				t.Errorf("%s: anchor %q is the other side's", tc.side, a.Name)
			}
		}
	}
}
