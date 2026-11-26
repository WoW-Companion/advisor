package route

import (
	"regexp"
	"testing"

	"github.com/WoW-Companion/advisor/internal/questie"
)

var bareMap = regexp.MustCompile(`\bmap \d+`)

func TestMapNameInstancesAndUnknown(t *testing.T) {
	inst := map[int]Instance{33: {Map: 33, Name: "Shadowfang Keep", Entrance: "Silverpine Forest"}, 43: {Map: 43, Name: "Wailing Caverns", Entrance: "The Barrens"}}
	withTable := &Places{
		Instances: inst,
		UIMaps: map[int]UIMap{
			310: {ID: 310, Name: "Shadowfang Keep", Instance: 33},
			500: {ID: 500, Name: "", Parent: 10},
			10:  {ID: 10, Name: "Elwynn Forest"},
		},
	}
	oldData := &Places{Instances: inst}
	cases := []struct {
		name   string
		p      *Places
		m      int
		want   string
		travel string
	}{
		{"ui_maps 310", withTable, 310, "Shadowfang Keep", "Shadowfang Keep (Silverpine Forest)"},
		{"built-in 310", oldData, 310, "Shadowfang Keep", "Shadowfang Keep (Silverpine Forest)"},
		{"built-in 279", oldData, 279, "Wailing Caverns", "Wailing Caverns (The Barrens)"},
		{"parent", withTable, 500, "Elwynn Forest (instance)", "Elwynn Forest (instance)"},
		{"unknown", withTable, 9999, "an unknown place", "another area"},
		{"no dataset", nil, 310, "an unknown place", "another area"},
	}
	for _, c := range cases {
		s := &questieSource{qx: &questie.Index{}, opt: Options{Places: c.p}}
		if got := s.mapName(c.m); got != c.want {
			t.Errorf("%s: mapName = %q, want %q", c.name, got, c.want)
		}
		if got := s.travelName(c.m, ""); got != c.travel {
			t.Errorf("%s: travelName = %q, want %q", c.name, got, c.travel)
		}
		if bareMap.MatchString(s.mapName(c.m)) || bareMap.MatchString(s.travelName(c.m, "")) {
			t.Errorf("%s: a bare map number leaked", c.name)
		}
	}
}

func TestPlanOutputsNeverSayMapN(t *testing.T) {
	p := BuildWith(townHero(5), townIndex(), townOpt())
	out := Envelope{Plans: []Plan{p}}.Lua() + labels(p.Steps) + labels(p.Later)
	if bareMap.MatchString(out) {
		t.Errorf("a bare map number in the plan output: %s", bareMap.FindString(out))
	}
}
