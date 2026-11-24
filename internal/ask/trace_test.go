package ask

import (
	"reflect"
	"testing"
)

func TestEntitiesIn(t *testing.T) {
	for _, tc := range []struct {
		name, tool, text string
		want             []Entity
	}{
		{"quest row", "mcp__wowc-advisor__quest_lookup", `{"rows":[{"id":94003,"name":"The Skybreaker Bulwark","zone":"Zephras Isle"}],"text":"x"}`,
			[]Entity{{"quest", 94003, "The Skybreaker Bulwark"}, {"zone", 0, "Zephras Isle"}}},
		{"npc by name", "npc_lookup", `{"rows":[{"id":5,"name":"Seena"}]}`, []Entity{{"npc", 5, "Seena"}}},
		{"typed id key", "whats_next", `{"steps":[{"quest_id":7,"title":"Wolves"}]}`, []Entity{{"quest", 7, "Wolves"}}},
		{"unknown tool without id", "whats_next", `{"steps":[{"title":"Wolves"}]}`, nil},
		{"plain text", "quest_lookup", `no rows`, nil},
		{"at most three", "item_lookup", `[{"id":1,"name":"a"},{"id":2,"name":"b"},{"id":3,"name":"c"},{"id":4,"name":"d"}]`,
			[]Entity{{"item", 1, "a"}, {"item", 2, "b"}, {"item", 3, "c"}}},
	} {
		if got := EntitiesIn(tc.tool, tc.text); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestTraceMatchesResultToTool(t *testing.T) {
	var tr Trace
	tr.Use("a", "quest_lookup")
	tr.Result("a", `{"id":1,"name":"Q"}`)
	tr.Result("unknown", `{"id":2,"name":"X"}`)
	if got := tr.Entities(); len(got) != 1 || got[0].Kind != "quest" {
		t.Errorf("%+v", got)
	}
}
