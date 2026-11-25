package professions

import (
	"strings"
	"testing"
)

func TestEstimateFloor(t *testing.T) {
	cases := []struct {
		name          string
		learn, yellow int
		source        string
		want          int
	}{
		{"estimate far under yellow is lifted", 155, 185, "estimate", 175},
		{"runecloth belt", 195, 225, "estimate", 215},
		{"estimate near yellow is kept", 180, 185, "estimate", 180},
		{"first recipes of a rank are kept", 1, 25, "estimate", 1},
		{"trainer row is never moved", 105, 125, "trainer", 105},
		{"recipe item is never moved", 80, 105, "recipe_item", 80},
	}
	for _, c := range cases {
		r := Recipe{LearnSkill: &c.learn, YellowSkill: &c.yellow, LearnSource: c.source}
		if got := estimateFloor(r); got == nil || *got != c.want {
			t.Errorf("%s: got %v, want %d", c.name, got, c.want)
		}
	}
}

func TestExpertEarlyNamesTrainerByFaction(t *testing.T) {
	if got := expertEarly("Tailoring", "horde", 60); !contains(got, "Josef Gregorian in Undercity") || contains(got, "Bolero") {
		t.Errorf("horde: %q", got)
	}
	if got := expertEarly("Tailoring", "Alliance", 60); !contains(got, "Georgio Bolero in Stormwind") {
		t.Errorf("alliance: %q", got)
	}
	if got := expertEarly("Mining", "horde", 60); got != "" {
		t.Errorf("only Tailoring has the early Expert step: %q", got)
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
