package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

func TestSetFacts(t *testing.T) {
	ds := store.NewDir("../../testdata/items_snapshot")
	for _, tc := range []struct{ q, want string }{
		{"which pieces make up the Dreadmist Raiment set?", "Dreadmist Raiment (set 183): 8 pieces in the dataset"},
		{"what set does [Stormshroud Pants] belong to?", "Stormshroud Armor (set 142): 4 pieces in the dataset: Stormshroud Armor, Stormshroud Gloves, Stormshroud Pants, Stormshroud Shoulders"},
		{"which pieces are in The Elements set?", "The Elements (set 1678): 8 pieces"},
	} {
		if got := SetFacts(ds, tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "state none") {
			t.Errorf("SetFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"which pieces make up the Pale Oak Raiment set?", "what pieces does my set have?", "which quest should I do next, Hogger Helps?", "where is Dreadmist Raiment sold"} {
		if got := SetFacts(ds, q); got != "" {
			t.Errorf("SetFacts(%q) = %q, want none", q, got)
		}
	}
	if got := SetFacts(nil, "which pieces make up the Dreadmist Raiment set?"); got != "" {
		t.Errorf("SetFacts(nil store) = %q, want none", got)
	}
}
