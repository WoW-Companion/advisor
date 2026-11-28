package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

type catchRow struct {
	ZoneName string  `parquet:"zone_name"`
	ItemID   int64   `parquet:"item_id"`
	Casts    int64   `parquet:"casts"`
	Rate     float64 `parquet:"rate"`
	ZoneID   int64   `parquet:"zone_id"`
}

type itemNameRow struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

func fishStore(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"fishing_catches": table(t, []catchRow{
			{"The Barrens", 1, 100, 0.4, 17}, {"Stranglethorn Vale", 1, 50, 0.2, 33}, {"Elwynn Forest", 1, 40, 0.5, 12},
			{"The Barrens", 2, 100, 0.1, 17}, {"Stranglethorn Vale", 2, 50, 0.6, 33},
		}),
		"items": table(t, []itemNameRow{{1, "Raw Testfin Snapper"}, {2, "Oily Testmouth"}}),
	})
}

func TestFishingZoneNamedLoosely(t *testing.T) {
	ds := fishStore(t)
	for _, q := range []string{"where can i fish in barrens?", "fishing in the barrens", "how high fishing for stranglethorn?"} {
		if got := GatherFacts(ds, q); !strings.Contains(got, "Catches reported in") || strings.Contains(got, "cover only these zones") {
			t.Errorf("%q: the zone is not found:\n%s", q, got)
		}
	}
	if got := GatherFacts(ds, "fishing in stranglethorn"); !strings.Contains(got, "Catches reported in Stranglethorn Vale, best rate first: Oily Testmouth (60% of 50 casts), Raw Testfin Snapper (20% of 50 casts).") {
		t.Errorf("short zone name:\n%s", got)
	}
	if got := GatherFacts(ds, "fishing in vale"); !strings.Contains(got, "cover only these zones") {
		t.Errorf("a bare trailing word names no zone:\n%s", got)
	}
}

func TestFishingFishNamed(t *testing.T) {
	ds := fishStore(t)
	for _, q := range []string{"where do i catch testfin snapper?", "where can i fish for raw testfin snapper"} {
		got := GatherFacts(ds, q)
		want := "Raw Testfin Snapper is caught in these zones, best rate first: Elwynn Forest (Alliance home zone, 50% of 40 casts), The Barrens (40% of 100 casts), Stranglethorn Vale (20% of 50 casts)."
		if !strings.Contains(got, want) || !strings.Contains(got, "holds no fishing skill") || strings.Contains(got, "cover only these zones") {
			t.Errorf("%q:\n%s", q, got)
		}
	}
	if got := GatherFacts(ds, "where do i buy a snapper?"); got != "" {
		t.Errorf("a question with no fish or fishing word got facts:\n%s", got)
	}
}

func TestNodeQuestionPhrasings(t *testing.T) {
	ds := gatherStore(t)
	for _, q := range []string{"what skill do i need to mine gold?", "what can i mine at 100 mining?", "where do i farm tin?", "where to find tin"} {
		if got := GatherFacts(ds, q); !strings.Contains(got, "Tin Vein: needs Mining 65") {
			t.Errorf("%q: no node facts:\n%s", q, got)
		}
	}
	for _, q := range []string{"where is mine?", "what is mine?", "the mine is mine"} {
		if got := GatherFacts(ds, q); got != "" {
			t.Errorf("%q: node facts for a question that is not about a node:\n%s", q, got)
		}
	}
}

func TestNodeSkillStated(t *testing.T) {
	ds := gatherStore(t)
	for q, want := range map[string]string{
		"what can i mine at mining 100?":      "At Mining 100: can gather Tin Vein.",
		"what can i mine at 60 mining?":       "At Mining 60: can gather none of the listed nodes; not yet Tin Vein (needs 65).",
		"which herbs can i pick at skill 20?": "At Herbalism 20: can gather Peacebloom.",
		"mining skill of 65, can i mine tin?": "At Mining 65: can gather Tin Vein.",
	} {
		if got := GatherFacts(ds, q); !strings.Contains(got, want) {
			t.Errorf("%q: want %q in:\n%s", q, want, got)
		}
	}
	// no skill stated: no comparison; a level is not a skill
	for _, q := range []string{"where is tin ore?", "what mining skill for a level 40 character?"} {
		if got := GatherFacts(ds, q); strings.Contains(got, "At Mining") {
			t.Errorf("%q: a skill was read from the question:\n%s", q, got)
		}
	}
}
