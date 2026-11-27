package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

type nodeRow struct {
	NodeID        int32  `parquet:"node_id"`
	Name          string `parquet:"name"`
	Profession    string `parquet:"profession"`
	SkillRequired *int16 `parquet:"skill_required,optional"`
	ZoneName      string `parquet:"zone_name"`
}

type noteRow struct {
	Section string `parquet:"section"`
	Subject string `parquet:"subject"`
	Text    string `parquet:"text"`
}

func gatherStore(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"gathering_nodes": table(t, []nodeRow{
			{1, "Tin Vein", "Mining", i16(65), "The Barrens"}, {1, "Tin Vein", "Mining", i16(65), "Ashenvale"},
			{2, "Peacebloom", "Herbalism", i16(1), "Durotar"}, {3, "Feuillargent", "Herbalism", nil, "Mulgore"},
		}),
		"profession_notes": table(t, []noteRow{{"rules", "skinning", "Skill needed is five times the creature's level from level 20 up."}}),
	})
}

func TestGatherFactsNodeSkill(t *testing.T) {
	ds := gatherStore(t)
	for _, tc := range []struct {
		q    string
		want []string
		not  []string
	}{
		{"what skill do I need to mine tin?", []string{"- Tin Vein: needs Mining 65; found in Ashenvale, The Barrens", "name no skill or zone from memory"}, []string{"Peacebloom"}},
		{"which herbalism skill does Peacebloom need?", []string{"- Peacebloom: needs Herbalism 1; found in Durotar"}, []string{"Tin Vein"}},
		{"what mining skill does Thorium Vein need?", []string{"- Tin Vein: needs Mining 65", "For any other ore or herb"}, []string{"Thorium Vein: needs"}},
		{"what level should I be to mine at the tin mine in Dun Morogh?", nil, nil},
		{"what should I do next?", nil, nil},
	} {
		got := GatherFacts(ds, tc.q)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%q: facts lack %q:\n%s", tc.q, w, got)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(got, n) {
				t.Errorf("%q: facts hold %q:\n%s", tc.q, n, got)
			}
		}
	}
}

func TestSkinningSkillRule(t *testing.T) {
	for _, tc := range []struct {
		level, skill int
		ok           bool
	}{{10, 0, false}, {11, 10, true}, {15, 50, true}, {19, 90, true}, {20, 100, true}, {30, 150, true}, {45, 225, true}} {
		if skill, ok := skinningSkill(tc.level); skill != tc.skill || ok != tc.ok {
			t.Errorf("level %d: skill %d ok %v, want %d %v", tc.level, skill, ok, tc.skill, tc.ok)
		}
	}
	got := GatherFacts(gatherStore(t), "what skinning skill for a level 30 creature, and a level 8 one?")
	for _, w := range []string{"five times the creature's level", "A level 30 creature needs Skinning 150.", "A level 8 creature: the rule gives no skill below level 11"} {
		if !strings.Contains(got, w) {
			t.Errorf("skinning facts lack %q:\n%s", w, got)
		}
	}
	if got := GatherFacts(store.New(store.ReaderSource{}), "what skinning skill for level 30?"); !strings.Contains(got, "holds no rule") {
		t.Errorf("no rule row: %q", got)
	}
}

func TestFishingFactsSayNoSkill(t *testing.T) {
	got := GatherFacts(gatherStore(t), "what fishing skill do I need in the Barrens?")
	if !strings.Contains(got, "holds no fishing skill") || strings.Contains(got, "needs Fishing") {
		t.Errorf("fishing facts: %q", got)
	}
}

// A question on a recipe's skill is asked many ways: "can I make it yet", "how high must my skill be to cook it".
func TestRecipeAskPhrasings(t *testing.T) {
	for _, q := range []string{"can I make Heavy Silk Bandage yet?", "how high must cooking be for Boiled Clams", "when can I cook Crocolisk Steak?", "what do I need to craft Anti-Venom?"} {
		if !recipeAsk.MatchString(q) {
			t.Errorf("%q is not read as a question on a recipe's skill", q)
		}
	}
	if recipeAsk.MatchString("where do I sell Boiled Clams?") {
		t.Error("a question on where to sell was read as one on a recipe's skill")
	}
}
