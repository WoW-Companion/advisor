package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type pTier struct {
	Profession string `parquet:"profession"`
	Tier       string `parquet:"tier"`
	How        string `parquet:"how"`
	Trainer    string `parquet:"trainer_name"`
	Faction    string `parquet:"faction"`
	Place      string `parquet:"place"`
}

func trainerStore(t *testing.T, trainers []pTrainer, tiers []pTier) store.DataStore {
	t.Helper()
	src := store.ReaderSource{"trainers": table(t, trainers)}
	if tiers != nil {
		src["profession_tiers"] = table(t, tiers)
	}
	return store.New(src)
}

func TestTrainerStep(t *testing.T) {
	tiers := []pTier{
		{"Alchemy", "Journeyman", "trainer", "Hana Mixer", "Horde", "Ember Town"},
		{"Alchemy", "Journeyman", "trainer", "Alder Brew", "Alliance", "Stone Keep"},
		{"Alchemy", "Expert", "trainer", "Alder Expert", "Alliance", "Pine Hollow"},
		{"Alchemy", "Journeyman", "book", "Bookseller", "Both", "Anywhere"},
	}
	for _, tc := range []struct {
		name     string
		trainers []pTrainer
		tiers    []pTier
		faction  string
		next     string
		want     string
	}{
		{"a named trainer wins", []pTrainer{{1, "Table Tam", "Alchemy", "Undercity", 9}}, tiers, "Horde", "Journeyman", "Trainer to visit: Table Tam in Undercity."},
		{"the guide names the trainer when the table has an id only", []pTrainer{{2, "", "Alchemy", "Undercity", 9}}, tiers, "Horde", "Journeyman", "Trainer to visit: Hana Mixer in Ember Town."},
		{"the guide's trainer of the tier about to be trained", nil, tiers, "Alliance", "Expert", "Trainer to visit: Alder Expert in Pine Hollow."},
		{"the other side's guide trainer is never named", nil, tiers[:1], "Alliance", "Journeyman", "The game data names no Alchemy trainer for your side; ask in a capital city."},
		{"an id-only trainer still beats nothing", []pTrainer{{3, "", "Alchemy", "Undercity", 9}}, nil, "Horde", "Journeyman", "Trainer to visit: npc 3 (name not in the dataset) in Undercity."},
		{"a continent is no place", []pTrainer{{4, "", "Alchemy", "Kalimdor", 9}}, nil, "Horde", "Journeyman", "Trainer to visit: npc 4 (name not in the dataset)."},
		{"a higher rank is taught by the trainer the tier row names, not the busiest trainer", []pTrainer{{1, "Table Tam", "Alchemy", "Stone Keep", 9}}, tiers, "Alliance", "Expert", "Trainer to visit: Alder Expert in Pine Hollow."},
		{"no trainer at all says so", nil, nil, "Horde", "Journeyman", "The game data names no Alchemy trainer for your side; ask in a capital city."},
	} {
		p := &planner{c: character.Character{Faction: tc.faction}, ds: trainerStore(t, tc.trainers, tc.tiers), prof: "Alchemy"}
		if got := p.trainerStep(tc.next); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
	if strings.Contains((&planner{ds: trainerStore(t, nil, tiers), prof: "Alchemy"}).trainerStep("Journeyman"), "Bookseller") {
		t.Error("a book vendor was named as a trainer")
	}
}

func TestTrainNowNamesTheRankAfterTheCapHeld(t *testing.T) {
	type tier struct {
		Profession string `parquet:"profession"`
		Tier       string `parquet:"tier"`
		Skill      int64  `parquet:"skill_required"`
		MinLevel   int64  `parquet:"min_level"`
		Cost       int64  `parquet:"cost_copper"`
		How        string `parquet:"how"`
		Trainer    string `parquet:"trainer_name"`
		Faction    string `parquet:"faction"`
		Place      string `parquet:"place"`
	}
	rows := []tier{
		{"Alchemy", "Journeyman", 50, 10, 0, "trainer", "Jo Brew", "Alliance", "Stone Keep"},
		{"Alchemy", "Expert", 125, 20, 5000, "trainer", "Alder Expert", "Alliance", "Pine Hollow"},
		{"Alchemy", "Artisan", 200, 35, 0, "trainer", "Art Brew", "Alliance", "Far Peak"},
		{"First Aid", "Expert", 125, 20, 0, "book", "Book Ben", "Alliance", "Reading Vale"},
	}
	ds := store.New(store.ReaderSource{"profession_tiers": table(t, rows)})
	lv, held, rk := 25, 150, 140
	for _, tc := range []struct{ prof, want, not string }{
		{"Alchemy", "Train now: Alchemy Expert (character level 20, skill 125, rank cap 225). Training costs 50s. Trainer to visit: Alder Expert in Pine Hollow.", "Art Brew"},
		{"First Aid", "Train now: First Aid Expert (character level 20, skill 125, rank cap 225). It is not taught by a trainer: it is a book from Book Ben in Reading Vale. The game data lists no training price for it.", "Trainer to visit"},
	} {
		p := &planner{c: character.Character{Faction: "Alliance", Level: &lv}, ds: ds, notes: LoadNotes(ds), prof: tc.prof, skill: rk}
		allowed, tier := p.notes.CapAt(lv)
		var pa Path
		p.where(&pa, character.Skill{Name: tc.prof, Rank: &rk, Max: &held}, allowed, tier)
		got := strings.Join(pa.Where, " ")
		if !strings.Contains(got, tc.want) || strings.Contains(got, tc.not) {
			t.Errorf("%s: %q, want %q without %q", tc.prof, got, tc.want, tc.not)
		}
	}
}
