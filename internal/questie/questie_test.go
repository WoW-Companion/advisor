package questie

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Fixtures are hand-written in QuestieDB's baked shape, with invented ids and names.

// enc writes v as CBOR with fixed-width integers, which is valid if not canonical.
func enc(v any) []byte {
	head := func(major byte, n int) []byte { return binary.BigEndian.AppendUint32([]byte{major<<5 | 26}, uint32(n)) }
	switch x := v.(type) {
	case nil:
		return []byte{0xf6}
	case int:
		if x < 0 {
			return head(1, -1-x)
		}
		return head(0, x)
	case string:
		return append(head(2, len(x)), x...)
	case float64:
		return binary.BigEndian.AppendUint64([]byte{0xfb}, math.Float64bits(x))
	case []any:
		b := head(4, len(x))
		for _, e := range x {
			b = append(b, enc(e)...)
		}
		return b
	case map[int]any:
		b := head(5, len(x))
		for k := range x { // one map per call site has few keys; any order is valid CBOR
			b = append(append(b, enc(k)...), enc(x[k])...)
		}
		return b
	}
	panic(fmt.Sprintf("enc %T", v))
}

type row struct {
	typ   string
	id    int
	field string
	val   any
}

// chunked is a value stored as a "~2~" head and two numbered pieces, as long values are.
type chunked struct{ v any }

func toc(header string, rows []row) string {
	var b strings.Builder
	b.WriteString(header)
	for _, r := range rows {
		if c, ok := r.val.(chunked); ok {
			e := base64.StdEncoding.EncodeToString(enc(c.v))
			fmt.Fprintf(&b, "## X-%s-%d-%s: ~2~\n## X-%s-%d-%s-1: %s\n## X-%s-%d-%s-2: %s\n", r.typ, r.id, r.field, r.typ, r.id, r.field, e[:8], r.typ, r.id, r.field, e[8:])
			continue
		}
		fmt.Fprintf(&b, "## X-%s-%d-%s: %s\n", r.typ, r.id, r.field, base64.StdEncoding.EncodeToString(enc(r.val)))
	}
	b.WriteString("\nsrc\\config.lua\n")
	return b.String()
}

const header = "## Interface: 16001\n## Title: QuestieDB\n## Version: 9.9.9\n## X-Contract-Version: 4\n## X-Flavor: Forever\n## X-Mode: baked\n"

const areas = `local ZoneDB = QuestieLoader:ImportModule("ZoneDB")
ZoneDB.private.areaIdToUiMapIdOverride = [[return {
    [5] = 0, -- an alias
}]]
ZoneDB.private.areaIdToUiMapId = [[return {
    [901] = 7001, -- Testisle
    [902] = 7001, -- Kelp Cove -> Testisle
    [903] = 7002, -- Farland
}]]
`

func fixtureRows() []row {
	q := func(id int, name string, lvl int, extra map[int]any, start, finish int) []row {
		s := map[int]any{1: name, 4: lvl - 1, 5: lvl, 17: 901}
		for k, v := range extra {
			s[k] = v
		}
		return []row{{"Quest", id, "S", s}, {"Quest", id, "2", []any{[]any{start}}}, {"Quest", id, "3", []any{[]any{finish}}}}
	}
	var r []row
	r = append(r, q(1, "Logged Task", 9, nil, 100, 100)...)
	r = append(r, q(2, "Gather Kelp", 9, nil, 100, 100)...)
	r = append(r, q(3, "Priest Rite", 9, map[int]any{7: 16}, 100, 100)...) // priests only
	r = append(r, q(4, "Second Step", 9, nil, 100, 100)...)
	r = append(r, row{"Quest", 4, "13", []any{99}}) // needs quest 99, not done
	r = append(r, q(5, "Done Already", 9, nil, 100, 100)...)
	r = append(r, q(6, "[DNT] Old Quest", 9, nil, 100, 100)...)
	r = append(r, q(7, "Too Hard", 15, nil, 100, 100)...)
	r = append(r, q(8, "Far Away", 9, nil, 103, 103)...)
	r = append(r, q(9, "Next Step", 10, nil, 100, 100)...)
	r = append(r, row{"Quest", 9, "13", []any{5}}) // needs quest 5, done
	r = append(r, q(10, "Skill Needed", 9, map[int]any{18: []any{171, 50}}, 100, 100)...)
	r = append(r, row{"Quest", 1, "10", []any{[]any{[]any{200, nil, 0}}}}) // a kill objective
	r = append(r,
		row{"Npc", 100, "S", map[int]any{1: "Elder Mira", 14: "Elder"}},
		row{"Npc", 100, "7", chunked{map[int]any{902: []any{[]any{50.5, 60.5}}, 901: []any{[]any{10.0, 10.0}}}}},
		row{"Npc", 103, "S", map[int]any{1: "Distant Hermit"}},
		row{"Npc", 103, "7", map[int]any{903: []any{[]any{20.0, 20.0}}}},
		row{"Npc", 200, "S", map[int]any{1: "Reef Crab"}},
		row{"Npc", 200, "7", map[int]any{902: []any{[]any{70.0, 30.0}}}})
	return r
}

