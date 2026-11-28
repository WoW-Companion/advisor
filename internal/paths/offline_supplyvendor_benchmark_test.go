package paths_test

import (
	"os"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// TestOfflineBenchmarkPoisonVendorBootyBay: "poison vendor in Booty Bay" names Sly Garrett with the Poisons reagents he
// sells, read from the real dataset's recipes, recipe_reagents and vendor_sources. It runs when WOWC_DATASET names the
// dataset directory and skips otherwise, because the snapshots under testdata hold no row for him.
func TestOfflineBenchmarkPoisonVendorBootyBay(t *testing.T) {
	dir := os.Getenv("WOWC_DATASET")
	if dir == "" {
		t.Skip("WOWC_DATASET is not set: the real dataset is not present")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skip("dataset directory not present")
	}
	res := vendors.Search(store.NewDir(dir), nil, vendors.Query{Text: "poison vendor in Booty Bay", Faction: "Horde"})
	t.Log(res.Text)
	if len(res.Found) == 0 || res.Found[0].NPCID != 2622 || len(res.Found[0].Sells) == 0 || !strings.Contains(res.Text, "Sly Garrett") {
		t.Errorf("want Sly Garrett with reagents, got %+v", res)
	}
}
