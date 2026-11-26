package talents

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/benchsummary"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline talent advice benchmark: the advisor's talent plan set beside the dataset's talent tables, with
// no network. testdata/advice_snapshot (scripts/advice-snapshot.sh) holds the published talent_builds,
// talent_nodes and talent_priorities rows. Two parts:
//
//   - every spec of the snapshot: the advisor says it has a checked build exactly when the dataset has verified
//     rows (a level and a rank on every row, a source that is not "curated (unverified)"). The published
//     dataset has none yet (the Icy Veins WoW Forever leveling builds could not be read), so each class is also
//     listed as a dataset-side difference.
//   - the next points of a checked build: the real nodes and order of four specs written as one row per point
//     in the dataset's per-point schema (an in-memory table, point i at level 9+i), and the points the advisor
//     lists for characters at different levels set beside the next rows of that table.
//
// Run it with make offline-benchmark; WOWC_BENCH_REPORT receives the result tables in Markdown.

const (
	snapshotDir  = "../../testdata/advice_snapshot"
	talentSource = "dataset talent_builds, talent_priorities"
	fixtureLabel = "icy-veins.com WoW Forever leveling guides (fixture rows in the per-point schema)"
)

// benchRow has the fields of benchsummary.Row; a local type keeps go vet quiet about unkeyed literals.
type benchRow benchsummary.Row

func finishBench(t *testing.T, area, title string, rows []benchRow) {
	t.Helper()
	results := make([]string, len(rows))
	for i, r := range rows {
		results[i] = r.Result
		if strings.HasPrefix(r.Result, "differ") && !strings.Contains(r.Result, "(data)") {
			t.Errorf("%s: advisor %q, reference %q: %s", r.Check, r.Advisor, r.Reference, r.Result)
		}
	}
	fmt.Println(benchsummary.AreaLine(area, results))
	table := make([]benchsummary.Row, len(rows))
	for i, r := range rows {
		table[i] = benchsummary.Row(r)
	}
	if err := benchsummary.AppendTable(os.Getenv("WOWC_BENCH_REPORT"), title, table); err != nil {
		t.Fatal(err)
	}
}

// mixSource serves the tables held in memory first and the snapshot's for the rest.
type mixSource struct {
	mem store.ReaderSource
	dir store.DirSource
}

func (m mixSource) Open(table string) (io.ReaderAt, int64, error) {
	if _, ok := m.mem[table]; ok {
		return m.mem.Open(table)
	}
	return m.dir.Open(table)
}

func snapshot() store.DataStore { return store.NewDir(snapshotDir) }

// pointRow is a talent_builds row of the per-point schema (DATA-045), one rank of one talent.
type pointRow struct {
	Class     string `parquet:"class"`
	Spec      string `parquet:"spec"`
	Order     int16  `parquet:"order"`
	NodeID    int32  `parquet:"node_id"`
	Level     int16  `parquet:"level"`
	Name      string `parquet:"talent_name"`
	RankAfter int16  `parquet:"rank_after"`
	Source    string `parquet:"source"`
}

// point is one build point read for the reference: the talent, the rank it reaches and the level it is spent at.
type point struct {
	node        int64
	name        string
	rank, level int
}

func (p point) String() string { return fmt.Sprintf("%s %d (level %d)", p.name, p.rank, p.level) }

// nodeNames reads talent_nodes: node id to name.
func nodeNames(t *testing.T, ds store.DataStore) map[int64]string {
	t.Helper()
	rows, err := ds.Filter("talent_nodes", store.Range{Column: "node_id", Min: 1})
	if err != nil {
		t.Fatal(err)
	}
	out := map[int64]string{}
	for _, r := range rows {
		out[num(r["node_id"])] = text(r["name"])
	}
	return out
}

// bandRows are a spec's rows of the snapshot's banded builds, in order.
func bandRows(t *testing.T, ds store.DataStore, class, spec string) []store.Row {
	t.Helper()
	rows, err := ds.Filter("talent_builds", store.Range{Column: "order", Min: 1})
	if err != nil {
		t.Fatal(err)
	}
	var out []store.Row
	for _, r := range rows {
		if text(r["class"]) == class && text(r["spec"]) == spec {
			out = append(out, r)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return num(out[i]["order"]) < num(out[j]["order"]) })
	return out
}

// perPoint expands a spec's banded rows into one point per rank: point i is spent at level 9+i, and a node's
// rank is the count of its points so far.
func perPoint(t *testing.T, ds store.DataStore, class, spec string) []point {
	t.Helper()
	names := nodeNames(t, ds)
	var out []point
	ranks := map[int64]int{}
	for _, r := range bandRows(t, ds, class, spec) {
		for k := int64(0); k < num(r["points"]); k++ {
			id := num(r["node_id"])
			ranks[id]++
			out = append(out, point{id, names[id], ranks[id], 9 + len(out) + 1})
		}
	}
	return out
}

