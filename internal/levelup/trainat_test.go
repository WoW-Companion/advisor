package levelup

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

func priestIn(zone string) character.Character {
	level, x, y := 24, 50.0, 50.0
	return character.Character{Class: "PRIEST", Faction: "Horde", Level: &level, Location: &character.Location{Zone: zone, X: &x, Y: &y}}
}

// Deathknell's trainer is in QuestieDB but not in a city: it is never the answer for a priest far away.
func starterIndex() *questie.Index {
	return &questie.Index{DB: "t",
		Areas: map[int]questie.Area{1: {Zone: "Tirisfal Glades"}, 2: {Zone: "Undercity"}},
		Npcs: map[int]questie.Entity{
			20: {Name: "Starter Cleric", Sub: "Priest Trainer", Faction: "H", Spawns: []questie.Spawn{{Area: 1, X: 31, Y: 66}}},
		}}
}

func TestTrainAtSendsADistantPriestToTheCity(t *testing.T) {
	p, ok := TrainAt(priestIn("Hillsbrad Foothills"), starterIndex())
	if !ok || p.City != "Undercity" || p.From != "Tarren Mill" || p.Trainer != nil {
		t.Fatalf("place = %+v %v", p, ok)
	}
	if got := p.Line("PRIEST"); got != "Train at: Undercity, your priest trainer there (fly from Tarren Mill)" {
		t.Errorf("line = %q", got)
	}
}

func TestTrainAtTrimsTheZoneAndSaysWhereTheHearthstoneIsBound(t *testing.T) {
	c := priestIn(" Hillsbrad Foothills ")
	c.Hearth = "Undercity"
	p, _ := TrainAt(c, nil)
	if got := p.Line("PRIEST"); got != "Train at: Undercity, your priest trainer there (fly from Tarren Mill; your hearthstone is bound there)" {
		t.Errorf("line = %q", got)
	}
	c.Hearth = "Orgrimmar"
	if p, _ = TrainAt(c, nil); p.Bound {
		t.Errorf("a bind elsewhere is not the city: %+v", p)
	}
}

func TestTrainAtNamesATrainerOnlyWhenADataSourceDoes(t *testing.T) {
	qx := starterIndex()
	qx.Npcs[21] = questie.Entity{Name: "City Cleric", Sub: "Priest Trainer", Faction: "H", Spawns: []questie.Spawn{{Area: 2, X: 70, Y: 58}}}
	p, _ := TrainAt(priestIn("Hillsbrad Foothills"), qx)
	if p.Trainer == nil || p.Trainer.Name != "City Cleric" || !strings.Contains(p.Line("PRIEST"), "City Cleric (70, 58)") {
		t.Errorf("place = %+v", p)
	}
	if got, _ := TrainAt(priestIn("Hillsbrad Foothills"), starterIndex()); strings.Contains(got.Line("PRIEST"), "Starter Cleric") {
		t.Errorf("named a trainer no source places in the city: %q", got.Line("PRIEST"))
	}
	text, _, _ := Facts(priestIn("Hillsbrad Foothills"), ds(t, false), starterIndex(), 24, 24)
	if strings.Contains(text, "Starter Cleric") {
		t.Errorf("facts name a starting-area trainer:\n%s", text)
	}
}

func TestClassCityCoversEveryClass(t *testing.T) {
	for _, f := range []string{"Horde", "Alliance"} {
		for cl := range ClassIDs {
			if ClassCity(f, cl) == "" {
				t.Errorf("no city for %s %s", f, cl)
			}
		}
	}
}

// A capture that names the zone but holds no position still tells the player where to fly from.
func TestTrainAtUsesTheZoneWhenTheCaptureHasNoPosition(t *testing.T) {
	c := priestIn("Silverpine Forest")
	c.Location.X, c.Location.Y = nil, nil
	p, ok := TrainAt(c, starterIndex())
	if !ok || p.From != "The Sepulcher" {
		t.Fatalf("place = %+v %v", p, ok)
	}
}

func TestClassCityForPrefersTheCharactersOwnCityThenContinent(t *testing.T) {
	for _, tc := range []struct{ side, class, zone, want string }{
		{"Horde", "MAGE", "Orgrimmar", "Orgrimmar"}, {"Horde", "WARRIOR", "Durotar", "Orgrimmar"}, {"Horde", "WARRIOR", "Tirisfal Glades", "Undercity"},
		{"Horde", "WARRIOR", "", "Undercity"}, {"Horde", "DRUID", "Durotar", "Thunder Bluff"}, {"Alliance", "ROGUE", "Teldrassil", "Darnassus"},
		{"Alliance", "MAGE", "Teldrassil", "Stormwind City"}, {"Horde", "ROGUE", "Valley of Trials", "Orgrimmar"},
		{"Alliance", "ROGUE", "Loch Modan", "Ironforge"}, {"Alliance", "WARRIOR", "Wetlands", "Ironforge"}, {"Alliance", "WARRIOR", "Duskwood", "Stormwind City"},
		{"Alliance", "WARRIOR", "Elwynn Forest", "Stormwind City"},
	} {
		if got := classCityFor(tc.side, tc.class, tc.zone); got != tc.want {
			t.Errorf("%s %s in %q: %q, want %q", tc.side, tc.class, tc.zone, got, tc.want)
		}
	}
	if got := classCityFor("Horde", "HUNTER", "Elwynn Forest"); got != ClassCity("Horde", "HUNTER") {
		t.Errorf("a continent with no trainer city: %q", got)
	}
}
