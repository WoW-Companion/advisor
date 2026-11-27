package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

func TestCreatureFacts(t *testing.T) {
	ds := store.NewDir("../../testdata/drops_snapshot")
	for _, tc := range []struct{ q, want string }{
		{"where is Black Ooze?", "Black Ooze (creature 1032): level not held; zone not held; faction not held; rank (rare or elite) not held"},
		{"What does Razorfen Thornweaver drop?", "Silk Cloth (about 44% of 9 kills reported)"},
		{"what loot does Death's Head Geomancer give?", "Death's Head Geomancer (creature 7335)"},
		{"what level is the Jubling?", "Jubling (creature 14878): level 5; zone not held; faction template 1555"},
	} {
		if got := CreatureFacts(ds, tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "state none") {
			t.Errorf("CreatureFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"where is Hogger?", "where do I find Silk Cloth?", "which quest should I do next?"} {
		if got := CreatureFacts(ds, q); got != "" {
			t.Errorf("CreatureFacts(%q) = %q, want none", q, got)
		}
	}
	if got := CreatureFacts(nil, "where is Black Ooze?"); got != "" {
		t.Errorf("CreatureFacts(nil store) = %q, want none", got)
	}
}
