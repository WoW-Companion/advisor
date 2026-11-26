package learn

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

type recipeRow struct {
	SpellID     int32  `parquet:"spell_id"`
	Line        string `parquet:"skill_line_name"`
	Name        string `parquet:"name"`
	Creates     int32  `parquet:"creates_item_id"`
	LearnSkill  int16  `parquet:"learn_skill"`
	LearnedFrom string `parquet:"learned_from"`
}

type trainerRow struct {
	Npc       int32    `parquet:"npc_id"`
	Name      string   `parquet:"name"`
	Prof      string   `parquet:"profession"`
	ZoneID    int32    `parquet:"zone_id"`
	Zone      string   `parquet:"zone_name"`
	X         *float64 `parquet:"x,optional"`
	Y         *float64 `parquet:"y,optional"`
	Faction   string   `parquet:"faction"`
	Recipes   int32    `parquet:"recipes"`
	Reporters int32    `parquet:"reporters"`
}

type trainerSpell struct {
	Npc   int32 `parquet:"npc_id"`
	Spell int32 `parquet:"spell_id"`
	Skill int16 `parquet:"required_skill"`
}

type npcRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
	Zone string `parquet:"zone_name"`
}

type itemRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

type vendorRow struct {
	Npc   int32 `parquet:"npc_id"`
	Item  int32 `parquet:"item_id"`
	Price int64 `parquet:"price_copper"`
}

type recipeItem struct {
	Item  int32 `parquet:"item_id"`
	Spell int32 `parquet:"teaches_spell_id"`
	Skill int16 `parquet:"required_skill"`
}

func table[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func fp(v float64) *float64 { return &v }

func data(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"recipes": table(t, []recipeRow{
			{500, "Blacksmithing", "Runed Copper Gauntlets", 90, 1, "trainer"},
			{501, "Blacksmithing", "Bronze Hammer", 91, 90, "trainer"},
			{502, "Blacksmithing", "Plans: Thorium Bar", 92, 1, "item"},
			{700, "Leatherworking", "Embossed Leather Boots", 94, 1, "trainer"},
			{701, "Leatherworking", "Fine Leather Belt", 95, 150, "trainer"},
			{600, "Alchemy", "Minor Healing Potion", 93, 1, "trainer"},
		}),
		"trainers": table(t, []trainerRow{
			{1, "Alliance Smith", "Blacksmithing", 12, "Elwynn Forest", fp(40), fp(60), "Alliance", 20, 3},
			{2, "Horde Smith", "Blacksmithing", 14, "Durotar", fp(50), fp(50), "Horde", 20, 3},
			{3, "Far Alliance Smith", "Blacksmithing", 1537, "Ironforge", fp(50), fp(40), "Alliance", 30, 3},
			{4, "Questie Smith", "Blacksmithing", 0, "", nil, nil, "Horde", 5, 1},
		}),
		"trainer_spells": table(t, []trainerSpell{{1, 501, 90}}),
		"npcs":           table(t, []npcRow{{10, "Recipe Vendor", "Orgrimmar"}, {1, "Alliance Smith", ""}}),
		"items":          table(t, []itemRow{{90, "Runed Copper Gauntlets"}, {70, "Plans: Thorium Bar"}}),
		"vendor_sources": table(t, []vendorRow{{10, 70, 12500}}),
		"recipe_items":   table(t, []recipeItem{{70, 502, 0}}),
	})
}

func char(faction, zone string) character.Character {
	c := character.Character{Faction: faction, Professions: []character.Skill{{Name: "Blacksmithing"}}}
	if zone != "" {
		c.Location = &character.Location{Zone: zone}
	}
	return c
}

