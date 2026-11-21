package eval

// The dataset tables and bag contents the newer tools read: recipes, talent builds, dungeons and
// vendor prices. Like the rest of the fixtures they are invented and built in code.

import (
	"fmt"
	"sort"
)

// ---- professions ----

// profDef is a profession the dataset holds recipes for. Primary is the reagent of the recipes in
// reach; Secondary (when set) only of a recipe far above any character's skill.
type profDef struct{ name, primary, secondary string }

var profDefs = []profDef{
	{"Tailoring", "Linen Cloth", "Silk Thread"},
	{"Enchanting", "Strange Dust", ""},
	{"Blacksmithing", "Copper Bar", "Copper Ore"},
	{"Alchemy", "Peacebloom", "Empty Vial"},
}

// profDefOf is nil for a profession with no recipes in the dataset (the gatherers).
func profDefOf(name string) *profDef {
	for i := range profDefs {
		if profDefs[i].name == name {
			return &profDefs[i]
		}
	}
	return nil
}

// recipeSpec is one recipe of every profession. yellow and grey are 0 where the dataset has no
// skill-up thresholds; p and s are the counts of the primary and secondary reagent.
type recipeSpec struct{ k, learn, yellow, grey, p, s int }

var recipeSpecs = []recipeSpec{
	{1, 1, 30, 60, 2, 0},     // yellow at skill 40
	{2, 25, 50, 70, 3, 0},    // orange
	{3, 40, 45, 65, 1, 0},    // orange, learnable exactly at skill 40
	{4, 60, 70, 90, 1, 0},    // not learnable yet, but within reach
	{5, 20, 25, 35, 1, 0},    // grey
	{6, 30, 0, 0, 0, 0},      // no thresholds, no reagents
	{7, 120, 125, 140, 0, 1}, // far above the skill
}

func spellID(pi int, k int) int64 { return int64(100000 + pi*100 + k) }

// bagNames is every item name a capture's bags hold, so a name keeps one id across characters.
func bagNames() []string {
	set := map[string]bool{}
	for _, s := range charSpecs {
		for n := range s.bags {
			set[n] = true
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func bagItemID(name string) int64 {
	for i, n := range bagNames() {
		if n == name {
			return int64(800001 + i)
		}
	}
	panic("no bag item " + name)
}

func i32p(n int) *int32 {
	if n == 0 {
		return nil
	}
	return i32(n)
}

func professionTables() (recipes []recipeRow, reagents []reagentRow, items []itemRow) {
	seen := map[string]bool{}
	reagent := func(pi, k int, name string, n int) {
		if name == "" || n == 0 {
			return
		}
		reagents = append(reagents, reagentRow{spellID(pi, k), bagItemID(name), int32(n)})
		if !seen[name] {
			seen[name] = true
			p := int64(10)
			items = append(items, itemRow{ID: bagItemID(name), Name: name, Quality: 1, ClassID: 7, StatsStatus: "known", SellPrice: &p})
		}
	}
	for pi, d := range profDefs {
		for _, r := range recipeSpecs {
			recipes = append(recipes, recipeRow{spellID(pi, r.k), fmt.Sprintf("Eval %s Recipe %d", d.name, r.k), d.name, i32p(r.learn), i32p(r.yellow), i32p(r.grey)})
			reagent(pi, r.k, d.primary, r.p)
			reagent(pi, r.k, d.secondary, r.s)
		}
	}
	return
}

// ---- talents ----

var (
	buildBands  = []string{"10-19", "20-29", "30-39", "40-49"}
	buildPoints = []int{2, 1, 2} // the steps of a band
	classSpecs  = map[string][]string{"PRIEST": {"Discipline", "Holy", "Shadow"}, "ROGUE": {"Assassination", "Combat", "Subtlety"}}
)

func nodeID(class string, spec, j int) int64 {
	return int64(map[string]int{"PRIEST": 1000, "ROGUE": 2000}[class] + spec*100 + j%4 + 1)
}

func talentTables() (builds []buildRow, nodes []nodeRow) {
	for _, class := range []string{"PRIEST", "ROGUE"} {
		for si, spec := range classSpecs[class] {
			order := int64(0)
			for _, band := range buildBands {
				for _, pts := range buildPoints {
					builds = append(builds, buildRow{class, spec, band, "Eval rationale " + band, order + 1, nodeID(class, si, int(order)), int32(pts)})
					order++
				}
			}
			for j := 0; j < 4; j++ {
				id := nodeID(class, si, j)
				nodes = append(nodes, nodeRow{id, fmt.Sprintf("Eval %s %s node %d", class, spec, id)})
			}
		}
	}
	return
}

// ---- dungeons ----

// dungeonSpec is one dungeon of every band: its map id is band*100+k, its levels offsets from the
// band, known false when the dataset holds no level range.
type dungeonSpec struct {
	k      int
	name   string
	typ    string
	lo, hi int
	known  bool
	boss   bool // has a boss linked to the creature the band's items drop from
	ghost  bool // and one with no creature
}

var dungeonSpecs = []dungeonSpec{
	{1, "Deadmine", "dungeon", -2, 2, true, true, true},
	{2, "Unmapped Crypt", "dungeon", 0, 0, false, true, false},
	{3, "Far Keep", "dungeon", 10, 14, true, true, false},
	{4, "Edge Hold", "dungeon", 3, 6, true, true, false},
	{5, "Raid Spire", "raid", -2, 2, true, true, false},
	{6, "Low Cave", "dungeon", -12, -6, true, true, false},
}

func dungeonName(b int, d dungeonSpec) string { return fmt.Sprintf("Eval %s %d", d.name, b) }

// questDungeon is the dungeon (by k) each quest of a band points at in the dataset.
var questDungeon = map[int]int{q1: 1, q3: 1, q7: 1, q5: 4}

func dungeonTables(b int) (inst []instanceRow, bosses []bossRow) {
	near := int64(b*10 + 1)
	for _, d := range dungeonSpecs {
		id := int64(b*100 + d.k)
		row := instanceRow{MapID: id, Name: dungeonName(b, d), Type: d.typ, MaxPlayers: i32(5)}
		if d.known {
			row.MinLevel, row.MaxLevel = i32(b+d.lo), i32(b+d.hi)
		}
		inst = append(inst, row)
		if d.boss {
			bosses = append(bosses, bossRow{id, fmt.Sprintf("Eval Chief %d-%d", b, d.k), &near})
		}
		if d.ghost {
			bosses = append(bosses, bossRow{id, fmt.Sprintf("Eval Shade %d-%d", b, d.k), nil})
		}
	}
	return
}

// stash is what each capture carries besides its materials, as band item numbers: a grey item with
// a price, a grey item without one, and two armor pieces that are not upgrades for some class.
var stash = []int{23, 26, 5, 1}