func install(t *testing.T, hdr string, rows []row) string {
	t.Helper()
	game := t.TempDir()
	add := filepath.Join(game, "Interface", "AddOns")
	db := filepath.Join(add, "QuestieDB")
	zones := filepath.Join(db, "support", "Forever", "Zones")
	for _, d := range []string{zones, filepath.Join(add, "Questie")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(p, s string) {
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(db, "QuestieDB_Forever.toc"), toc(hdr, rows))
	write(filepath.Join(zones, "areaIdToUiMapId.lua"), areas)
	write(filepath.Join(add, "Questie", "Questie_Forever.toc"), "## Title: Questie\n## Version: 1.2.3\n")
	CacheDir = func() string { return filepath.Join(game, "cache") }
	return game
}

func who() Who {
	return Who{Level: 9, ClassID: 8, RaceID: 1, Below: 2, Above: 2, InLog: map[int]bool{1: true}, Done: map[int]bool{5: true}}
}

func TestAvailable(t *testing.T) {
	ix, st := OpenGame(install(t, header, fixtureRows()))
	if ix == nil {
		t.Fatalf("not readable: %+v", st)
	}
	m := 7001
	skip := func(n string) bool { return strings.Contains(n, "DNT") }
	got := ix.Available(who(), Where{MapID: &m, X: 50, Y: 60}, skip)
	var ids []int
	for _, o := range got {
		ids = append(ids, o.Quest.ID)
	}
	// 2 is eligible, and 9 follows the finished quest 5. Decoys: 1 in the log, 3 wrong class, 4 missing
	// prerequisite, 5 done, 6 placeholder name, 7 out of band, 8 other map, 10 needs a skill.
	if fmt.Sprint(ids) != "[2 9]" {
		t.Fatalf("got %v", ids)
	}
	if g := got[0].Giver; g.String() != "Elder Mira, Kelp Cove (50.5, 60.5)" {
		t.Fatalf("giver %q", g)
	}
	if ix.Available(who(), Where{}, skip) != nil {
		t.Fatal("no position, yet quests offered")
	}
}

func TestLocate(t *testing.T) {
	ix, _ := OpenGame(install(t, header, fixtureRows()))
	m := 7001
	at := Where{MapID: &m, X: 0, Y: 0}
	p, objective, ok := ix.Locate(1, false, at)
	if !ok || !objective || p.Name != "Reef Crab" {
		t.Fatalf("objective: %+v %v %v", p, objective, ok)
	}
	p, objective, ok = ix.Locate(1, true, at)
	if !ok || objective || p.Name != "Elder Mira" {
		t.Fatalf("turn-in: %+v %v %v", p, objective, ok)
	}
}

func TestProbe(t *testing.T) {
	cases := []struct {
		name   string
		hdr    string
		ok     bool
		detail string
	}{
		{"readable", header, true, "Found Questie v1.2.3 (QuestieDB v9.9.9)"},
		{"unknown contract", strings.Replace(header, "Contract-Version: 4", "Contract-Version: 77", 1), false, "unreadable (v9.9.9)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			game := install(t, c.hdr, fixtureRows())
			ok, text := Probe(game).Describe()
			if ok != c.ok || !strings.Contains(text, c.detail) {
				t.Fatalf("ok=%v %q", ok, text)
			}
			if ix, _ := OpenGame(game); (ix != nil) != c.ok {
				t.Fatalf("index loaded=%v", ix != nil)
			}
		})
	}
	ok, text := Probe(t.TempDir()).Describe()
	if ok || !strings.Contains(text, "Not installed") {
		t.Fatalf("absent: %v %q", ok, text)
	}
	if ix := ForSavedVariables(""); ix != nil {
		t.Fatal("index without a game folder")
	}
}