func TestTrainerChosenByFactionAndDistance(t *testing.T) {
	ds := data(t)
	for _, tc := range []struct{ faction, zone, want string }{
		{"Alliance", "Ironforge", "Far Alliance Smith in Ironforge (50, 40)"},
		{"Alliance", "Elwynn Forest", "Alliance Smith in Elwynn Forest (40, 60)"},
		{"Horde", "Elwynn Forest", "Horde Smith in Durotar (50, 50)"},
		{"Alliance", "Durotar", "Far Alliance Smith in Ironforge (50, 40)"}, // no same-zone trainer: an Alliance one, never the Horde one
	} {
		res, err := Find(char(tc.faction, tc.zone), ds, nil, "Runed Copper Gauntlets")
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(res.Text, "Learn it from "+tc.want) || strings.Contains(res.Text, "Horde Smith") && tc.faction == "Alliance" {
			t.Errorf("%s in %s: %s", tc.faction, tc.zone, res.Text)
		}
		if len(res.Ways) > MaxWays {
			t.Errorf("%d ways", len(res.Ways))
		}
	}
}

func TestQuestiePlacesATrainerTheDatasetCannot(t *testing.T) {
	ds := data(t)
	qx := &questie.Index{
		Areas: map[int]questie.Area{1637: {UIMap: 85, Zone: "Orgrimmar"}},
		Npcs:  map[int]questie.Entity{4: {Name: "Questie Smith", Faction: "H", Spawns: []questie.Spawn{{Area: 1637, X: 77.3, Y: 21.8}}}},
	}
	res, err := Find(char("Horde", "Orgrimmar"), ds, qx, "Runed Copper Gauntlets")
	if err != nil {
		t.Fatal(err)
	}
	want := "Learn it from Questie Smith in Orgrimmar (77, 22) (from your Questie database)."
	if !strings.Contains(res.Text, want) || !res.Ways[0].Questie {
		t.Errorf("want %q first:\n%s", want, res.Text)
	}
}

func TestVendorSoldRecipe(t *testing.T) {
	res, err := Find(char("Horde", "Orgrimmar"), data(t), nil, "Plans: Thorium Bar")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Ways) != 1 || res.Ways[0].Kind != Vendor || !strings.HasPrefix(res.Text, "Buy the recipe from Recipe Vendor in Orgrimmar for 1g 25s.") {
		t.Errorf("%+v\n%s", res.Ways, res.Text)
	}
	if res.Gate.Unknown || strings.Contains(res.Text, "trainer will show") {
		t.Errorf("an item recipe has no trainer gate: %s", res.Text)
	}
}

func TestClientMinimumOfOneStatesNoNumber(t *testing.T) {
	ds := data(t)
	res, _ := Find(char("Alliance", "Elwynn Forest"), ds, nil, "Runed Copper Gauntlets")
	if !res.Gate.Unknown || res.Gate.Skill != 0 || !strings.Contains(res.Text, "The trainer will show whether you can learn it yet.") {
		t.Errorf("gate %+v\n%s", res.Gate, res.Text)
	}
	if strings.Contains(res.Text, "skill 1") || strings.Contains(res.Text, "Blacksmithing 1") {
		t.Errorf("a number was stated: %s", res.Text)
	}
	real, _ := Find(char("Alliance", "Elwynn Forest"), ds, nil, "Bronze Hammer")
	if real.Gate.Skill != 90 || !strings.Contains(real.Text, "It needs Blacksmithing 90.") {
		t.Errorf("trainer value lost: %+v", real.Gate)
	}
}

func TestGateCounts(t *testing.T) {
	got := GateCounts(data(t))
	if got["Blacksmithing"] != 1 || got["Alchemy"] != 1 || got["Leatherworking"] != 1 || len(got) != 3 {
		t.Errorf("%v", got)
	}
}

func npc(name, sub, faction string, flags, area int, x, y float64) questie.Entity {
	return questie.Entity{Name: name, Sub: sub, Faction: faction, Flags: flags, Spawns: []questie.Spawn{{Area: area, X: x, Y: y}}}
}

