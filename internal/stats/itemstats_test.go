package stats

import (
	"fmt"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
)

func TestItemStats(t *testing.T) {
	for _, c := range []struct {
		name string
		row  store.Row
		want string
	}{
		{"types and amounts", store.Row{"stat_types": []any{int64(5), int64(45)}, "stats": []any{int64(5), int64(45), int64(13), int64(9)}}, "map[5:13 45:9]"},
		{"no stats column", store.Row{"stat_types": []any{int64(5)}}, "map[]"},
		{"amounts for other types", store.Row{"stat_types": []any{int64(5)}, "stats": []any{int64(7), int64(4)}}, "map[]"},
		{"list of the wrong length", store.Row{"stat_types": []any{int64(5), int64(7)}, "stats": []any{int64(5), int64(7), int64(4)}}, "map[]"},
		{"an item with no stats", store.Row{"stat_types": []any{}, "stats": []any{}}, "map[]"},
	} {
		if got := fmt.Sprint(ItemStats(c.row)); got != c.want {
			t.Errorf("%s: %s, want %s", c.name, got, c.want)
		}
	}
}

func TestItemWeightsAddTheLeadStatsEquivalents(t *testing.T) {
	for _, c := range []struct {
		class, spec, role string
		stat              string
		want              float64
	}{
		{"MAGE", "Frost", RoleDamage, SpellPower, 1},
		{"PRIEST", "Holy", RoleHealer, SpellPower, 1},
		{"ROGUE", "Combat", RoleDamage, AttackPower, 0.8},
		{"WARRIOR", "Fury", RoleDamage, AttackPower, 0.8},
		{"WARRIOR", "Protection", RoleTank, AttackPower, 0},
		{"WARRIOR", "Protection", RoleTank, SpellPower, 0},
		{"MAGE", "Frost", RoleDamage, AttackPower, 0},
	} {
		p, ok := Lookup(c.class, c.spec, c.role)
		if !ok {
			t.Fatalf("%s %s: no profile", c.class, c.spec)
		}
		if got := (Prof{Profile: p, Weights: p.Weights}).ItemWeights()[c.stat]; got != c.want {
			t.Errorf("%s %s: %s weight %v, want %v", c.class, c.spec, c.stat, got, c.want)
		}
	}
}