func TestWarmQueryIsFast(t *testing.T) {
	game := install(t, header, fixtureRows())
	if ix, _ := OpenGame(game); ix == nil {
		t.Fatal("cold load failed")
	}
	start := time.Now()
	ix, _ := OpenGame(game)
	m := 7001
	ix.Available(who(), Where{MapID: &m}, nil)
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Fatalf("warm open+query took %v", d)
	}
	if files, _ := filepath.Glob(filepath.Join(game, "cache", "index-*.gob")); len(files) != 1 {
		t.Fatalf("cache files %v", files)
	}
}

// Fixtures must never hold text copied from Questie. With a Questie checkout in WOWC_QUESTIE_SOURCE,
// no testdata file may contain a run of more than 40 characters found in its .lua or .toc files.
func TestTestdataHoldsNoQuestieText(t *testing.T) {
	src := os.Getenv("WOWC_QUESTIE_SOURCE")
	if src == "" {
		t.Skip("WOWC_QUESTIE_SOURCE not set: no Questie source to compare with")
	}
	var corpus strings.Builder
	walk := func(root string, visit func(p string, b []byte)) {
		filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if b, e := os.ReadFile(p); err == nil && !d.IsDir() && e == nil {
				visit(p, b)
			}
			return nil
		})
	}
	walk(src, func(p string, b []byte) {
		if strings.HasSuffix(p, ".lua") || strings.HasSuffix(p, ".toc") {
			corpus.Write(b)
		}
	})
	if corpus.Len() == 0 {
		t.Skip("no Questie source under " + src)
	}
	walk(filepath.Join("..", "..", "testdata"), func(p string, b []byte) {
		for _, line := range strings.Split(string(b), "\n") {
			for i := 0; i+41 <= len(line); i += 4 {
				if strings.Contains(corpus.String(), line[i:i+41]) {
					t.Errorf("%s holds text found in the Questie source: %q", p, line[i:i+41])
					return
				}
			}
		}
	})
}

func TestQuestieVersionFromFlavourTOCs(t *testing.T) {
	dir := t.TempDir()
	write := func(name, version string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("## Version: "+version+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("Questie_Vanilla.toc", "1.0.0")
	write("Questie_Forever.toc", "12.11.1")
	if h, ok := questieHeader(dir); !ok || h["Version"] != "12.11.1" {
		t.Fatalf("forever first: %v %v", h, ok)
	}
	if err := os.Remove(filepath.Join(dir, "Questie_Forever.toc")); err != nil {
		t.Fatal(err)
	}
	if h, ok := questieHeader(dir); !ok || h["Version"] != "1.0.0" {
		t.Fatalf("any Questie*.toc: %v %v", h, ok)
	}
	if _, ok := questieHeader(t.TempDir()); ok {
		t.Fatal("empty folder read")
	}
}

func TestParseItemAndCreditObjectives(t *testing.T) {
	rows := append(fixtureRows(),
		row{"Quest", 2, "10", []any{nil, nil, []any{[]any{300, "Kelp Bundle"}}, nil, []any{[]any{[]any{200}, 200, "Crab Kills"}}}},
		row{"Item", 300, "S", map[int]any{1: "Kelp Bundle"}},
		row{"Item", 300, "2", []any{200}})
	ix, st := OpenGame(install(t, header, rows))
	if ix == nil {
		t.Fatalf("not readable: %+v", st)
	}
	q := ix.Quests[2]
	if len(q.Items) != 1 || q.Items[0] != 300 || len(q.Credits) != 1 || q.Credits[0].Name != "Crab Kills" {
		t.Fatalf("objectives: %+v", q)
	}
	if it := ix.Items[300]; it.Name != "Kelp Bundle" || len(it.Npcs) != 1 || len(ix.Npcs[200].Spawns) == 0 {
		t.Fatalf("item %+v, npc %+v", it, ix.Npcs[200])
	}
}

func TestParseObjectiveText(t *testing.T) {
	rows := append(fixtureRows(), row{"Quest", 2, "8", []any{"Reclaim the keep", "from the raiders."}})
	ix, st := OpenGame(install(t, header, rows))
	if ix == nil {
		t.Fatalf("not readable: %+v", st)
	}
	if got := ix.Quests[2].Objective; got != "Reclaim the keep from the raiders." {
		t.Fatalf("objective %q", got)
	}
	if got := ix.Describe(ix.Quests[2].Starters); !strings.HasPrefix(got, "Elder Mira") {
		t.Fatalf("describe %q", got)
	}
}
