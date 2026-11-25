package professions

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type pRecipeSrc struct {
	SpellID       int32   `parquet:"spell_id"`
	SkillLineName string  `parquet:"skill_line_name"`
	Name          *string `parquet:"name,optional"`
	CreatesItemID *int32  `parquet:"creates_item_id,optional"`
	LearnSkill    *int16  `parquet:"learn_skill,optional"`
	LearnSource   string  `parquet:"learn_skill_source"`
	YellowSkill   *int16  `parquet:"yellow_skill,optional"`
	GreySkill     *int16  `parquet:"grey_skill,optional"`
	LearnedFrom   string  `parquet:"learned_from"`
}

func rs(spell int32, line, name string, create *int32, learn int16, src string, yellow, grey int16, from string) pRecipeSrc {
	return pRecipeSrc{spell, line, str(name), create, i16(learn), src, i16(yellow), i16(grey), from}
}

// chainData is a small Blacksmithing and Mining world: bars smelted from ore, a grinding stone made
// from stone, a recipe whose learn skill is estimated and a sword learnt from a drop.
func chainData(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"items": table(t, []pItem{
			{1, "Copper Ore", 1, 1, 7}, {2, "Copper Bar", 1, 1, 7}, {3, "Rough Stone", 1, 1, 7}, {4, "Rough Grinding Stone", 1, 1, 7},
			{9, "Plans: Drop Blade", 1, 1, 9},
		}),
		"recipes": table(t, []pRecipeSrc{
			rs(100, "Blacksmithing", "Rough Grinding Stone", i32(4), 1, "trainer", 45, 60, "trainer"),
			rs(101, "Blacksmithing", "Copper Shortsword", nil, 1, "trainer", 30, 60, "trainer"),
			rs(102, "Blacksmithing", "Guessed Sword", nil, 40, "estimate", 60, 90, "trainer"),
			rs(103, "Blacksmithing", "Drop Blade", nil, 20, "recipe_item", 40, 100, "item"),
			rs(104, "Blacksmithing", "Heavy Sword", nil, 70, "trainer", 80, 110, "trainer"),
			rs(105, "Blacksmithing", "Sharpened Sword", nil, 1, "trainer", 20, 50, "trainer"),
			rs(200, "Mining", "Smelt Copper", i32(2), 1, "trainer", 25, 47, "trainer"),
		}),
		"recipe_reagents": table(t, []reagentRow{
			{100, 3, 1}, {101, 2, 6}, {102, 2, 8}, {103, 2, 2}, {104, 2, 10}, {105, 4, 1}, {105, 2, 1}, {200, 1, 1},
		}),
		"vendor_sources":   table(t, []pVendor{{1, 3, 10}}),
		"recipe_items":     table(t, []pRecipeItem{{9, 103, 0, "Blacksmithing"}}),
		"loot_sources":     table(t, []pLoot{{9, "creature", 900, 100, 3}}),
		"npcs":             table(t, []pNpc{{900, "Rare Smith"}}),
		"gathering_yields": table(t, []pYield{{50, 1}}),
		"gathering_nodes":  table(t, []pNode{{50, "Mining", "Copper Vein", "Elwynn Forest", 1}}),
		"trainers":         table(t, []pTrainer{{1, "Smith Sam", "Blacksmithing", "Goldshire", 5}}),
		"trainer_spells":   table(t, []pTrainerSpell{{1, 101, 100}}),
		"profession_notes": table(t, rankTiers()),
	})
}

func smith(skill, level int, known ...int64) character.Character {
	c := character.Character{Name: "Smith", Level: ip(level), Professions: []character.Skill{{Name: "Blacksmithing", Rank: ip(skill)}}}
	if len(known) > 0 {
		c.KnownSpells = map[int64]bool{}
		for _, k := range known {
			c.KnownSpells[k] = true
		}
	}
	return c
}

func TestNoRecipeBeforeItsLearnSkill(t *testing.T) {
	b := NewBenchmark(chainData(t), BenchOptions{Profession: "Blacksmithing"})
	for _, x := range b.Bands {
		if x.LearnSkill != nil && *x.LearnSkill > x.From {
			t.Errorf("%s planned at skill %d, learnt at %d", x.Recipe, x.From, *x.LearnSkill)
		}
	}
}

func TestOrangeStartsAtLearnSkill(t *testing.T) {
	ds := chainData(t)
	has := func(skill int) bool {
		b := ComputePath(smith(skill, 60), ds, "Blacksmithing", 80, 1).Bands[0]
		return b.Best.Recipe == "Heavy Sword" || b.Or != nil && b.Or.Recipe == "Heavy Sword"
	}
	if has(69) {
		t.Error("Heavy Sword offered at 69, below its learn skill 70")
	}
	if !has(70) {
		t.Error("Heavy Sword not offered at its learn skill 70")
	}
}

