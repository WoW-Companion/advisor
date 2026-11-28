package questie

import "testing"

func TestEliteQuestReadsTheInstallsOwnFields(t *testing.T) {
	l := Layout{tables: map[string]map[string]int{"quest": {"name": 1, "questTagId": 40, "suggestedGroup": 41}}}
	tests := []struct {
		name string
		row  map[any]any
		want bool
	}{
		{"group tag", map[any]any{int64(40): int64(1)}, true},
		{"dungeon tag", map[any]any{int64(40): int64(81)}, true},
		{"pvp tag", map[any]any{int64(40): int64(41)}, false},
		{"group size", map[any]any{int64(41): int64(5)}, true},
		{"solo size", map[any]any{int64(41): int64(1)}, false},
		{"no field set", map[any]any{int64(1): "x"}, false},
	}
	for _, tt := range tests {
		if got := l.eliteQuest(tt.row); got != tt.want {
			t.Errorf("%s: elite %v, want %v", tt.name, got, tt.want)
		}
	}
	if (Layout{tables: map[string]map[string]int{"quest": {"name": 1}}}).eliteQuest(map[any]any{int64(1): int64(1)}) {
		t.Errorf("an install with no tag field has no elite answer")
	}
}