func leatherIndex() *questie.Index {
	return &questie.Index{
		Areas: map[int]questie.Area{1: {UIMap: 85, Zone: "Orgrimmar"}, 2: {UIMap: 87, Zone: "Ironforge"}, 3: {UIMap: 88, Zone: "Thunder Bluff"}},
		Npcs: map[int]questie.Entity{
			1: npc("Orc Leatherer", "Leatherworking Trainer", "H", 0, 1, 60, 50),
			2: npc("Dwarf Leatherer", "Leatherworking Trainer", "A", 0, 2, 30, 30),
			3: npc("Far Tauren", "Leatherworking Trainer", "H", 0, 3, 40, 40),
			4: npc("Leather Goods", "Leatherworking Supplies", "H", 0, 1, 61, 51),
			5: npc("Journeyman Orc", "Journeyman Leatherworker", "H", 0, 1, 70, 70),
			6: npc("Expert Orc", "Expert Leatherworker", "H", 0, 1, 80, 80),
		},
	}
}

func TestQuestieTrainerWhenDatasetHasNone(t *testing.T) {
	qx := leatherIndex()
	res, err := Find(char("Horde", "Orgrimmar"), data(t), qx, "Embossed Leather Boots")
	if err != nil {
		t.Fatal(err)
	}
	want := "Learn it from Orc Leatherer, Leatherworking Trainer, in Orgrimmar (60, 50) (from your Questie database)."
	if !strings.HasPrefix(res.Text, want) {
		t.Errorf("want %q first:\n%s", want, res.Text)
	}
	if strings.Contains(res.Text, "Dwarf") || strings.Contains(res.Text, "Leather Goods") {
		t.Errorf("wrong faction or a vendor: %s", res.Text)
	}
	al, _ := Find(char("Alliance", "Ironforge"), data(t), qx, "Embossed Leather Boots")
	if !strings.HasPrefix(al.Text, "Learn it from Dwarf Leatherer, Leatherworking Trainer, in Ironforge (30, 30)") {
		t.Errorf("%s", al.Text)
	}
	// Away from the zone, the same continent beats the other one.
	far, _ := Find(char("Horde", "Durotar"), data(t), qx, "Embossed Leather Boots")
	if !strings.Contains(far.Text, "Orc Leatherer") {
		t.Errorf("%s", far.Text)
	}
}

func TestRankTierSuitsTheSkill(t *testing.T) {
	qx := leatherIndex()
	c := char("Horde", "Orgrimmar")
	c.Professions = []character.Skill{{Name: "Leatherworking", Rank: &[]int{100}[0]}}
	res, _ := Find(c, data(t), qx, "Embossed Leather Boots")
	if !strings.HasPrefix(res.Text, "Learn it from Journeyman Orc, Journeyman Leatherworker, in Orgrimmar") {
		t.Errorf("skill 100 should go to the Journeyman:\n%s", res.Text)
	}
	// A recipe gated at 150 still suits the Journeyman; the apprentice trainer does not reach it.
	res, _ = Find(c, data(t), qx, "Fine Leather Belt")
	if !strings.HasPrefix(res.Text, "Learn it from Journeyman Orc") {
		t.Errorf("gate 150:\n%s", res.Text)
	}
	c.Professions[0].Rank = &[]int{200}[0]
	res, _ = Find(c, data(t), qx, "Embossed Leather Boots")
	if !strings.HasPrefix(res.Text, "Learn it from Expert Orc") {
		t.Errorf("skill 200 should go to the Expert:\n%s", res.Text)
	}
}

func TestTrainerOfRules(t *testing.T) {
	for _, tc := range []struct {
		sub   string
		flags int
		prof  string
		tier  int
	}{
		{"Blacksmithing Trainer", 0, "Blacksmithing", 75},
		{"Junior Blacksmithing Trainer", 0, "Blacksmithing", 75},
		{"Journeyman Blacksmith", 0, "Blacksmithing", 150},
		{"Expert Blacksmith", 0, "Blacksmithing", 225},
		{"Artisan Blacksmith", 0, "Blacksmithing", 300},
		{"First Aid Trainer", 0, "First Aid", 75},
		{"Blacksmith", 0x10, "Blacksmithing", 75},
		{"Blacksmith", 0, "", 0},
		{"Blacksmithing Supplies", 0x10, "", 0},
		{"", 0x10, "", 0},
	} {
		p, tier, ok := questie.TrainerOf(questie.Entity{Sub: tc.sub, Flags: tc.flags})
		if p != tc.prof || tier != tc.tier || ok != (tc.prof != "") {
			t.Errorf("%q flags %d: %q %d %v", tc.sub, tc.flags, p, tier, ok)
		}
	}
}

