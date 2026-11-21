package professions

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type recipeRow struct {
	SpellID       int32   `parquet:"spell_id"`
	SkillLineID   int32   `parquet:"skill_line_id"`
	SkillLineName string  `parquet:"skill_line_name"`
	Name          *string `parquet:"name,optional"`
	CreatesItemID *int32  `parquet:"creates_item_id,optional"`
	LearnSkill    *int16  `parquet:"learn_skill,optional"`
	YellowSkill   *int16  `parquet:"yellow_skill,optional"`
	GreySkill     *int16  `parquet:"grey_skill,optional"`
}

type reagentRow struct {
	SpellID       int32 `parquet:"spell_id"`
	ReagentItemID int32 `parquet:"reagent_item_id"`
	Count         int16 `parquet:"count"`
}

type itemRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

func table[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func i16(n int16) *int16   { return &n }
func i32(n int32) *int32   { return &n }
func str(s string) *string { return &s }
func ip(n int) *int        { return &n }

func rec(spell int32, line, name string, create *int32, learn, yellow, grey *int16) recipeRow {
	return recipeRow{spell, 1, line, str(name), create, learn, yellow, grey}
}

// Tailoring: skill 40 sees orange (yellow 60+), yellow (yellow<=40<grey), grey, too-hard and
// threshold-less recipes. Enchanting and Leatherworking are small.
func synth(t *testing.T, withRecipes bool) store.DataStore {
	src := store.ReaderSource{"items": table(t, []itemRow{
		{1, "Linen Cloth"}, {2, "Coarse Thread"}, {3, "Bolt of Linen"}, {4, "Strange Dust"}, {5, "Light Leather"},
		{6, "Greater Dust"}, {10, "Linen Bag"}, {11, "Reinforced Linen Cape"}, {12, "Leather Belt"},
	})}
	if withRecipes {
		src["recipes"] = table(t, []recipeRow{
			rec(100, "Tailoring", "Bolt of Linen Cloth", i32(3), i16(1), i16(45), i16(70)), // orange
			rec(101, "Tailoring", "Linen Belt", nil, i16(30), i16(50), i16(80)),            // orange, higher learn
			rec(102, "Tailoring", "Linen Cape", i32(11), i16(35), i16(40), i16(65)),        // yellow
			rec(103, "Tailoring", "Linen Bag", i32(10), i16(20), i16(30), i16(45)),         // yellow, lower learn
			rec(104, "Tailoring", "Old Shirt", nil, i16(1), i16(20), i16(40)),              // grey at 40
			rec(105, "Tailoring", "Fancy Robe", nil, i16(60), i16(70), i16(90)),            // too hard
			rec(106, "Tailoring", "Mystery Hat", nil, i16(25), nil, nil),                   // no thresholds
			rec(107, "Tailoring", "Orange Tie", nil, i16(30), i16(50), i16(80)),            // ties with 101
			rec(108, "Tailoring", "Late Orange", nil, i16(10), i16(90), i16(120)),          // orange, low learn
			rec(200, "Enchanting", "Enchant Bracer", nil, i16(1), i16(70), i16(90)),        // orange at 1
			rec(201, "Enchanting", "Enchant Chest", nil, i16(61), i16(80), i16(100)),       // learnable at 61
			rec(300, "Leatherworking", "Leather Belt", i32(12), i16(1), i16(55), i16(80)),  // orange
			rec(301, "Leatherworking", "Far Boots", nil, i16(120), i16(130), i16(150)),     // beyond keep window
		})
		src["recipe_reagents"] = table(t, []reagentRow{
			{100, 1, 2}, {101, 2, 1}, {101, 3, 2}, {102, 3, 1}, {103, 1, 5}, {104, 1, 1}, {105, 1, 9},
			{107, 3, 1}, {108, 1, 1}, {200, 4, 2}, {201, 6, 1}, {300, 5, 4}, {301, 5, 99},
		})
	}
	return store.New(src)
}

func stack(id, n int, name string) character.ItemStack {
	return character.ItemStack{ItemID: id, Count: n, Name: name}
}

func priest(bank bool) character.Character {
	return character.Character{
		Name: "Testpriest", Class: "PRIEST",
		Professions:  []character.Skill{{Name: "Tailoring", Rank: ip(40), Max: ip(75)}, {Name: "Enchanting", Rank: ip(10), Max: ip(75)}},
		Bags:         []character.ItemStack{stack(1, 6, "Linen Cloth"), stack(4, 1, "Strange Dust"), stack(6, 2, "Greater Dust"), stack(99, 3, "Junk")},
		Bank:         []character.ItemStack{stack(1, 4, "Linen Cloth"), stack(3, 1, "Bolt of Linen")},
		BankCaptured: bank,
	}
}

func rogue() character.Character {
	return character.Character{
		Name: "Testrogue", Class: "ROGUE", BankCaptured: true,
		Professions: []character.Skill{{Name: "Leatherworking", Rank: ip(50), Max: ip(75)}},
		Bags:        []character.ItemStack{stack(5, 3, "Light Leather"), stack(1, 2, "Linen Cloth")},
	}
}

func stepNames(p Leveling) string {
	var out []string
	for _, s := range p.Steps {
		out = append(out, s.Recipe.Name+":"+s.Difficulty)
	}
	return strings.Join(out, ",")
}

func TestSelect(t *testing.T) {
	recipes, err := Recipes(synth(t, true), "tailoring") // case-insensitive profession
	if err != nil || len(recipes) != 9 {
		t.Fatalf("recipes = %d, %v", len(recipes), err)
	}
	cases := []struct {
		skill int
		want  string
	}{
		// Orange by learn skill descending then spell id, then yellow, then unknown; grey and
		// too-hard recipes dropped; at most MaxRecipes.
		{40, "Linen Belt:orange,Orange Tie:orange,Late Orange:orange,Bolt of Linen Cloth:orange,Linen Cape:yellow"},
		{44, "Linen Belt:orange,Orange Tie:orange,Late Orange:orange,Bolt of Linen Cloth:orange,Linen Cape:yellow"},
		{1, "Bolt of Linen Cloth:orange,Old Shirt:orange"},
		{100, "Late Orange:yellow,Mystery Hat:unknown"},
		{125, "Mystery Hat:unknown"},
	}
	for _, c := range cases {
		var got []string
		for _, s := range Select(recipes, c.skill) {
			got = append(got, s.Recipe.Name+":"+s.Difficulty)
		}
		if strings.Join(got, ",") != c.want {
			t.Errorf("skill %d: got %v, want %s", c.skill, got, c.want)
		}
	}
}

func TestDifficulty(t *testing.T) {
	r := Recipe{YellowSkill: ip(50), GreySkill: ip(80)}
	cases := []struct {
		r     Recipe
		skill int
		want  string
	}{
		{r, 49, Orange}, {r, 50, Yellow}, {r, 79, Yellow}, {r, 80, ""},
		{Recipe{YellowSkill: ip(50)}, 10, Unknown}, {Recipe{GreySkill: ip(50)}, 10, Unknown}, {Recipe{}, 10, Unknown},
	}
	for _, c := range cases {
		if got := difficulty(c.r, c.skill); got != c.want {
			t.Errorf("difficulty(%+v, %d) = %q, want %q", c.r, c.skill, got, c.want)
		}
	}
}

func TestPlan(t *testing.T) {
	ds := synth(t, true)
	p := Plan(priest(true), ds, "Tailoring")
	if got := stepNames(p); got != "Linen Belt:orange,Orange Tie:orange,Late Orange:orange,Bolt of Linen Cloth:orange,Linen Cape:yellow" {
		t.Errorf("tailoring steps = %s", got)
	}
	if !p.BankKnown || len(p.Notes) != 0 {
		t.Errorf("notes = %v", p.Notes)
	}
	// Linen Belt: Coarse Thread x1 (none held), Bolt of Linen x2 (the bank has 1).
	belt := p.Steps[0]
	if len(belt.Reagents) != 2 || belt.Reagents[0] != (Reagent{2, "Coarse Thread", 1, 0, 1}) || belt.Reagents[1] != (Reagent{3, "Bolt of Linen", 2, 1, 1}) {
		t.Errorf("belt reagents = %+v", belt.Reagents)
	}
	// Bolt of Linen Cloth: Linen Cloth x2; bags 6 plus bank 4 are held. It makes a named item.
	bolt := p.Steps[3]
	if bolt.CreatesName != "Bolt of Linen" || bolt.Reagents[0].Have != 10 || bolt.Reagents[0].Missing != 0 {
		t.Errorf("bolt = %+v", bolt)
	}

	// Bank not captured: the plan says so.
	p = Plan(priest(false), ds, "tailoring")
	if p.BankKnown || !strings.Contains(strings.Join(p.Notes, "|"), "Bank not captured") {
		t.Errorf("bank notes = %v", p.Notes)
	}

	// Enchanting at skill 10: one orange recipe that makes no item; Enchant Chest needs skill 61.
	p = Plan(priest(true), ds, "Enchanting")
	if stepNames(p) != "Enchant Bracer:orange" || p.Steps[0].CreatesName != "" || p.Steps[0].Reagents[0].Missing != 1 {
		t.Errorf("enchanting = %s %+v", stepNames(p), p.Steps)
	}

	// Rogue Leatherworking at 50: Leather Belt needs 4 leather, 3 held.
	p = Plan(rogue(), ds, "Leatherworking")
	if stepNames(p) != "Leather Belt:orange" || p.Steps[0].Reagents[0].Missing != 1 || len(p.Notes) != 0 {
		t.Errorf("leatherworking = %s %+v notes %v", stepNames(p), p.Steps, p.Notes)
	}
}

func TestPlanUnknowns(t *testing.T) {
	// Skill 125: only the threshold-less recipe remains, and the dataset lists no reagents for it.
	c := priest(true)
	c.Professions[0].Rank = ip(125)
	p := Plan(c, synth(t, true), "Tailoring")
	joined := strings.Join(p.Notes, "|")
	if stepNames(p) != "Mystery Hat:unknown" || p.Steps[0].ReagentsKnown || !strings.Contains(joined, "no skill-up thresholds") || !strings.Contains(joined, "list no reagents") {
		t.Errorf("unknowns: %s notes %v", stepNames(p), p.Notes)
	}
	if text := Render([]Leveling{p}); !strings.Contains(text, "reagents unknown") || !strings.Contains(text, "[unknown]") {
		t.Errorf("render = %q", text)
	}

	cases := []struct {
		name string
		c    character.Character
		ds   store.DataStore
		prof string
		note string
	}{
		{"no recipes table", priest(true), synth(t, false), "Tailoring", "has no recipes"},
		{"nil dataset", priest(true), nil, "Tailoring", "has no recipes"},
		{"profession not captured", rogue(), synth(t, true), "Tailoring", "Skill level not captured"},
		{"skill not captured", character.Character{Professions: []character.Skill{{Name: "Tailoring"}}}, synth(t, true), "Tailoring", "Skill level not captured"},
	}
	for _, tc := range cases {
		p := Plan(tc.c, tc.ds, tc.prof)
		if len(p.Steps) != 0 || len(p.Notes) == 0 || !strings.Contains(p.Notes[0], tc.note) {
			t.Errorf("%s: steps %d notes %v", tc.name, len(p.Steps), p.Notes)
		}
	}
	if !strings.Contains(Render(nil), "No professions") {
		t.Error("Render(nil) names no professions")
	}
}

func keepString(ks []Keep) string {
	var out []string
	for _, k := range ks {
		out = append(out, k.Name+" "+strings.Join(k.Professions, "+")+"="+strconv.Itoa(k.Count))
	}
	return strings.Join(out, ",")
}

func TestKeepList(t *testing.T) {
	ds := synth(t, true)
	with := func(tailoring, enchanting int) character.Character {
		c := priest(true)
		c.Professions[0].Rank, c.Professions[1].Rank = ip(tailoring), ip(enchanting)
		c.Bags = append(c.Bags, stack(2, 1, "Coarse Thread"))
		return c
	}
	cases := []struct {
		name string
		c    character.Character
		want string
	}{
		// Reagents of Tailoring and Enchanting recipes in reach; Junk is no reagent; the bank counts.
		{"priest", priest(true), "Linen Cloth Tailoring=10,Bolt of Linen Tailoring=1,Strange Dust Enchanting=1"},
		{"rogue", rogue(), "Light Leather Leatherworking=3"},
		// Window: Enchant Chest (learn 61) needs Greater Dust; in reach from skill 11.
		{"enchanting 10", with(40, 10), "Linen Cloth Tailoring=10,Bolt of Linen Tailoring=1,Coarse Thread Tailoring=1,Strange Dust Enchanting=1"},
		{"enchanting 11", with(40, 11), "Linen Cloth Tailoring=10,Greater Dust Enchanting=2,Bolt of Linen Tailoring=1,Coarse Thread Tailoring=1,Strange Dust Enchanting=1"},
		// Grey: Coarse Thread is only in Linen Belt, grey at 80.
		{"tailoring 79", with(79, 10), "Linen Cloth Tailoring=10,Bolt of Linen Tailoring=1,Coarse Thread Tailoring=1,Strange Dust Enchanting=1"},
		{"tailoring 80", with(80, 10), "Linen Cloth Tailoring=10,Strange Dust Enchanting=1"},
	}
	for _, tc := range cases {
		ks, err := KeepList(tc.c, ds)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := keepString(ks); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
	// A bag/bank split is kept.
	ks, _ := KeepList(priest(true), ds)
	if ks[0].Bags != 6 || ks[0].Bank != 4 {
		t.Errorf("split = %+v", ks[0])
	}
	for _, d := range []store.DataStore{nil, synth(t, false)} {
		if _, err := KeepList(priest(true), d); err != ErrNoRecipes {
			t.Errorf("no recipes: err = %v", err)
		}
	}
}
