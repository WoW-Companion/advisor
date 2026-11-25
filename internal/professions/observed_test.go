package professions

import (
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/character"
)

func obsSmith(skill int) character.Character {
	return character.Character{Name: "Smith", Level: ip(20), Faction: "Alliance",
		Professions: []character.Skill{{Name: "Blacksmithing", Rank: ip(skill), Max: ip(150)}}}
}

func bsBand(t *testing.T, c character.Character, target int) Option {
	t.Helper()
	pa := ComputePath(c, pathData(t), "Blacksmithing", target, 1)
	if len(pa.Bands) == 0 {
		t.Fatalf("no bands: %+v", pa)
	}
	return pa.Bands[0].Best
}

func ownRecipes(entries ...character.SeenRecipe) map[string]character.SeenRecipes {
	return map[string]character.SeenRecipes{"blacksmithing": {SeenAt: time.Now().Unix(), Entries: entries}}
}

func TestKnownRecipeIsNotLearnedAgain(t *testing.T) {
	c := obsSmith(100)
	if o := bsBand(t, c, 140); o.Recipe != "Copper Mace" || o.Learn == "" {
		t.Fatalf("without the list the mace is learnt: %+v", o)
	}
	c.Recipes = ownRecipes(character.SeenRecipe{Name: "copper mace", Difficulty: "orange"})
	if o := bsBand(t, c, 140); o.Recipe != "Copper Mace" || o.Learn != "" || o.LearnInfo.Via != "known" {
		t.Errorf("a recipe in the player's list is known: %+v", o)
	}
}

func TestObservedReagentsOverrideDataset(t *testing.T) {
	c := obsSmith(100)
	c.Recipes = ownRecipes(character.SeenRecipe{Name: "Copper Mace", Difficulty: "orange",
		Reagents: []character.ItemStack{{ItemID: 4, Count: 2}}})
	o := bsBand(t, c, 140)
	if len(o.Needs) != 1 || o.Needs[0].Item != "Bronze Bar" || o.Needs[0].Total != 2*o.Crafts {
		t.Errorf("needs %+v for %d crafts", o.Needs, o.Crafts)
	}
}

func TestObservedOnlyRecipeIsPlanned(t *testing.T) {
	c := obsSmith(100)
	c.BankCaptured = true
	c.Bank = []character.ItemStack{stack(3, 500, "Copper Bar")}
	c.Recipes = ownRecipes(character.SeenRecipe{Name: "Sharpening Wheel", Difficulty: "orange",
		Reagents: []character.ItemStack{{ItemID: 3, Count: 1}}})
	pa := ComputePath(c, pathData(t), "Blacksmithing", 140, 1)
	text := RenderPath(pa)
	if pa.Bands[0].Best.Recipe != "Sharpening Wheel" || !strings.Contains(strings.Join(pa.Notes, " "), "Sharpening Wheel is on your own recipe list") {
		t.Errorf("observed-only recipe not used:\n%s", text)
	}
}

func TestSeenTrainerOfferReplacesEstimate(t *testing.T) {
	c := obsSmith(100)
	before := bsBand(t, c, 140).Learn
	if !strings.Contains(before, "probably from your trainer") {
		t.Fatalf("baseline %q", before)
	}
	c.Seen = &character.Observed{Trainers: []character.SeenTrainer{{Name: "Smith Sam", Faction: "Alliance", T: time.Now().Add(-48 * time.Hour).Unix(),
		Offers: []character.SeenOffer{{Name: "Copper Mace", State: "available", Cost: 500, Level: 8}}}}}
	after := bsBand(t, c, 140).Learn
	for _, want := range []string{"your trainer (Smith Sam) teaches it", Money(500), "needs level 8", "seen 2 days ago"} {
		if !strings.Contains(after, want) {
			t.Errorf("missing %q in %q", want, after)
		}
	}
	if strings.Contains(after, "probably") {
		t.Errorf("estimate left in %q", after)
	}
}

func TestSeenVendorPriceOverridesDataset(t *testing.T) {
	c := obsSmith(100)
	need := func() Need { return bsBand(t, c, 140).Needs[0] }
	if n := need(); !strings.Contains(n.From, "a vendor, "+Money(80)+" each") {
		t.Fatalf("baseline %q", n.From)
	}
	c.Seen = &character.Observed{Vendors: []character.SeenVendor{{Name: "Trade Tom", Faction: "Alliance", Map: 12, X: 40, Y: 50,
		T: time.Now().Add(-45 * 24 * time.Hour).Unix(), Items: []character.SeenStock{{ItemID: 3, Price: 30}}}}}
	n := need()
	for _, want := range []string{"a vendor, " + Money(30) + " each", "Trade Tom", "prices may have changed"} {
		if !strings.Contains(n.From, want) {
			t.Errorf("missing %q in %q", want, n.From)
		}
	}
}

func TestAbsentObservedSectionsAreHarmless(t *testing.T) {
	base := RenderPath(ComputePath(obsSmith(100), pathData(t), "Blacksmithing", 140, 1))
	c := obsSmith(100)
	c.Seen = &character.Observed{}
	c.Recipes = map[string]character.SeenRecipes{"mining": {}}
	if got := RenderPath(ComputePath(c, pathData(t), "Blacksmithing", 140, 1)); got != base {
		t.Errorf("empty observations changed the plan:\n%s\nvs\n%s", got, base)
	}
}
