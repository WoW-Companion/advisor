package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

// Orange gives a point a craft, yellow three in four, green one in four, grey none.
func TestCraftsFollowTheColourChances(t *testing.T) {
	r := Recipe{YellowSkill: ip(15), GreySkill: ip(55)} // orange to 15, yellow 15-34, green 35-54
	for _, tc := range []struct {
		name                       string
		have, target               int
		crafts, reaches, o, y, grn int
		chance                     float64
	}{
		{"orange only", 1, 10, 9, 10, 9, 0, 0, 1},
		{"orange then yellow", 5, 25, 24, 25, 10, 14, 0, 1},
		{"into green", 25, 45, 54, 45, 0, 14, 40, 0.75},
		{"green only", 40, 50, 40, 50, 0, 0, 40, 0.25},
		{"capped by grey", 50, 99, 20, 55, 0, 0, 20, 0.25},
		{"already grey", 55, 99, 0, 55, 0, 0, 0, 0},
	} {
		crafts, reaches := craftsFor(r, tc.have, tc.target)
		o, y, g := splitCrafts(r, tc.have, reaches)
		if crafts != tc.crafts || reaches != tc.reaches || o != tc.o || y != tc.y || g != tc.grn || o+y+g != crafts {
			t.Errorf("%s: crafts %d reach %d split %d/%d/%d, want %d reach %d split %d/%d/%d", tc.name, crafts, reaches, o, y, g,
				tc.crafts, tc.reaches, tc.o, tc.y, tc.grn)
		}
		if c := chanceAt(r, tc.have); c != tc.chance {
			t.Errorf("%s: chance at %d is %v, want %v", tc.name, tc.have, c, tc.chance)
		}
	}
	if c, _ := craftsFor(Recipe{}, 1, 50); c != 0 {
		t.Errorf("a recipe with no colours gives %d crafts", c)
	}
}

type nodeRow struct {
	ID    int64  `parquet:"node_id"`
	Name  string `parquet:"name"`
	Prof  string `parquet:"profession"`
	Skill *int16 `parquet:"skill_required,optional"`
	Zone  string `parquet:"zone_name"`
}

// A gathering profession with no recipe for a stretch gets nodes to gather, never a pattern from the Auction House.
func TestGatheringProfessionNamesNodesNotPatterns(t *testing.T) {
	ds := store.New(store.ReaderSource{
		"recipes": table(t, []recipeRow{{SpellID: 1, SkillLineID: 1, SkillLineName: "Mining", Name: str("Smelt Copper"), LearnSkill: i16(1), YellowSkill: i16(25), GreySkill: i16(45)}}),
		"gathering_nodes": table(t, []nodeRow{{1, "Copper Vein", "Mining", i16(1), "Durotar"}, {2, "Tin Vein", "Mining", i16(65), "The Barrens"},
			{3, "Tin Vein", "Mining", i16(65), "Silverpine Forest"}, {4, "Le Fer", "Mining", nil, "Orgrimmar"}}),
		"profession_notes": table(t, rankTiers()),
	})
	text := RenderBenchmark(NewBenchmark(ds, BenchOptions{Profession: "Mining", From: 60, To: 75}))
	for _, want := range []string{"Mining levels by gathering, not crafting", "Tin Vein (needs skill 65; The Barrens, Silverpine Forest)", "Copper Vein (needs skill 1; Durotar)"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "pattern at the Auction House") || strings.Contains(text, "Le Fer") {
		t.Errorf("pattern advice or an unusable node in:\n%s", text)
	}
}

// A trainer the dataset has no name for is named by its id, not "trainer trainer 12".
func TestUnnamedTrainerShowsItsID(t *testing.T) {
	ds := store.New(store.ReaderSource{
		"trainers": table(t, []pTrainer{{12, "", "Tailoring", "Undercity", 3}}),
	})
	p := &planner{ds: ds, prof: "Tailoring"}
	if got := p.trainer(); got != "npc 12 (name not in the dataset) in Undercity" {
		t.Errorf("trainer %q", got)
	}
}
