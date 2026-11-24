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
	Zone      string   `parquet:"zone_name"`
	Recipes   int32    `parquet:"recipes"`
	Faction   string   `parquet:"faction"`
	X         *float64 `parquet:"x,optional"`
	Y         *float64 `parquet:"y,optional"`
	Continent string   `parquet:"continent"`
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
			{600, "Alchemy", "Minor Healing Potion", 93, 1, "trainer"},
		}),
		"trainers": table(t, []trainerRow{
			{1, "Alliance Smith", "Blacksmithing", "Elwynn Forest", 20, "Alliance", fp(40), fp(60), "Eastern Kingdoms"},
			{2, "Horde Smith", "Blacksmithing", "Durotar", 20, "Horde", fp(50), fp(50), "Kalimdor"},
			{3, "Far Alliance Smith", "Blacksmithing", "Ironforge", 30, "Alliance", fp(50), fp(40), "Eastern Kingdoms"},
			{4, "Questie Smith", "Blacksmithing", "", 5, "Horde", nil, nil, ""},
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
	if got["Blacksmithing"] != 1 || got["Alchemy"] != 1 || len(got) != 2 {
		t.Errorf("%v", got)
	}
}