func seenChar() character.Character {
	c := char("Alliance", "Ironforge")
	c.Seen = &character.Observed{
		Trainers: []character.SeenTrainer{
			{NPC: 77, Name: "Seen Smith", Map: 84, X: 30.5, Y: 70, Faction: "Alliance", T: 9, Offers: []character.SeenOffer{
				{Name: "Runed Copper Gauntlets", Cost: 1250, Skill: "Blacksmithing", SkillLevel: 75, Spell: 500}}},
			{NPC: 78, Name: "Horde Seen Smith", Faction: "Horde", T: 10, Offers: []character.SeenOffer{{Name: "Runed Copper Gauntlets", Spell: 500}}},
		},
		Vendors: []character.SeenVendor{{NPC: 79, Name: "Seen Seller", X: 5, Y: 6, T: 3, Items: []character.SeenStock{{ItemID: 70, Price: 30000}}}},
	}
	return c
}

func TestSeenTrainerLeadsWithCostAndSkillGate(t *testing.T) {
	res, err := Find(seenChar(), data(t), nil, "Runed Copper Gauntlets")
	if err != nil {
		t.Fatal(err)
	}
	w := res.Ways[0]
	if !w.Seen || w.Name != "Seen Smith" || w.Price == "" {
		t.Fatalf("first way %+v", w)
	}
	for _, want := range []string{"Learn it from Seen Smith", "(30, 70)", "for " + w.Price, "(seen by you)"} {
		if !strings.Contains(res.Text, want) {
			t.Errorf("text lacks %q: %s", want, res.Text)
		}
	}
	if res.Gate.Skill != 75 || strings.Contains(res.Text, "Horde Seen Smith") {
		t.Errorf("gate %+v, text %s", res.Gate, res.Text)
	}
	// Without the sighting the dataset's own trainer answers, unmarked.
	plain, _ := Find(char("Alliance", "Ironforge"), data(t), nil, "Runed Copper Gauntlets")
	if strings.Contains(plain.Text, "seen by you") || plain.Ways[0].Seen {
		t.Errorf("unmarked answer: %s", plain.Text)
	}
}

func TestSeenVendorLeadsWithItsPrice(t *testing.T) {
	res, err := Find(seenChar(), data(t), nil, "Plans: Thorium Bar")
	if err != nil {
		t.Fatal(err)
	}
	if w := res.Ways[0]; !w.Seen || w.Kind != Vendor || w.Name != "Seen Seller" || !strings.Contains(res.Text, "(seen by you)") || w.Price == "" {
		t.Errorf("%+v / %s", w, res.Text)
	}
}

// A recipe a recipe item teaches, with no vendor or drop the dataset can name, is not sent to a trainer.
func TestItemRecipeWithNoNamedPlace(t *testing.T) {
	ds := store.New(store.ReaderSource{
		"recipes":      table(t, []recipeRow{{502, "Blacksmithing", "Synthetic Plates", 92, 1, "item"}, {501, "Blacksmithing", "Synthetic Bar", 91, 90, "trainer"}}),
		"items":        table(t, []itemRow{{70, "Plans: Synthetic Plates"}}),
		"recipe_items": table(t, []recipeItem{{70, 502, 0}}),
		"trainers":     table(t, []trainerRow{}),
	})
	res, err := Find(char("Horde", ""), ds, nil, "Synthetic Plates")
	if err != nil {
		t.Fatal(err)
	}
	want := "It is learned from the recipe item Plans: Synthetic Plates, but the game data you have names no place that sells or drops it."
	if len(res.Ways) != 0 || res.Text != want {
		t.Errorf("text %q, %d ways", res.Text, len(res.Ways))
	}
	res, err = Find(char("Horde", ""), ds, nil, "Synthetic Bar")
	if err != nil || !strings.HasPrefix(res.Text, "Ask a Blacksmithing trainer") {
		t.Errorf("a trainer recipe still points to the trainers: %v %q", err, res.Text)
	}
}
