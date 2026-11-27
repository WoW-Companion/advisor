package gather

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Everything here is invented: ids, names and places.

type recipeRow struct {
	SpellID       int32  `parquet:"spell_id"`
	SkillLineName string `parquet:"skill_line_name"`
}

type reagentRow struct {
	SpellID       int32 `parquet:"spell_id"`
	ReagentItemID int32 `parquet:"reagent_item_id"`
}

type noteRow struct {
	Section  string `parquet:"section"`
	Subject  string `parquet:"subject"`
	SkillMin *int16 `parquet:"skill_min,optional"`
}

func table[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func dataset(t *testing.T) store.DataStore {
	s1, s100 := int16(1), int16(100)
	return store.New(store.ReaderSource{
		"recipes":         table(t, []recipeRow{{1, "Mining"}, {2, "Alchemy"}}),
		"recipe_reagents": table(t, []reagentRow{{1, 10}, {2, 20}}),
		"profession_notes": table(t, []noteRow{
			{"nodes", "Tin Seam", &s100}, {"nodes", "Copper Seam", &s1}, {"ranks", "Apprentice", nil},
		}),
	})
}

const (
	isle, farland = 901, 903 // area ids
)

// index: copper ore (item 10) comes from two vein ids, one in each zone; Bloomweed (item 20) from
// a herb node in the isle; Tinfish (item 30) from a school in both zones; a chest holds a herb.
func index() *questie.Index {
	sp := func(area int, xy ...float64) []questie.Spawn {
		var out []questie.Spawn
		for i := 0; i+1 < len(xy); i += 2 {
			out = append(out, questie.Spawn{Area: area, X: xy[i], Y: xy[i+1]})
		}
		return out
	}
	join := func(a, b []questie.Spawn) []questie.Spawn { return append(append([]questie.Spawn{}, a...), b...) }
	return &questie.Index{
		Areas: map[int]questie.Area{isle: {UIMap: 7001, Zone: "Testisle"}, farland: {UIMap: 7002, Zone: "Farland"}},
		Nodes: map[int]questie.Node{
			1: {ID: 1, Name: "Copper Seam", Kind: questie.KindOre, Spawns: sp(isle, 10, 10, 12, 12, 14, 14),
				Yields: []questie.Yield{{Item: 10, Name: "Copper Ore", Level: 1}}},
			2: {ID: 2, Name: "Copper Seam", Kind: questie.KindOre, Spawns: sp(farland, 50, 50),
				Yields: []questie.Yield{{Item: 10, Name: "Copper Ore", Level: 1}}},
			3: {ID: 3, Name: "Tin Seam", Kind: questie.KindOre, Spawns: sp(farland, 60, 60, 61, 61),
				Yields: []questie.Yield{{Item: 11, Name: "Tin Ore", Level: 40}}},
			4: {ID: 4, Name: "Bloomweed Patch", Spawns: sp(isle, 30, 30),
				Yields: []questie.Yield{{Item: 20, Name: "Bloomweed", Level: 1}}},
			5: {ID: 5, Name: "School of Tinfish", Kind: questie.KindFish, Spawns: join(sp(isle, 80, 80), sp(farland, 20, 20, 22, 22, 24, 24)),
				Yields: []questie.Yield{{Item: 30, Name: "Raw Tinfish", Level: 1}}},
			6: {ID: 6, Name: "Dusty Chest", Spawns: sp(isle, 5, 5), Yields: []questie.Yield{{Item: 20, Name: "Bloomweed", Level: 1}}},
			7: {ID: 7, Name: "Mystery Crate Item", Spawns: sp(isle, 6, 6), Yields: []questie.Yield{{Item: 99, Name: "Odd Trinket"}}},
		},
	}
}

func char(zone string, mining, fishing int) character.Character {
	x, y := 50.0, 50.0
	c := character.Character{Name: "Gath", Professions: []character.Skill{
		{Name: "Mining", Rank: &mining}, {Name: "Fishing", Rank: &fishing}}}
	if zone != "" {
		c.Location = &character.Location{Zone: zone, X: &x, Y: &y}
	}
	return c
}

func TestClassification(t *testing.T) {
	g := New(index(), dataset(t))
	kinds := map[int]string{}
	for _, n := range g.nodes {
		kinds[n.ID] = n.Kind
	}
	want := map[int]string{1: Ore, 2: Ore, 3: Ore, 4: Herb, 5: Fish}
	for id, k := range want {
		if kinds[id] != k {
			t.Errorf("node %d is %q, want %q", id, kinds[id], k)
		}
	}
	if _, ok := kinds[6]; ok {
		t.Error("a chest is not a node")
	}
	if _, ok := kinds[7]; ok {
		t.Error("an object yielding a trinket is not a node")
	}
}

func TestFindItemYieldedByTwoNodeIDsNamesBothZones(t *testing.T) {
	g := New(index(), dataset(t))
	r := g.Find(char("", 1, 1), "copper", "mine")
	if r.Item != "Copper Ore" || len(r.Zones) != 2 {
		t.Fatalf("%+v", r)
	}
	if r.Zones[0].Zone != "Testisle" || r.Zones[0].Spawns != 3 || r.Zones[1].Spawns != 1 {
		t.Fatalf("zones %+v", r.Zones)
	}
	if len(r.Zones[0].Coords) != questie.MaxSpots || !strings.Contains(r.Text, "from your Questie database") || !strings.Contains(r.Text, "(10, 10)") {
		t.Fatalf("text %q", r.Text)
	}
}

func TestFindNearestZoneFirst(t *testing.T) {
	g := New(index(), dataset(t))
	r := g.Find(char("Farland", 1, 1), "Raw Tinfish", "fish")
	if len(r.Zones) != 2 || r.Zones[0].Zone != "Farland" || !r.Zones[0].Here {
		t.Fatalf("farland should come first: %+v", r.Zones)
	}
	// Within the character's zone the spot nearest them leads.
	if c := r.Zones[0].Coords[0]; c != [2]float64{24, 24} {
		t.Fatalf("nearest spot %v", c)
	}
}

func TestFindKindFilterAndHerb(t *testing.T) {
	g := New(index(), dataset(t))
	if r := g.Find(char("", 1, 1), "Bloomweed", "pick"); len(r.Zones) != 1 || r.Kind != Herb || r.Zones[0].Zone != "Testisle" {
		t.Fatalf("%+v", r)
	}
	if r := g.Find(char("", 1, 1), "Bloomweed", "mine"); len(r.Zones) != 0 {
		t.Fatalf("a herb is not mined: %+v", r)
	}
}

func TestForLevelUsesCuratedNodeSkills(t *testing.T) {
	g := New(index(), dataset(t))
	low := g.ForLevel(char("", 20, 1), Ore)
	if len(low.Zones) != 2 || strings.Contains(strings.Join(low.Zones[0].Nodes, ","), "Tin") {
		t.Fatalf("tin needs 100: %+v", low.Zones)
	}
	for _, z := range low.Zones {
		for _, n := range z.Nodes {
			if n == "Tin Seam" {
				t.Fatalf("tin listed at 20: %+v", low.Zones)
			}
		}
	}
	high := g.ForLevel(char("", 120, 1), Ore)
	if len(high.Zones) == 0 || high.Zones[0].Zone != "Farland" || high.Zones[0].Nodes[0] != "Tin Seam" {
		t.Fatalf("at 120 the tin zone leads: %+v", high.Zones)
	}
	if !strings.Contains(high.Text, "Questie") || high.Skill != 120 {
		t.Fatalf("%+v", high)
	}
}

func TestForLevelFishingAndNoSkill(t *testing.T) {
	g := New(index(), dataset(t))
	if r := g.ForLevel(char("Testisle", 1, 5), Fish); len(r.Zones) != 2 || r.Zones[0].Zone != "Testisle" {
		t.Fatalf("%+v", r)
	}
	c := char("", 1, 1)
	c.Professions = nil
	if r := g.ForLevel(c, Herb); len(r.Zones) != 0 || !strings.Contains(r.Text, "Herbalism") {
		t.Fatalf("%+v", r)
	}
}

func TestNoObjectSectionIsGraceful(t *testing.T) {
	for _, g := range []*Gatherer{nil, New(nil, nil), New(&questie.Index{NodesReason: "no object rows"}, dataset(t))} {
		if r := g.Find(char("", 1, 1), "copper", "mine"); len(r.Zones) != 0 || r.Text == "" {
			t.Fatalf("%+v", r)
		}
		if r := g.ForLevel(char("", 1, 1), Ore); len(r.Zones) != 0 || r.Text == "" {
			t.Fatalf("%+v", r)
		}
		if k, z := g.ItemZones(10, questie.Where{}); k != "" || z != nil {
			t.Fatal("zones from nothing")
		}
	}
}

func TestItemZonesFeedsThePlanner(t *testing.T) {
	g := New(index(), dataset(t))
	kind, zs := g.ItemZones(10, questie.Where{Zone: "Farland", X: 50, Y: 50})
	if kind != Ore || len(zs) != 2 || zs[0].Zone != "Farland" {
		t.Fatalf("%q %+v", kind, zs)
	}
	if got := Line(zs); !strings.HasPrefix(got, "Farland: Copper Seam (50, 50); Testisle") {
		t.Fatalf("line %q", got)
	}
}

func seenChar(zone string) character.Character {
	c := char(zone, 100, 100)
	m := 84
	c.NPCs = []character.NPC{{ID: 1, Name: "Greeter", MapID: &m, Zone: "Seenvale"}}
	c.Seen = &character.Observed{Gather: []character.SeenGather{
		{Kind: "mine", Map: 84, CX: 4, CY: 6, N: 5, SkillMin: 1, SkillMax: 60, Items: map[int64]int{10: 8}},
		{Kind: "fish", Map: 84, CX: 1, CY: 1, N: 2, SkillMin: 1, SkillMax: 60, Items: map[int64]int{30: 2}},
	}}
	return c
}

func seenDataset(t *testing.T) store.DataStore {
	type item struct {
		ID   int64  `parquet:"id"`
		Name string `parquet:"name"`
	}
	s1 := int16(1)
	return store.New(store.ReaderSource{
		"items":            table(t, []item{{10, "Copper Ore"}, {30, "Tinfish"}}),
		"recipes":          table(t, []recipeRow{{1, "Mining"}}),
		"recipe_reagents":  table(t, []reagentRow{{1, 10}}),
		"profession_notes": table(t, []noteRow{{"nodes", "Copper Seam", &s1}}),
	})
}

func TestSeenGatheringLeadsAndIsMarked(t *testing.T) {
	g := New(index(), seenDataset(t))
	res := g.Find(seenChar("Farland"), "Copper Ore", "mine")
	if len(res.Zones) == 0 || !res.Zones[0].Seen || res.Zones[0].Zone != "Seenvale" {
		t.Fatalf("%+v", res.Zones)
	}
	if !strings.Contains(res.Text, "Seenvale") || !strings.Contains(res.Text, "(seen by you)") || res.Zones[0].Coords[0] != [2]float64{22.5, 32.5} {
		t.Errorf("text %q coords %v", res.Text, res.Zones[0].Coords)
	}
	// With no Questie index the player's own sightings still answer, for a fish at their level too.
	bare := New(nil, seenDataset(t))
	if r := bare.Find(seenChar(""), "Tinfish", ""); len(r.Zones) != 1 || !strings.Contains(r.Text, "seen by you") || strings.Contains(r.Text, FromQuestie) {
		t.Errorf("bare %+v", r)
	}
	if r := bare.ForLevel(seenChar(""), Fish); len(r.Zones) != 1 || !r.Zones[0].Seen {
		t.Errorf("level %+v", r)
	}
	// Nothing seen: the Questie answer is unmarked.
	if r := g.Find(char("Farland", 100, 100), "Copper Ore", "mine"); strings.Contains(r.Text, "seen by you") {
		t.Errorf("unmarked: %s", r.Text)
	}
}

func TestSeenZonesFeedThePlannerFeeders(t *testing.T) {
	g := New(nil, seenDataset(t))
	kind, zs := g.ItemZonesFor(seenChar(""), 10, questie.Where{})
	if kind != Ore || len(zs) != 1 || !zs[0].Seen || !strings.Contains(Line(zs), "seen by you") {
		t.Errorf("%q %+v", kind, zs)
	}
}

type itemNameRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

type catchRow struct {
	ItemID int64   `parquet:"item_id"`
	Zone   string  `parquet:"zone_name"`
	Casts  int64   `parquet:"casts"`
	Rate   float64 `parquet:"rate"`
}

// A fish answered from the dataset's fishing reports names them as its source, never a Questie database the player may not have.
func TestCatchZonesNameTheFishingReportsNotQuestie(t *testing.T) {
	ds := store.New(store.ReaderSource{
		"items":           table(t, []itemNameRow{{7, "Raw Testfish"}}),
		"fishing_catches": table(t, []catchRow{{7, "Testisle", 40, 0.25}, {7, "Farland", 10, 0.5}}),
	})
	res := New(nil, ds).Find(character.Character{Faction: "Horde"}, "Raw Testfish", "fish")
	if want := "Fish for Raw Testfish: Farland: 50% of 10 casts; Testisle: 25% of 40 casts (" + FromCatches + ")."; res.Text != want {
		t.Errorf("text %q, want %q", res.Text, want)
	}
	if res.FromQuestie || strings.Contains(res.Text, "Questie") {
		t.Errorf("a Questie source was claimed: %+v", res)
	}
}
