package questie

import (
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The fixtures here are hand-written in QuestieDB's baked file shape with invented ids and names.
// Nothing is copied from Questie.

func enc(v any) []byte {
	head := func(major byte, n uint64) []byte {
		switch {
		case n < 24:
			return []byte{major<<5 | byte(n)}
		case n < 256:
			return []byte{major<<5 | 24, byte(n)}
		case n < 65536:
			return []byte{major<<5 | 25, byte(n >> 8), byte(n)}
		}
		return append([]byte{major<<5 | 26}, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	switch x := v.(type) {
	case nil:
		return []byte{0xf6}
	case int:
		if x < 0 {
			return head(1, uint64(-1-x))
		}
		return head(0, uint64(x))
	case int64:
		return head(0, uint64(x))
	case string:
		return append(head(2, uint64(len(x))), x...)
	case float64:
		return append([]byte{0xfb}, be8(math.Float64bits(x))...)
	case []any:
		b := head(4, uint64(len(x)))
		for _, e := range x {
			b = append(b, enc(e)...)
		}
		return b
	case map[int]any:
		keys := make([]int, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Ints(keys)
		b := head(5, uint64(len(x)))
		for _, k := range keys {
			b = append(b, enc(k)...)
			b = append(b, enc(x[k])...)
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

func toc(header string, rows []row) string {
	var b strings.Builder
	b.WriteString(header)
	for _, r := range rows {
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
		row{"Npc", 100, "7", map[int]any{902: []any{[]any{50.5, 60.25}}, 901: []any{[]any{10.0, 10.0}}}},
		row{"Npc", 103, "S", map[int]any{1: "Distant Hermit"}},
		row{"Npc", 103, "7", map[int]any{903: []any{[]any{20.0, 20.0}}}},
		row{"Npc", 200, "S", map[int]any{1: "Reef Crab"}},
		row{"Npc", 200, "7", map[int]any{902: []any{[]any{70.0, 30.0}}}})
	return r
}

// install writes a game folder with QuestieDB (and Questie) and returns its path.
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
	write(filepath.Join(add, "Questie", "Questie.toc"), "## Title: Questie\n## Version: 1.2.3\n")
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
	map1 := 7001
	at := Where{MapID: &map1, X: 50, Y: 60, Known: true}
	got := ix.Available(who(), at, nil)
	var ids []int
	for _, o := range got {
		ids = append(ids, o.Quest.ID)
	}
	// 2 is eligible; 9 follows the finished quest 5. Decoys: 1 in the log, 3 wrong class, 4 missing
	// prerequisite, 5 done, 6 placeholder name (skip func), 7 out of band, 8 other map, 10 skill.
	if fmt.Sprint(ids) != "[2 6 9]" { // 6 is dropped by the name check below, as in the advisor
		t.Fatalf("got %v", ids)
	}
	if got[0].Quest.ID != 2 || got[0].Giver.Name != "Elder Mira" || got[0].Giver.Sub != "Kelp Cove" {
		t.Fatalf("giver: %+v", got[0])
	}
	if s := got[0].Giver.String(); s != "Elder Mira, Kelp Cove (50.5, 60.2)" && s != "Elder Mira, Kelp Cove (50.5, 60.3)" {
		t.Fatalf("place %q", s)
	}
	skip := func(n string) bool { return strings.Contains(n, "DNT") }
	for _, o := range ix.Available(who(), at, skip) {
		if o.Quest.ID == 6 {
			t.Fatal("DNT quest offered")
		}
	}
	if ix.Available(who(), Where{}, nil) != nil {
		t.Fatal("no position, yet quests offered")
	}
}

func TestLocate(t *testing.T) {
	ix, _ := OpenGame(install(t, header, fixtureRows()))
	m := 7001
	at := Where{MapID: &m, X: 0, Y: 0, Known: true}
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
		{"readable", header, true, "Found Questie v1.2.3 with QuestieDB v9.9.9"},
		{"unknown contract", strings.Replace(header, "Contract-Version: 4", "Contract-Version: 77", 1), false, "unreadable (v9.9.9)"},
		{"source mode", strings.Replace(header, "baked", "source", 1), false, "unreadable (v9.9.9)"},
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

func TestGarbageRowsAreUnreadable(t *testing.T) {
	rows := []row{{"Quest", 1, "S", "not a map"}}
	game := install(t, header, rows)
	// every row decodes but no quest has a name
	if ix, st := OpenGame(game); ix != nil || st.Readable {
		t.Fatalf("garbage accepted: %+v", st)
	}
	toc := filepath.Join(game, "Interface", "AddOns", "QuestieDB", "QuestieDB_Forever.toc")
	os.WriteFile(toc, []byte(header+"## X-Quest-1-S: !!!not-base64\n## X-Quest-2-S: also bad\n"), 0o644)
	if ix, st := OpenGame(game); ix != nil || st.Readable {
		t.Fatalf("bad base64 accepted: %+v", st)
	}
}

func TestWarmQueryIsFast(t *testing.T) {
	game := install(t, header, fixtureRows())
	if ix, _ := OpenGame(game); ix == nil {
		t.Fatal("cold load failed")
	}
	memoMu.Lock()
	memo = map[string]*Index{} // force the warm path through the on-disk index
	memoMu.Unlock()
	start := time.Now()
	ix, _ := OpenGame(game)
	m := 7001
	ix.Available(who(), Where{MapID: &m, Known: true}, nil)
	if d := time.Since(start); d > 200*time.Millisecond {
		t.Fatalf("warm open+query took %v", d)
	}
	if files, _ := filepath.Glob(filepath.Join(game, "cache", "index-*.gob.gz")); len(files) != 1 {
		t.Fatalf("cache files %v", files)
	}
}

func TestCBOR(t *testing.T) {
	v, err := decodeCBOR(enc(map[int]any{1: "a", 2: []any{1.5, nil, -3}}))
	m, _ := v.(map[any]any)
	if err != nil || m[int64(1)] != "a" || m[int64(2)].([]any)[0] != 1.5 || m[int64(2)].([]any)[2] != int64(-3) {
		t.Fatalf("%v %v", v, err)
	}
	for _, bad := range [][]byte{{0x82, 0x01}, {0x1f}, {0x61}, {0x01, 0x02}} {
		if _, err := decodeCBOR(bad); err == nil {
			t.Fatalf("% x accepted", bad)
		}
	}
}

// Fixtures must never hold text copied from Questie. When the source of a Questie checkout is
// available (WOWC_QUESTIE_SOURCE, a folder), no testdata file may contain a run of more than 40
// characters that appears in it.
func TestTestdataHoldsNoQuestieText(t *testing.T) {
	src := os.Getenv("WOWC_QUESTIE_SOURCE")
	if src == "" {
		t.Skip("WOWC_QUESTIE_SOURCE not set: no Questie source to compare with")
	}
	var corpus strings.Builder
	filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(p, ".lua") || strings.HasSuffix(p, ".toc")) {
			b, _ := os.ReadFile(p)
			corpus.Write(b)
			corpus.WriteByte('\n')
		}
		return nil
	})
	if corpus.Len() == 0 {
		t.Skip("no Questie source under " + src)
	}
	text := corpus.String()
	filepath.WalkDir(filepath.Join("..", "..", "testdata"), func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, line := range strings.Split(string(b), "\n") {
			for i := 0; i+41 <= len(line); i += 4 {
				if strings.Contains(text, line[i:i+41]) {
					t.Errorf("%s holds text found in the Questie source: %q", p, line[i:i+41])
					return nil
				}
			}
		}
		return nil
	})
}