// checkedStore is the snapshot with talent_builds replaced by the per-point rows of the given specs.
func checkedStore(t *testing.T, specs [][2]string) (store.DataStore, map[string][]point) {
	t.Helper()
	snap := snapshot()
	var rows []pointRow
	built := map[string][]point{}
	for _, sp := range specs {
		pts := perPoint(t, snap, sp[0], sp[1])
		built[sp[1]] = pts
		for i, p := range pts {
			rows = append(rows, pointRow{sp[0], sp[1], int16(i + 1), int32(p.node), int16(p.level), p.name, int16(p.rank), fixtureLabel})
		}
	}
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	mem := store.ReaderSource{"talent_builds": {ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}}
	return store.New(mixSource{mem, store.DirSource(snapshotDir)}), built
}

func classToken(class string) string { return strings.ToUpper(class) }

// TestOfflineBenchmarkTalentBuilds sets, for every spec of the snapshot, the advisor's "do I have a checked
// build" answer beside the dataset's rows, and every class beside the Forever build the dataset should hold.
func TestOfflineBenchmarkTalentBuilds(t *testing.T) {
	ds := snapshot()
	rows, err := ds.Filter("talent_builds", store.Range{Column: "order", Min: 1})
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ class, spec string }
	total, verified := map[key]int{}, map[key]int{}
	var order []key
	for _, r := range rows {
		k := key{text(r["class"]), text(r["spec"])}
		if total[k] == 0 {
			order = append(order, k)
		}
		total[k]++
		if _, ok := r["level"]; ok && r["level"] != nil && !strings.Contains(strings.ToLower(text(r["source"])), "unverified") {
			verified[k]++
		}
	}
	if len(order) < 12 {
		t.Fatalf("the snapshot has %d specs", len(order))
	}
	sort.Slice(order, func(i, j int) bool { return fmt.Sprint(order[i]) < fmt.Sprint(order[j]) })
	priorities, _ := ds.Filter("talent_priorities", store.Range{Column: "rank", Min: 1})
	var out []benchRow
	classes := map[string]bool{}
	lvl := 25
	for _, k := range order {
		c := character.Character{Class: classToken(k.class), Level: &lvl}
		p := Plan(c, ds, Options{Spec: k.spec})
		ref := fmt.Sprintf("%d of %d rows verified", verified[k], total[k])
		got := "a checked build"
		if p.NoBuild {
			got = p.Missing
		}
		res := "match"
		if p.NoBuild != (verified[k] == 0) || p.Spec != k.spec {
			res = "differ"
		}
		if !p.NoBuild && len(p.Next) == 0 && p.Remaining > 0 {
			res = "differ"
		}
		out = append(out, benchRow{k.class + " " + k.spec + ": checked build", got, ref, talentSource, res})
		classes[k.class] = true
	}
	var names []string
	for c := range classes {
		names = append(names, c)
	}
	sort.Strings(names)
	for _, class := range names {
		n := 0
		for _, r := range priorities {
			if text(r["class"]) == class {
				n++
			}
		}
		c := character.Character{Class: classToken(class), Level: &lvl}
		p := Plan(c, ds, Options{})
		res := "match"
		if n == 0 {
			res = "differ (data): the dataset holds no Icy Veins Forever leveling build or key talents for " + class + " yet; the advisor names no talent"
			if !p.NoBuild || len(p.Next) != 0 {
				res = "differ"
			}
		}
		out = append(out, benchRow{class + ": Forever leveling build in the dataset", p.Spec + ": " + p.Missing, fmt.Sprintf("%d key talents, 0 verified builds", n), "dataset talent_priorities, talent_builds", res})
	}
	finishBench(t, "talents-builds", "Offline talent builds: checked or not", out)
}

// takenOf is a character of the class who has taken the first n points of the build.
func takenOf(class, spec string, level int, pts []point, n int) character.Character {
	ranks := map[int64]int{}
	names := map[int64]string{}
	var ids []int64
	for _, p := range pts[:n] {
		if _, ok := ranks[p.node]; !ok {
			ids = append(ids, p.node)
		}
		ranks[p.node] = p.rank
		names[p.node] = p.name
	}
	var tl []character.Talent
	for _, id := range ids {
		node := int(id)
		tl = append(tl, character.Talent{Name: names[id], Rank: ranks[id], NodeID: &node})
	}
	unspent := max(0, level-9-n)
	return character.Character{Class: classToken(class), Level: &level, Talents: &character.Talents{Unspent: &unspent,
		Trees: []character.TalentTree{{Groups: []character.TalentGroup{{Name: spec, Spent: n}}, Talents: tl}}}}
}

