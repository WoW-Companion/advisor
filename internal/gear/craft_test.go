package gear

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type recipeRow struct {
	Spell   int32  `parquet:"spell_id"`
	Name    string `parquet:"name"`
	Line    string `parquet:"skill_line_name"`
	Skill   int32  `parquet:"learn_skill"`
	From    string `parquet:"learned_from"`
	Creates int32  `parquet:"creates_item_id"`
}

func tailor(rank int) character.Character {
	level := 24
	return character.Character{Class: "PRIEST", Level: &level, Professions: []character.Skill{{Name: "Tailoring", Rank: &rank}}}
}

func recipes(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{"recipes": table(t, []recipeRow{
		{1, "Linen Belt", "Tailoring", 40, "trainer", 100},
		{2, "Azure Silk Vest", "Tailoring", 125, "trainer", 200},
		{3, "Robes of Arcana", "Tailoring", 125, "item", 300},
		{4, "Pattern Belt", "Tailoring", 30, "item", 400},
		{5, "Iron Helm", "Blacksmithing", 30, "trainer", 500},
	})})
}

func TestCraftOnlyWhatTheSkillReaches(t *testing.T) {
	c, d := tailor(70), recipes(t)
	for _, tc := range []struct {
		item   int64
		status string
	}{{100, CraftNow}, {200, CraftNeedSkill}, {300, CraftNeedItem}, {400, CraftNeedItem}, {500, CraftNoProf}} {
		got, ok := craftOf(c, d, tc.item)
		if !ok || got.Status != tc.status {
			t.Errorf("item %d = %+v %v, want %s", tc.item, got, ok, tc.status)
		}
	}
	c.KnownSpells = map[int64]bool{3: true}
	if got, _ := craftOf(c, d, 300); got.Status != CraftNow {
		t.Errorf("a known recipe = %+v", got)
	}
	if _, ok := craftOf(c, d, 999); ok {
		t.Error("an item no recipe makes has a craft")
	}
}

func TestBeyondReachNeverCallsThemCraftable(t *testing.T) {
	var cands []Candidate
	for _, x := range []struct {
		name string
		cr   Craft
	}{{"Azure Silk Vest", Craft{"Tailoring", 125, CraftNeedSkill}}, {"Robes of Arcana", Craft{"Tailoring", 125, CraftNeedItem}}, {"Linen Belt", Craft{"Tailoring", 40, CraftNow}}} {
		cr := x.cr
		cands = append(cands, Candidate{Item: Item{Name: x.name}, Craft: &cr})
	}
	out := strings.Join(beyondReach(cands), "\n")
	if !strings.Contains(out, "At Tailoring 125: Azure Silk Vest") || !strings.Contains(out, "recipe you don't have: Robes of Arcana") || strings.Contains(out, "Linen Belt") {
		t.Errorf("lines = %q", out)
	}
	if cands[2].outOfReach() || !cands[0].outOfReach() {
		t.Error("outOfReach")
	}
}
