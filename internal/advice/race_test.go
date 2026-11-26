package advice

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

type raceRow struct {
	ID       int64   `parquet:"id"`
	Name     string  `parquet:"name"`
	Level    *int32  `parquet:"level,optional"`
	Zone     *string `parquet:"zone_name,optional"`
	RaceMask int64   `parquet:"allowable_race_mask"`
}

// Quests keyed by race mask: 1 Alliance-only (human), 2 Horde-only (orc), 3 Alliance Skyborne bit,
// 4 Horde Skyborne bit, 5 open to all.
func raceStore(t *testing.T) store.DataStore {
	z := "Testisle"
	lvl := i32(9)
	rows := []raceRow{
		{1, "Alliance Errand", lvl, &z, 1},
		{2, "Horde Errand", lvl, &z, 2},
		{3, "Alliance Sky Errand", lvl, &z, int64(questie.BitSkyborneAlliance)},
		{4, "Horde Sky Errand", lvl, &z, int64(questie.BitSkyborneHorde)},
		{5, "Open Errand", lvl, &z, 0},
	}
	return store.New(store.ReaderSource{"quests": parq(t, rows)})
}

func availIDs(c character.Character, ds store.DataStore) (string, []string) {
	a := AvailableQuests(c, ds)
	var ids []string
	for _, s := range a.Quests {
		ids = append(ids, strings.TrimPrefix(s.Title, PickUpPrefix))
	}
	return strings.Join(ids, "|"), a.Unchecked
}

func TestAvailableQuestsRaceFilter(t *testing.T) {
	lvl := 9
	loc := &character.Location{Zone: "Testisle"}
	ds := raceStore(t)
	for _, tc := range []struct {
		name, race, faction string
		want                []string
		note                bool
	}{
		{"horde skyborne", "Skyborne", "Horde", []string{"Horde Sky Errand", "Open Errand"}, false},
		{"alliance skyborne", "Skyborne", "Alliance", []string{"Alliance Sky Errand", "Open Errand"}, false},
		{"unknown race, horde", "Mystery", "Horde", []string{"Horde Errand", "Horde Sky Errand", "Open Errand"}, true},
		{"unknown race, alliance", "Mystery", "Alliance", []string{"Alliance Errand", "Alliance Sky Errand", "Open Errand"}, true},
		{"known race", "Orc", "Horde", []string{"Horde Errand", "Open Errand"}, false},
	} {
		c := character.Character{Level: &lvl, Class: "ROGUE", Race: tc.race, Faction: tc.faction, Location: loc}
		got, unchecked := availIDs(c, ds)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: %q missing from %q", tc.name, w, got)
			}
		}
		if n := len(strings.Split(got, "|")); n != len(tc.want) {
			t.Errorf("%s: got %q, want %v", tc.name, got, tc.want)
		}
		if said := strings.Contains(strings.Join(unchecked, ";"), "faction's races"); said != tc.note {
			t.Errorf("%s: fallback named = %v, want %v (%v)", tc.name, said, tc.note, unchecked)
		}
	}
}
