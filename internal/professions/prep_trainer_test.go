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
