package talents

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type buildRow struct {
	Class     string `parquet:"class"`
	Spec      string `parquet:"spec"`
	Band      string `parquet:"band"`
	Order     int16  `parquet:"order"`
	NodeID    int32  `parquet:"node_id"`
	Points    int16  `parquet:"points"`
	Rationale string `parquet:"rationale"`
	Source    string `parquet:"source"`
}

type nodeRow struct {
	NodeID int32  `parquet:"node_id"`
	Name   string `parquet:"name"`
}

func table[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func ip(n int) *int { return &n }

// synth is a Priest Shadow Magic build in two bands, a Priest Holy build in one and a Rogue
// Assassination build in one. Node ids: 1 Shadow Focus, 2 Improved Mind Blast, 3 Mind Flay,
// 4 Shadow Weaving, 5 Silence, 6 Renew, 7 Malice, 8 Lethality.
func synth(t *testing.T) store.DataStore {
	b := func(class, spec, band string, order int16, node int32, pts int16) buildRow {
		return buildRow{class, spec, band, order, node, pts, fmt.Sprintf("why %d", node), "curated"}
	}
	return store.New(store.ReaderSource{
		"talent_builds": table(t, []buildRow{
			b("Priest", "Shadow Magic", "10-19", 1, 1, 5), b("Priest", "Shadow Magic", "10-19", 2, 2, 5),
			b("Priest", "Shadow Magic", "20-29", 3, 3, 1), b("Priest", "Shadow Magic", "20-29", 4, 4, 3),
			b("Priest", "Shadow Magic", "20-29", 5, 1, 2), b("Priest", "Shadow Magic", "30-39", 6, 5, 1),
			b("Priest", "Holy", "10-19", 1, 6, 5),
			b("Rogue", "Assassination", "10-19", 1, 7, 3), b("Rogue", "Assassination", "10-19", 2, 8, 5),
			b("Rogue", "Assassination", "20-29", 3, 7, 2),
		}),
		"talent_nodes": table(t, []nodeRow{{1, "Shadow Focus"}, {2, "Improved Mind Blast"}, {3, "Mind Flay"}, {4, "Shadow Weaving"},
			{5, "Silence"}, {6, "Renew"}, {7, "Malice"}, {8, "Lethality"}}),
	})
}

func tl(node int, name string, rank int) character.Talent {
	return character.Talent{Name: name, Rank: rank, NodeID: &node}
}

func priest(level, unspent int, talents ...character.Talent) character.Character {
	return character.Character{Class: "PRIEST", Level: ip(level), Talents: &character.Talents{Unspent: ip(unspent), Trees: []character.TalentTree{{
		Groups:  []character.TalentGroup{{Name: "Discipline", Spent: 0}, {Name: "Shadow Magic", Spent: 0}},
		Talents: talents,
	}}}}
}

// withSpent sets the groups' spent points on a priest.
func withSpent(c character.Character, disc, shadow int) character.Character {
	g := c.Talents.Trees[0].Groups
	g[0].Spent, g[1].Spent = disc, shadow
	return c
}

func points(p Result) string {
	var out []string
	for _, pt := range p.Next {
		out = append(out, fmt.Sprintf("%s=%d", pt.Name, pt.Rank))
	}
	return strings.Join(out, ",")
}

func TestPlan(t *testing.T) {
	ds := synth(t)
	shadow24 := withSpent(priest(24, 1, tl(1, "Shadow Focus", 5), tl(2, "Improved Mind Blast", 5), tl(3, "Mind Flay", 1), tl(4, "Shadow Weaving", 2)), 0, 13)
	offBuild := withSpent(priest(24, 0, tl(1, "Shadow Focus", 5), tl(2, "Improved Mind Blast", 5), tl(6, "Renew", 2), tl(4, "Shadow Weaving", 5)), 2, 13)
	rogue12 := character.Character{Class: "ROGUE", Level: ip(12), Talents: &character.Talents{Trees: []character.TalentTree{{
		Groups: []character.TalentGroup{{Name: "Assassination", Spent: 2}}, Talents: []character.Talent{tl(7, "Malice", 2)}}}}}
	byName := priest(24, 1, character.Talent{Name: "Shadow Focus", Rank: 5})
	byName = withSpent(byName, 0, 5)

	cases := []struct {
		name    string
		c       character.Character
		opt     Options
		spec    string
		band    string
		next    string
		remain  int
		off     string
		missing string
	}{
		{"shadow priest at 24 keeps build order across bands", shadow24, Options{}, "Shadow Magic", "20-29",
			"Shadow Weaving=3,Shadow Focus=6,Shadow Focus=7", 3, "", ""},
		{"limit", shadow24, Options{Limit: 2}, "Shadow Magic", "20-29", "Shadow Weaving=3,Shadow Focus=6", 3, "", ""},
		{"capture without talents starts the build there", withSpent(priest(12, 5), 0, 3), Options{}, "Shadow Magic", "10-19",
			"Shadow Focus=1,Shadow Focus=2,Shadow Focus=3,Shadow Focus=4,Shadow Focus=5", 10, "", ""},
		{"rogue at 12", rogue12, Options{}, "Assassination", "10-19", "Malice=3,Lethality=1,Lethality=2,Lethality=3,Lethality=4", 6, "", ""},
		{"spec override", priest(15, 2), Options{Spec: "holy", Limit: 3}, "Holy", "10-19", "Renew=1,Renew=2,Renew=3", 5, "", ""},
		{"off-build points are listed", offBuild, Options{Limit: 1}, "Shadow Magic", "20-29", "Mind Flay=1", 3, "Renew 2/0,Shadow Weaving 5/3", ""},
		{"node matched by name when the capture has no node id", byName, Options{Limit: 1}, "Shadow Magic", "20-29", "Improved Mind Blast=1", 11, "", ""},
		{"no build for the level", priest(45, 0, tl(1, "Shadow Focus", 5)), Options{Spec: "Shadow Magic"}, "Shadow Magic", "", "", 0, "", "No curated Shadow Magic build for level 45."},
		{"no build for the spec", shadow24, Options{Spec: "Outlaw"}, "Outlaw", "", "", 0, "", "No curated build for Outlaw."},
		{"tied specs need an override", withSpent(priest(24, 1), 4, 4), Options{}, "", "", "", 0, "", "Discipline, Shadow Magic are tied at 4 points"},
		{"nothing spent", priest(24, 1), Options{}, "", "", "", 0, "", "No talent points captured"},
		{"level not captured", character.Character{Class: "PRIEST", Talents: shadow24.Talents}, Options{}, "Shadow Magic", "", "", 0, "", "Level not captured"},
	}
	for _, c := range cases {
		p := Plan(c.c, ds, c.opt)
		var off []string
		for _, o := range p.OffBuild {
			off = append(off, fmt.Sprintf("%s %d/%d", o.Name, o.Rank, o.Build))
		}
		got := points(p)
		if p.Spec != c.spec || p.Band != c.band || got != c.next || p.Remaining != c.remain || strings.Join(off, ",") != c.off || !strings.Contains(p.Missing, c.missing) {
			t.Errorf("%s: spec %q band %q next %q off %v missing %q", c.name, p.Spec, p.Band, got, off, p.Missing)
		}
		if c.missing != "" && len(p.Available) != 2 && c.c.Class == "PRIEST" {
			t.Errorf("%s: available %v", c.name, p.Available)
		}
	}
}

func TestNoBuilds(t *testing.T) {
	for name, ds := range map[string]store.DataStore{"nil store": nil, "no table": store.New(store.ReaderSource{})} {
		if p := Plan(priest(24, 1), ds, Options{}); p.Missing == "" || len(p.Next) != 0 {
			t.Errorf("%s: %+v", name, p)
		}
	}
	rogueOnly := Plan(character.Character{Class: "MAGE", Level: ip(20)}, synth(t), Options{Spec: "Fire"})
	if !strings.Contains(rogueOnly.Missing, "no curated talent builds for MAGE") {
		t.Errorf("mage: %q", rogueOnly.Missing)
	}
}

func TestRender(t *testing.T) {
	ds := synth(t)
	p := Plan(withSpent(priest(24, 1, tl(1, "Shadow Focus", 5), tl(2, "Improved Mind Blast", 5), tl(6, "Renew", 1)), 1, 10), ds, Options{Limit: 1})
	out := Render(p)
	for _, want := range []string{"PRIEST Shadow Magic, level band 20-29 (spec taken", "Unspent points: 1", "1. Mind Flay (rank 1) [20-29]: why 3", "more point(s)", "Renew: rank 1, build gives 0", "advisor defaults"} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}
	miss := Render(Plan(priest(45, 0, tl(1, "Shadow Focus", 5)), ds, Options{Spec: "Shadow Magic"}))
	if !strings.Contains(miss, "No curated Shadow Magic build for level 45.") || !strings.Contains(miss, "Builds available: Holy 10-19; Shadow Magic 10-19, 20-29, 30-39") {
		t.Errorf("missing render:\n%s", miss)
	}
}

func TestParseBand(t *testing.T) {
	for in, want := range map[string]string{"10-19": "10 19 true", "30-39": "30 39 true", "x": "0 0 false", "1-x": "1 0 false"} {
		lo, hi, ok := parseBand(in)
		if got := fmt.Sprint(lo, hi, ok); got != want {
			t.Errorf("%q: %s", in, got)
		}
	}
}
