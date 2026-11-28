package vendors

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

type fItem struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}
type fNpc struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
	Zone string `parquet:"zone_name"`
}
type fOffer struct {
	Npc   int32 `parquet:"npc_id"`
	Item  int32 `parquet:"item_id"`
	Price int64 `parquet:"price_copper"`
}

func findData(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"items":          ftable(t, []fItem{{10, "Deadly Poison"}, {11, "Crystal Vial"}, {12, "Linen Cloth"}}),
		"npcs":           ftable(t, []fNpc{{2622, "Sly Garrett", "Stranglethorn Vale"}, {5, "Cloth Seller", "Elwynn Forest"}}),
		"vendor_sources": ftable(t, []fOffer{{2622, 10, 100}, {2622, 11, 40}, {5, 12, 10}}),
	})
}

// Sly Garrett is in no quest, so the dataset and the vendor flag are what find him; his subname is Questie's.
func findIndex() *questie.Index {
	return &questie.Index{
		Areas: map[int]questie.Area{1: {UIMap: 224, Zone: "Stranglethorn Vale", Sub: "Booty Bay"}, 2: {UIMap: 37, Zone: "Elwynn Forest"}},
		Npcs: map[int]questie.Entity{
			2622: {Name: "Sly Garrett", Sub: "Shady Goods", Flags: 0x80, Spawns: []questie.Spawn{{Area: 1, X: 40.5, Y: 73.8}}},
			3:    {Name: "Trade Goods Seller", Sub: "Trade Supplies", Flags: 0x80, Spawns: []questie.Spawn{{Area: 1, X: 28, Y: 77}}, Faction: "H"},
		},
	}
}

func TestPoisonKeywordFindsNeutralShadyGoodsVendor(t *testing.T) {
	for _, side := range []string{"Alliance", "Horde"} {
		res := Search(findData(t), findIndex(), Query{Text: "is there a poison vendor", Zone: "Booty Bay", Faction: side})
		if len(res.Found) != 1 {
			t.Fatalf("%s: %+v", side, res)
		}
		f := res.Found[0]
		if f.Name != "Sly Garrett" || f.Sub != "Shady Goods" || f.Subzone != "Booty Bay" || !f.HasXY || len(f.Sells) == 0 || f.Sells[0] != "Deadly Poison" {
			t.Errorf("%s: %+v", side, f)
		}
		if !strings.Contains(res.Text, "<Shady Goods>") || !strings.Contains(res.Text, "both factions") {
			t.Errorf("text %q", res.Text)
		}
	}
}

func TestSubnameKeywordNeedsNoItem(t *testing.T) {
	res := Search(nil, findIndex(), Query{Text: "Shady Goods", Faction: "Alliance"})
	if len(res.Found) != 1 || res.Found[0].Name != "Sly Garrett" {
		t.Errorf("%+v", res)
	}
	// A Horde-only vendor is not offered to an Alliance player.
	if res := Search(nil, findIndex(), Query{Text: "Trade Supplies", Faction: "Alliance"}); len(res.Found) != 0 {
		t.Errorf("Horde vendor offered: %+v", res.Found)
	}
}

func TestEmptyLookupSaysNotInMyData(t *testing.T) {
	res := Search(findData(t), findIndex(), Query{Text: "poison", Zone: "Elwynn Forest"})
	if len(res.Found) != 0 || !strings.HasPrefix(res.Text, "Not in my data") || res.Gap == "" {
		t.Errorf("%+v", res)
	}
}

type fRecipe struct {
	Spell int64  `parquet:"spell_id"`
	Line  string `parquet:"skill_line_name"`
}
type fReagent struct {
	Spell int64 `parquet:"spell_id"`
	Item  int64 `parquet:"reagent_item_id"`
}

// supplyData: Sly Garrett sells Poisons reagents but no item named poison; a trade vendor sells Tailoring reagents.
func supplyData(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"items":           ftable(t, []fItem{{20, "Empty Vial"}, {21, "Dust of Decay"}, {22, "Rough Arrow"}, {23, "Coarse Thread"}}),
		"npcs":            ftable(t, []fNpc{{2622, "Sly Garrett", "Stranglethorn Vale"}, {8, "Cloth Trader", "Orgrimmar"}, {9, "Arrow Seller", "Orgrimmar"}}),
		"vendor_sources":  ftable(t, []fOffer{{2622, 20, 5}, {2622, 21, 9}, {8, 23, 3}, {9, 22, 1}}),
		"recipes":         ftable(t, []fRecipe{{1, "Poisons"}, {2, "Tailoring"}}),
		"recipe_reagents": ftable(t, []fReagent{{1, 20}, {1, 21}, {2, 23}}),
	})
}

func TestProfessionWordFindsReagentVendor(t *testing.T) {
	for _, q := range []Query{{Text: "poison", Zone: "Booty Bay"}, {Text: "poison vendor in Booty Bay"}, {Text: "poison vendor Booty Bay"}} {
		q.Faction = "Horde"
		res := Search(supplyData(t), findIndex(), q)
		if len(res.Found) != 1 || res.Found[0].NPCID != 2622 || res.Found[0].Sub != "Shady Goods" {
			t.Fatalf("%+v: %+v", q, res)
		}
		if got := strings.Join(res.Found[0].Sells, ","); got != "Dust of Decay,Empty Vial" {
			t.Errorf("sells %q", got)
		}
	}
}

func TestSupplyWordsNameTradeVendor(t *testing.T) {
	res := Search(supplyData(t), nil, Query{Text: "tailoring supplies", Zone: "Orgrimmar", Faction: "Horde"})
	if len(res.Found) != 1 || res.Found[0].Name != "Cloth Trader" || len(res.Found[0].Sells) == 0 || res.Found[0].Sells[0] != "Coarse Thread" {
		t.Errorf("%+v", res)
	}
}

func TestProfessionWordMissStaysHonest(t *testing.T) {
	res := Search(supplyData(t), findIndex(), Query{Text: "alchemy vendor", Zone: "Booty Bay"})
	if len(res.Found) != 0 || !strings.HasPrefix(res.Text, "Not in my data") {
		t.Errorf("%+v", res)
	}
}

func ftable[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}
