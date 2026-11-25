package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type pPriced struct {
	ID    int64  `parquet:"id"`
	Name  string `parquet:"name"`
	Price int64  `parquet:"buy_price"`
}

func TestBasePrice(t *testing.T) {
	ds := store.New(store.ReaderSource{"items": table(t, []pPriced{{1, "Mild Spices", 10}, {2, "Unpriced", 0}})})
	for _, c := range []struct {
		name         string
		item, listed int64
		want         int64
	}{
		{"the base price beats a discounted listing", 1, 9, 10},
		{"no base price: the listing stands", 2, 9, 9},
		{"item not in the dataset: the listing stands", 3, 9, 9},
	} {
		if got := basePrice(ds, c.item, c.listed); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

func TestTrainerIsOnTheCharactersSide(t *testing.T) {
	ds := store.New(store.ReaderSource{"trainers": table(t, []pTrainer{
		{1, "Horde Hana", "Alchemy", "Undercity", 9}, {2, "Alliance Al", "Alchemy", "Stormwind City", 8}, {3, "Unplaced Una", "Alchemy", "", 2},
	})})
	for _, c := range []struct{ faction, want string }{
		{"Horde", "Horde Hana in Undercity"},
		{"Alliance", "Alliance Al in Stormwind City"},
		{"", "Horde Hana in Undercity"},
	} {
		p := &planner{c: character.Character{Faction: c.faction}, ds: ds, prof: "Alchemy"}
		if got := p.trainer(); got != c.want {
			t.Errorf("faction %q: trainer %q, want %q", c.faction, got, c.want)
		}
	}
}

func TestTrainerCostTextNeverCallsAnUnknownPriceFree(t *testing.T) {
	for _, c := range []struct {
		t    BenchTier
		want string
	}{
		{BenchTier{TrainerSpells: 0}, "0"},
		{BenchTier{TrainerSpells: 3, TrainerCopper: 1500}, "3 for 15s"},
		{BenchTier{TrainerSpells: 3, TrainerUnpriced: 3}, "price not in the dataset"},
		{BenchTier{TrainerSpells: 3, TrainerCopper: 1500, TrainerUnpriced: 1}, "plus 1 with no price in the dataset"},
	} {
		if got := trainerCostText(c.t); !strings.Contains(got, c.want) || strings.Contains(got, "0c") {
			t.Errorf("%+v: %q, want it to say %q and never 0c", c.t, got, c.want)
		}
	}
}

func TestGuideCheckHasNoEmptyBrackets(t *testing.T) {
	got := guideCheck([]BenchBand{{From: 1, To: 75, Recipe: "Fish (Apprentice)"}, {From: 75, To: 150, Recipe: "Linen Bandage", Source: "trainer, confirmed"}, {From: 150, To: 225, Gap: "x"}})
	want := "Guide check: 1-75 Fish (Apprentice); 75-150 Linen Bandage (trainer, confirmed); 150-225 GAP."
	if got != want {
		t.Errorf("%q, want %q", got, want)
	}
}