// nextCases: a character at a level who has taken the first n points of a spec's build.
var nextCases = []struct {
	class, spec string
	level, n    int
}{
	{"Mage", "Frost", 10, 0}, {"Mage", "Frost", 20, 8}, {"Mage", "Frost", 25, 14}, {"Mage", "Frost", 39, 20},
	{"Warrior", "Arms", 12, 2}, {"Warrior", "Arms", 24, 9}, {"Warrior", "Arms", 30, 20}, {"Warrior", "Arms", 39, 30},
	{"Priest", "Shadow Magic", 15, 5}, {"Priest", "Shadow Magic", 22, 10}, {"Priest", "Shadow Magic", 28, 16}, {"Priest", "Shadow Magic", 39, 30},
	{"Rogue", "Combat", 10, 0}, {"Rogue", "Combat", 19, 8}, {"Rogue", "Combat", 26, 10}, {"Rogue", "Combat", 34, 24},
}

// TestOfflineBenchmarkNextPoints sets the points the advisor lists next beside the next rows of the build table:
// the first rows after the points taken, whose level the character has reached.
func TestOfflineBenchmarkNextPoints(t *testing.T) {
	ds, built := checkedStore(t, [][2]string{{"Mage", "Frost"}, {"Warrior", "Arms"}, {"Priest", "Shadow Magic"}, {"Rogue", "Combat"}})
	var out []benchRow
	for _, nc := range nextCases {
		pts := built[nc.spec]
		if nc.n > len(pts) {
			t.Fatalf("%s %s has %d points", nc.class, nc.spec, len(pts))
		}
		c := takenOf(nc.class, nc.spec, nc.level, pts, nc.n)
		p := Plan(c, ds, Options{Spec: nc.spec, Limit: 3})
		var want, got []string
		for _, pt := range pts[nc.n:] {
			if pt.level <= nc.level && len(want) < 3 {
				want = append(want, pt.String())
			}
		}
		for _, pt := range p.Next {
			got = append(got, fmt.Sprintf("%s %d (level %d)", pt.Name, pt.Rank, pt.Level))
		}
		res := "match"
		if strings.Join(got, "; ") != strings.Join(want, "; ") {
			res = "differ"
		}
		label := fmt.Sprintf("%s %s at level %d with %d points taken: next points", nc.class, nc.spec, nc.level, nc.n)
		show := func(l []string) string {
			if len(l) == 0 {
				return "none due"
			}
			return strings.Join(l, "; ")
		}
		out = append(out, benchRow{label, show(got), show(want), fixtureLabel, res})
	}
	finishBench(t, "talents-next", "Offline next talent points against the build table", out)
}

// TestOfflineBenchmarkOffBuild: a talent taken that the build does not give is noted, not judged, and does not
// hide the build's next point.
func TestOfflineBenchmarkOffBuild(t *testing.T) {
	ds, built := checkedStore(t, [][2]string{{"Mage", "Frost"}})
	pts := built["Frost"]
	snap := snapshot()
	var other string
	var otherID int64
	for id, n := range nodeNames(t, snap) {
		in := false
		for _, p := range pts {
			in = in || p.node == id
		}
		if !in && (otherID == 0 || id < otherID) {
			otherID, other = id, n
		}
	}
	c := takenOf("Mage", "Frost", 15, pts, 3)
	node := int(otherID)
	c.Talents.Trees[0].Talents = append(c.Talents.Trees[0].Talents, character.Talent{Name: other, Rank: 1, NodeID: &node})
	p := Plan(c, ds, Options{Spec: "Frost", Limit: 1})
	var out []benchRow
	res := "match"
	if len(p.OffBuild) != 1 || p.OffBuild[0].Name != other || p.OffBuild[0].Build != 0 {
		res = "differ"
	}
	out = append(out, benchRow{"Mage Frost with a talent outside the build: noted", fmt.Sprint(p.OffBuild), other + " is in no row of the build", fixtureLabel, res})
	res = "match"
	if len(p.Next) != 1 || p.Next[0].Name != pts[3].name || p.Next[0].Rank != pts[3].rank {
		res = "differ"
	}
	out = append(out, benchRow{"Mage Frost with a talent outside the build: next point", points(p), pts[3].String(), fixtureLabel, res})
	finishBench(t, "talents-offbuild", "Offline off-build talents", out)
}
