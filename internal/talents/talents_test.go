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

// buildRow is a talent_builds row as DATA-045 writes it: one per point.
type buildRow struct {
	Class     string `parquet:"class"`
	Spec      string `parquet:"spec"`
	Level     int16  `parquet:"level"`
	Order     int16  `parquet:"order"`
	NodeID    int32  `parquet:"node_id"`
	Name      string `parquet:"talent_name"`
	RankAfter int16  `parquet:"rank_after"`
	Source    string `parquet:"source"`
}

// oldRow is a talent_builds row from before DATA-045: bands, no levels.
type oldRow struct {
	Class     string `parquet:"class"`
	Spec      string `parquet:"spec"`
	Band      string `parquet:"band"`
	Order     int16  `parquet:"order"`
	NodeID    int32  `parquet:"node_id"`
	Points    int16  `parquet:"points"`
	Rationale string `parquet:"rationale"`
	Source    string `parquet:"source"`
}

type priorityRow struct {
	Class  string `parquet:"class"`
	Spec   string `parquet:"spec"`
	Rank   int16  `parquet:"rank"`
	NodeID int32  `parquet:"node_id"`
	Name   string `parquet:"talent_name"`
	Reason string `parquet:"reason"`
}

// pts is one build row per rank of a node, from level lvl on, in order from order.
func pts(class, spec string, order, lvl int16, node int32, name string, ranks int16) []buildRow {
	var out []buildRow
	for r := int16(1); r <= ranks; r++ {
		out = append(out, buildRow{class, spec, lvl + r - 1, order + r - 1, node, name, r, "Icy Veins"})
	}
	return out
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

// synth is a Priest Shadow build (Spirit Tap 5 at levels 10-14, Mind Flay 15, Improved Shadow Word: Pain 16-17,
// Vampiric Embrace 18, Improved Mind Blast 19-23, Devouring Contagion 24, Shadowform 25), a shorter Priest Holy
// build, a Rogue Assassination build marked unverified, and priorities for Shadow. Node ids: 1 Spirit Tap,
// 2 Mind Flay, 3 Improved Shadow Word: Pain, 4 Vampiric Embrace, 5 Improved Mind Blast, 6 Devouring Contagion,
// 7 Shadowform, 8 Shadow Focus, 9 Renew, 10 Malice.
func synth(t *testing.T) store.DataStore {
	var b []buildRow
	add := func(rows []buildRow) { b = append(b, rows...) }
	add(pts("Priest", "Shadow", 1, 10, 1, "Spirit Tap", 5))
	add(pts("Priest", "Shadow", 6, 15, 2, "Mind Flay", 1))
	add(pts("Priest", "Shadow", 7, 16, 3, "Improved Shadow Word: Pain", 2))
	add(pts("Priest", "Shadow", 9, 18, 4, "Vampiric Embrace", 1))
	add(pts("Priest", "Shadow", 10, 19, 5, "Improved Mind Blast", 5))
	add(pts("Priest", "Shadow", 15, 24, 6, "Devouring Contagion", 1))
	add(pts("Priest", "Shadow", 16, 25, 7, "Shadowform", 1))
	add(pts("Priest", "Holy", 1, 10, 9, "Renew", 3))
	b = append(b, buildRow{"Rogue", "Assassination", 10, 1, 10, "Malice", 1, "curated (unverified)"})
	return store.New(store.ReaderSource{
		"talent_builds": table(t, b),
		"talent_priorities": table(t, []priorityRow{
			{"Priest", "Shadow", 1, 1, "Spirit Tap", "mana back on killing blows; the core leveling talent"},
			{"Priest", "Shadow", 2, 2, "Mind Flay", "your channelled filler"},
			{"Priest", "Shadow", 4, 6, "Devouring Contagion", "spreads your damage over time"},
		}),
		"talent_nodes": table(t, []nodeRow{{1, "Spirit Tap"}, {2, "Mind Flay"}, {3, "Improved Shadow Word: Pain"}, {4, "Vampiric Embrace"},
			{5, "Improved Mind Blast"}, {6, "Devouring Contagion"}, {7, "Shadowform"}, {8, "Shadow Focus"}, {9, "Renew"}, {10, "Malice"}}),
	})
}

// oldData is a dataset from before DATA-045: banded builds, no levels.
func oldData(t *testing.T) store.DataStore {
	return store.New(store.ReaderSource{
		"talent_builds": table(t, []oldRow{{"Priest", "Shadow Magic", "20-29", 1, 8, 5, "invented", "curated (advisor defaults)"}}),
		"talent_nodes":  table(t, []nodeRow{{8, "Shadow Focus"}}),
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

// level23 is the state Icy Veins' build reaches at level 23: every point through level 23.
func level23(level, unspent int) character.Character {
	return withSpent(priest(level, unspent, tl(1, "Spirit Tap", 5), tl(2, "Mind Flay", 1), tl(3, "Improved Shadow Word: Pain", 2),
		tl(4, "Vampiric Embrace", 1), tl(5, "Improved Mind Blast", 5)), 0, 14)
}

func TestPlan(t *testing.T) {
	ds := synth(t)
	offBuild := level23(24, 1)
	offBuild.Talents.Trees[0].Talents = append(offBuild.Talents.Trees[0].Talents, tl(8, "Shadow Focus", 2))
	byName := priest(24, 1, character.Talent{Name: "Spirit Tap", Rank: 5})
	byName = withSpent(byName, 0, 5)

	cases := []struct {
		name    string
		c       character.Character
		opt     Options
		spec    string
		next    string
		done    string
		remain  int
		off     string
		missing string
	}{
		{"shadow priest at 24 with the level 23 state gets the level 24 point", level23(24, 1), Options{}, "Shadow",
			"Devouring Contagion=1", "Improved Mind Blast 5/5", 1, "", ""},
		{"at 25 the next two", level23(25, 2), Options{}, "Shadow", "Devouring Contagion=1,Shadowform=1", "Improved Mind Blast 5/5", 2, "", ""},
		{"limit", level23(25, 2), Options{Limit: 1}, "Shadow", "Devouring Contagion=1", "Improved Mind Blast 5/5", 2, "", ""},
		{"points not yet reached by level are not listed", withSpent(priest(12, 3), 0, 0), Options{}, "Shadow",
			"Spirit Tap=1,Spirit Tap=2,Spirit Tap=3", "", 3, "", ""},
		{"a rank half taken", withSpent(priest(24, 1, tl(1, "Spirit Tap", 3)), 0, 3), Options{Limit: 2}, "Shadow", "Spirit Tap=4,Spirit Tap=5", "Spirit Tap 3/5", 12, "", ""},
		{"spec override", priest(12, 2), Options{Spec: "holy"}, "Holy", "Renew=1,Renew=2,Renew=3", "", 3, "", ""},
		{"2 points in Shadow Focus are off-build, advice continues", offBuild, Options{}, "Shadow", "Devouring Contagion=1", "Improved Mind Blast 5/5", 1, "Shadow Focus 2/0", ""},
		{"node matched by name when the capture has no node id", byName, Options{Limit: 1}, "Shadow", "Mind Flay=1", "Spirit Tap 5/5", 10, "", ""},
		{"tied specs go to the default", withSpent(priest(12, 1), 4, 4), Options{Limit: 1}, "Shadow", "Spirit Tap=1", "", 3, "", ""},
		{"nothing spent goes to the default", priest(12, 1), Options{Limit: 1}, "Shadow", "Spirit Tap=1", "", 3, "", ""},
		{"build finished through the level", level23(23, 0), Options{}, "Shadow", "", "Improved Mind Blast 5/5", 0, "", ""},
		{"no build for the spec", level23(24, 1), Options{Spec: "Outlaw"}, "Outlaw", "", "", 0, "", "I don't have a checked build for Outlaw yet."},
		{"level not captured", character.Character{Class: "PRIEST", Talents: level23(24, 1).Talents}, Options{}, "Shadow", "", "", 0, "", "Level not captured"},
	}
	for _, c := range cases {
		p := Plan(c.c, ds, c.opt)
		var off []string
		for _, o := range p.OffBuild {
			off = append(off, fmt.Sprintf("%s %d/%d", o.Name, o.Rank, o.Build))
		}
		got := points(p)
		if p.Spec != c.spec || got != c.next || p.Done != c.done || p.Remaining != c.remain || strings.Join(off, ",") != c.off || !strings.Contains(p.Missing, c.missing) {
			t.Errorf("%s: spec %q next %q done %q remaining %d off %v missing %q", c.name, p.Spec, got, p.Done, p.Remaining, off, p.Missing)
		}
		for _, pt := range p.Next {
			if pt.Name == "Shadow Focus" {
				t.Errorf("%s: suggests Shadow Focus", c.name)
			}
		}
	}
}

func TestDefaultSpecFlag(t *testing.T) {
	ds := synth(t)
	if p := Plan(priest(12, 1), ds, Options{}); !p.SpecDefault || p.SpecInferred || p.Spec != "Shadow" {
		t.Errorf("zero points: %+v", p)
	}
	if p := Plan(level23(24, 1), ds, Options{}); p.SpecDefault || !p.SpecInferred {
		t.Errorf("points: %+v", p)
	}
}

func TestOffBuildNoteOnce(t *testing.T) {
	c := level23(24, 1)
	c.Talents.Trees[0].Talents = append(c.Talents.Trees[0].Talents, tl(8, "Shadow Focus", 2))
	out := Render(Plan(c, synth(t), Options{}))
	want := "You have 2 points in Shadow Focus, which the Icy Veins leveling build skips"
	if strings.Count(out, want) != 1 || !strings.Contains(out, "never suggest a respec") {
		t.Errorf("render:\n%s", out)
	}
	if strings.Contains(Render(Plan(level23(24, 1), synth(t), Options{})), "skips") {
		t.Error("an off-build note without off-build points")
	}
}

func TestPriorityReasons(t *testing.T) {
	p := Plan(withSpent(priest(12, 1, tl(1, "Spirit Tap", 2)), 0, 2), synth(t), Options{Limit: 1})
	if len(p.Next) != 1 || p.Next[0].Rationale != "mana back on killing blows; the core leveling talent" || len(p.Priorities) != 3 {
		t.Fatalf("%+v", p)
	}
	out := Render(p)
	for _, want := range []string{"Spirit Tap 2/5 done · next: Spirit Tap 3/5", "1. Spirit Tap (rank 3 of 5): mana back", "key leveling talents", "2. Mind Flay: your channelled filler", "never from general knowledge"} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}
}

func TestHonestFallback(t *testing.T) {
	cases := map[string]struct {
		ds   store.DataStore
		c    character.Character
		spec string
	}{
		"old dataset":     {oldData(t), withSpent(priest(24, 2, tl(8, "Shadow Focus", 2)), 0, 2), "Shadow Magic"},
		"unverified spec": {synth(t), character.Character{Class: "ROGUE", Level: ip(12), Talents: &character.Talents{Unspent: ip(2)}}, "Assassination"},
	}
	for name, c := range cases {
		p := Plan(c.c, c.ds, Options{})
		out := Render(p)
		if !p.NoBuild || len(p.Next) != 0 || len(p.Options) != 0 || !strings.Contains(out, "I don't have a checked build for "+c.spec+" yet.") || !strings.Contains(out, "Unspent points: 2") {
			t.Errorf("%s: %+v\n%s", name, p, out)
		}
		if strings.Contains(out, "Shadow Focus") {
			t.Errorf("%s names a pick:\n%s", name, out)
		}
	}
}

func TestNoBuilds(t *testing.T) {
	for name, ds := range map[string]store.DataStore{"nil store": nil, "no table": store.New(store.ReaderSource{})} {
		if p := Plan(priest(24, 1), ds, Options{}); p.Missing == "" || len(p.Next) != 0 {
			t.Errorf("%s: %+v", name, p)
		}
	}
	mage := Plan(character.Character{Class: "MAGE", Level: ip(20)}, synth(t), Options{Spec: "Fire"})
	if !strings.Contains(mage.Missing, "no talent builds for MAGE") {
		t.Errorf("mage: %q", mage.Missing)
	}
}

// A character who has taken every point of the build is told the build ends at its last level, not that it has nothing more.
func TestPlanBuildEnds(t *testing.T) {
	ds := synth(t)
	lvl := 60
	var taken []character.Talent
	for _, tl := range []struct {
		id   int
		name string
		rank int
	}{{1, "Spirit Tap", 5}, {2, "Mind Flay", 1}, {3, "Improved Shadow Word: Pain", 2}, {4, "Vampiric Embrace", 1}, {5, "Improved Mind Blast", 5}, {6, "Devouring Contagion", 1}, {7, "Shadowform", 1}} {
		id := tl.id
		taken = append(taken, character.Talent{Name: tl.name, Rank: tl.rank, NodeID: &id})
	}
	c := character.Character{Class: "PRIEST", Level: &lvl, Talents: &character.Talents{Unspent: ip(20), Trees: []character.TalentTree{{Name: "Shadow", Talents: taken}}}}
	p := Plan(c, ds, Options{Spec: "Shadow"})
	if len(p.Next) != 0 || len(p.Notes) == 0 || !strings.Contains(p.Notes[0], "build ends at level 25") || strings.Contains(p.Notes[0], "nothing more") {
		t.Errorf("a finished build: next %v, notes %v", p.Next, p.Notes)
	}
	// Mid-build with nothing due, the build is not over.
	lvl = 12
	c.Talents.Trees[0].Talents = taken[:1]
	c.Talents.Trees[0].Talents[0].Rank = 3
	if p = Plan(c, ds, Options{Spec: "Shadow"}); len(p.Notes) == 0 || !strings.Contains(p.Notes[0], "nothing more to take at level 12") {
		t.Errorf("a build in progress: %v", p.Notes)
	}
}

// The tank role of a druid is Feral Combat; a class with no spec of the stored role is told so.
func TestPlanRoleWithoutSpec(t *testing.T) {
	var b []buildRow
	b = append(b, pts("Druid", "Balance", 1, 10, 20, "Starfire", 1)...)
	b = append(b, pts("Druid", "Feral Combat", 1, 10, 21, "Ferocity", 1)...)
	b = append(b, pts("Druid", "Restoration", 1, 10, 22, "Improved Mark", 1)...)
	b = append(b, pts("Hunter", "Beast Mastery", 1, 10, 23, "Endurance", 1)...)
	ds := store.New(store.ReaderSource{"talent_builds": table(t, b), "talent_nodes": table(t, []nodeRow{{20, "Starfire"}, {21, "Ferocity"}, {22, "Improved Mark"}, {23, "Endurance"}})})
	lvl := 20
	druid := character.Character{Class: "DRUID", Level: &lvl, Profile: &character.Profile{Role: "tank"}}
	if p := Plan(druid, ds, Options{}); p.Spec != "Feral Combat" || len(p.Notes) > 0 && strings.Contains(strings.Join(p.Notes, " "), "no tank") {
		t.Errorf("a tank druid: spec %q, notes %v", p.Spec, p.Notes)
	}
	hunter := character.Character{Class: "HUNTER", Level: &lvl, Profile: &character.Profile{Role: "healer"}}
	p := Plan(hunter, ds, Options{})
	if p.Spec != "Beast Mastery" || !strings.Contains(strings.Join(p.Notes, " "), "Hunter has no healer talent tree") {
		t.Errorf("a healer hunter: spec %q, notes %v", p.Spec, p.Notes)
	}
}

// The no-build answer carries the role note too.
func TestRenderNoBuildNote(t *testing.T) {
	r := Result{NoBuild: true, Missing: "I don't have a checked build for Arms yet.", Notes: []string{"Warrior has no healer talent tree; this is the Arms build."}}
	if got := Render(r); !strings.Contains(got, "Note: Warrior has no healer talent tree") {
		t.Errorf("Render = %q", got)
	}
}