func TestBandStopsAtRankCap(t *testing.T) {
	pa := ComputePath(smith(70, 60), chainData(t), "Blacksmithing", 120, 1)
	if len(pa.Bands) < 2 {
		t.Fatalf("bands %+v", pa.Bands)
	}
	for _, b := range pa.Bands {
		if b.From < 75 && b.To > 75 {
			t.Errorf("band %d→%d crosses 75", b.From, b.To)
		}
	}
	if b := pa.Bands[1]; b.From != 75 || b.Rank != "Journeyman" || b.RankLevel != 10 {
		t.Errorf("second band %+v", b)
	}
	if text := RenderPath(pa); !strings.Contains(text, "First train Journeyman at your trainer (character level 10)") {
		t.Errorf("rank not named:\n%s", text)
	}
}

func TestDropPatternOnlyWhenKnown(t *testing.T) {
	ds := chainData(t)
	pa := ComputePath(smith(30, 60), ds, "Blacksmithing", 60, 1)
	var listed bool
	for _, b := range pa.Bands {
		if b.Best.Recipe == "Drop Blade" {
			t.Errorf("drop recipe planned without the pattern: %+v", b)
		}
		for _, p := range b.Patterns {
			listed = listed || p.Recipe == "Drop Blade"
		}
	}
	if !listed {
		t.Error("Drop Blade is not listed as an if-you-have-the-pattern alternative")
	}
	if text := RenderPath(pa); !strings.Contains(text, "If you have the pattern") {
		t.Errorf("pattern line missing:\n%s", text)
	}
	b := NewBenchmark(ds, BenchOptions{Profession: "Blacksmithing", From: 60, To: 75, Known: []int64{103}})
	used := false
	for _, x := range b.Bands {
		used = used || x.Recipe == "Drop Blade"
	}
	if !used {
		t.Errorf("known pattern not used:\n%s", RenderBenchmark(b))
	}
}

func TestBarExpandsToOre(t *testing.T) {
	b := NewBenchmark(chainData(t), BenchOptions{Profession: "Blacksmithing", From: 60, To: 75})
	var bar *BenchMake
	for _, x := range b.Bands {
		for _, m := range x.Materials {
			if m.Item == "Copper Bar" {
				bar = m.Make
			}
		}
	}
	if bar == nil || bar.Verb != "smelt" || bar.Profession != "Mining" || bar.Materials[0].Item != "Copper Ore" {
		t.Fatalf("bar not expanded to ore: %+v\n%s", bar, RenderBenchmark(b))
	}
	for _, m := range b.Materials {
		if m.Rare {
			t.Errorf("%s is marked rare", m.Item)
		}
		if m.Item == "Copper Ore" && m.Kind != "gathering" {
			t.Errorf("ore is %s, want gathering", m.Kind)
		}
	}
}

func TestGrindingStoneCountsSkillUps(t *testing.T) {
	ds := chainData(t)
	p := &planner{c: smith(30, 60), ds: ds, notes: LoadNotes(ds), prof: "Blacksmithing", skill: 30, known: map[int64]bool{},
		learnt: map[int64]bool{}, names: map[int64]string{}, srcs: map[int64]source{}}
	_, ch := p.resolve(4, 30, 0)
	if ch == nil {
		t.Fatal("the grinding stone is not made from stone")
	}
	m := p.expand(ch, 10, 30, 0)
	if m.SkillUps != 10 || m.Needs[0].Item != "Rough Stone" || m.Needs[0].Total != 10 {
		t.Errorf("make %+v", m)
	}
	if m := p.expand(ch, 10, 55, 0); m.SkillUps != 5 {
		t.Errorf("yellow crafts give %v skill-ups, want 5", m.SkillUps)
	}
	if m := p.expand(ch, 10, 70, 0); m.SkillUps != 0 {
		t.Errorf("grey crafts give %v skill-ups, want 0", m.SkillUps)
	}
}

func TestEstimatedLearnSkillIsAround(t *testing.T) {
	ds := chainData(t)
	b := NewBenchmark(ds, BenchOptions{Profession: "Blacksmithing", From: 45, To: 75})
	text := RenderBenchmark(b)
	if !strings.Contains(text, "Guessed Sword: learn around skill 40, estimated") {
		t.Errorf("estimate not shown as around:\n%s", text)
	}
	c := smith(45, 60, 999) // a captured known-recipe list, so every recipe says how to learn it
	if !strings.Contains(RenderPath(ComputePath(c, ds, "Blacksmithing", 75, 1)), "around skill 40") {
		t.Error("path does not say around skill 40")
	}
	for _, x := range b.Bands {
		if x.Recipe == "Guessed Sword" && x.LearnSkillSource != "estimate" {
			t.Errorf("source %q", x.LearnSkillSource)
		}
	}
}
