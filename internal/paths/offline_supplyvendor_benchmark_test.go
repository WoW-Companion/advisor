package paths_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// TestOfflineBenchmarkPoisonVendorBootyBay: the real dataset holds no row for Sly Garrett (npc 2622), so
// "poison vendor in Booty Bay" finds him only through the shop a character opened. The dataset half runs when
// WOWC_DATASET names the dataset directory; the observed half (synthetic capture) always runs.
func TestOfflineBenchmarkPoisonVendorBootyBay(t *testing.T) {
	seen := []character.SeenVendor{{NPC: 2622, Name: "Sly Garrett", Map: 224, X: 40.5, Y: 73.8,
		Items: []character.SeenStock{{ItemID: 3371}, {ItemID: 8924}}}}
	qx := &questie.Index{Areas: map[int]questie.Area{1: {UIMap: 224, Zone: "Booty Bay"}}}
	q := vendors.Query{Text: "poison vendor in Booty Bay", Faction: "Horde"}

	if dir := os.Getenv("WOWC_DATASET"); dir != "" {
		if _, err := os.Stat(dir); err == nil {
			for _, f := range vendors.Search(store.NewDir(dir), qx, q).Found {
				if f.NPCID == 2622 {
					t.Errorf("the dataset is expected to hold no row for Sly Garrett, got %+v", f)
				}
			}
		}
	} else {
		t.Log("WOWC_DATASET is not set: the dataset-gap half is not checked")
	}

	// A synthetic dataset that, like the real one, knows the Poisons reagents but not the vendor.
	ds := store.New(store.ReaderSource{
		"items":           pqTable(t, []pqItem{{3371, "Empty Vial"}, {8924, "Dust of Decay"}}),
		"recipes":         pqTable(t, []pqRecipe{{1, "Poisons"}}),
		"recipe_reagents": pqTable(t, []pqReagent{{1, 3371}, {1, 8924}}),
		"npcs":            pqTable(t, []pqNpc{{1, "Other"}}),
		"vendor_sources":  pqTable(t, []pqOffer{{1, 3371}}),
	})
	if res := vendors.Search(ds, qx, q); len(res.Found) != 0 {
		t.Errorf("Sly Garrett found with no observation: %+v", res)
	}
	q.Observed = seen
	res := vendors.Search(ds, qx, q)
	t.Log(res.Text)
	if len(res.Found) == 0 || res.Found[0].NPCID != 2622 || !res.Found[0].Observed || res.Found[0].Zone != "Booty Bay" || !strings.Contains(res.Text, "Sly Garrett") {
		t.Errorf("want Sly Garrett from the observed shop, got %+v", res)
	}
}

type pqItem struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}
type pqRecipe struct {
	Spell int64  `parquet:"spell_id"`
	Line  string `parquet:"skill_line_name"`
}
type pqReagent struct {
	Spell int64 `parquet:"spell_id"`
	Item  int64 `parquet:"reagent_item_id"`
}
type pqNpc struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}
type pqOffer struct {
	Npc  int32 `parquet:"npc_id"`
	Item int32 `parquet:"item_id"`
}

func pqTable[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}
