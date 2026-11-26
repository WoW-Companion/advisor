package hosted

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/server"
)

type item struct {
	ID    int64  `parquet:"id"`
	Name  string `parquet:"name"`
	Level int32  `parquet:"item_level"`
}
type quest struct {
	ID          int64  `parquet:"id"`
	QuestLineID *int32 `parquet:"quest_line_id,optional"`
}
type npc struct {
	ID       int64   `parquet:"id"`
	Name     string  `parquet:"name"`
	ZoneName *string `parquet:"zone_name,optional"`
}
type loot struct {
	ItemID int64   `parquet:"item_id"`
	Kind   string  `parquet:"source_kind"`
	ID     int64   `parquet:"source_id"`
	Kills  int32   `parquet:"kills"`
	Rate   float64 `parquet:"rate"`
}
type instance struct {
	MapID int32  `parquet:"map_id"`
	Name  string `parquet:"name"`
}
type boss struct {
	MapID      int32  `parquet:"map_id"`
	Name       string `parquet:"name"`
	Order      int32  `parquet:"order_index"`
	CreatureID *int32 `parquet:"creature_id,optional"`
}
type recipe struct {
	SpellID int64  `parquet:"spell_id"`
	Name    string `parquet:"name"`
	Skill   string `parquet:"skill_line_name"`
}
type reagent struct {
	SpellID int64 `parquet:"spell_id"`
	ItemID  int64 `parquet:"reagent_item_id"`
	Count   int32 `parquet:"count"`
}
type line struct {
	LineID  int32  `parquet:"line_id"`
	Name    string `parquet:"name"`
	QuestID int32  `parquet:"quest_id"`
}

func enc[T any](t testing.TB, rows []T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := parquet.Write(&b, rows); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

type nameRow struct {
	Norm  string `parquet:"norm"`
	Name  string `parquet:"name"`
	Table string `parquet:"table"`
	ID    int32  `parquet:"id"`
}

// nameIndex is names.parquet for the fixture's named rows, sorted by norm.
func nameIndex(t testing.TB, items []item, npcs []npc, recipes []recipe, insts []instance) []byte {
	var rows []nameRow
	for _, i := range items {
		rows = append(rows, nameRow{names.Normalize(i.Name), i.Name, names.Items, int32(i.ID)})
	}
	for _, n := range npcs {
		rows = append(rows, nameRow{names.Normalize(n.Name), n.Name, names.Npcs, int32(n.ID)})
	}
	for _, r := range recipes {
		rows = append(rows, nameRow{names.Normalize(r.Name), r.Name, names.Recipes, int32(r.SpellID)})
	}
	for _, i := range insts {
		rows = append(rows, nameRow{names.Normalize(i.Name), i.Name, names.Instances, int32(i.MapID)})
	}
	sort.Slice(rows, func(a, b int) bool { return rows[a].Norm < rows[b].Norm })
	var b bytes.Buffer
	if err := parquet.Write(&b, rows); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func ptr[T any](v T) *T { return &v }

// files is the fixture dataset: every required table, with n synthetic items besides the named ones.
func files(t testing.TB, n int) map[string][]byte {
	items := []item{{1, "Linen Cloth", 5}, {2, "Wolf Pelt", 6}, {3, "Linen Bandage", 7}, {4, "Wolf Fang", 6}}
	for i := 5; i <= n; i++ {
		items = append(items, item{int64(i), fmt.Sprintf("Filler Item %d", i), 10})
	}
	npcs := []npc{{100, "Timber Wolf", ptr("Elwynn Forest")}, {101, "Defias Thug", nil}}
	insts := []instance{{34, "The Stockade"}, {36, "Deadmines"}}
	recipes := []recipe{{300, "Heavy Linen Bandage", "First Aid"}}
	tables := map[string][]byte{
		"names":           nameIndex(t, items[:4], npcs, recipes, insts),
		"items":           enc(t, items),
		"quests":          enc(t, []quest{{10, ptr(int32(7))}, {11, nil}}),
		"npcs":            enc(t, npcs),
		"loot_sources":    enc(t, []loot{{2, "creature", 100, 200, 0.5}, {2, "object", 900, 10, 0.1}, {2, "crafted", 300, 0, 0}}),
		"instances":       enc(t, insts),
		"instance_bosses": enc(t, []boss{{36, "Edwin VanCleef", 2, nil}, {36, "Rhahk'Zor", 1, ptr(int32(101))}}),
		"recipes":         enc(t, recipes),
		"recipe_reagents": enc(t, []reagent{{300, 1, 2}}),
		"quest_lines":     enc(t, []line{{7, "Wolf Hunt", 10}, {7, "Wolf Hunt", 11}}),
	}
	return tables
}

func manifest(tables map[string][]byte) []byte {
	type entry struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
	}
	var es []entry
	for name, b := range tables {
		sum := sha256.Sum256(b)
		es = append(es, entry{name + ".parquet", hex.EncodeToString(sum[:])})
	}
	out, _ := json.Marshal(map[string]any{"dataset_version": "1.0", "game_build": "1.60.1.71037", "tables": es})
	return out
}

// host serves the fixture with per-request latency; it counts requests and the peak in flight.
type host struct {
	srv      *httptest.Server
	latency  atomic.Int64 // nanoseconds
	inflight atomic.Int64
	peak     atomic.Int64
	stall    atomic.Bool // when set, requests block until the client gives up
}

func newHost(t testing.TB, tables map[string][]byte) *host {
	h := &host{}
	h.latency.Store(int64(20 * time.Millisecond))
	files := map[string][]byte{"/manifest.json": manifest(tables)}
	for name, b := range tables {
		files["/"+name+".parquet"] = b
	}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := h.inflight.Add(1)
		defer h.inflight.Add(-1)
		for p := h.peak.Load(); n > p && !h.peak.CompareAndSwap(p, n); p = h.peak.Load() {
		}
		if h.stall.Load() {
			<-r.Context().Done() // returns only once the client's timeout has cancelled the request
			return
		}
		time.Sleep(time.Duration(h.latency.Load()))
		b, ok := files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(b)) // honours Range and HEAD
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func open(t testing.TB, h *host, lim Limits) *Dataset {
	t.Helper()
	d, err := Open(context.Background(), h.srv.URL, lim)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func connect(t testing.TB, d *Dataset) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := New(d).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func call(t testing.TB, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func TestToolListAndRefusal(t *testing.T) {
	t.Parallel()
	h := newHost(t, files(t, 4))
	cs := connect(t, open(t, h, Limits{}))
	listed, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, tl := range listed.Tools {
		got[tl.Name] = true
	}
	want := []string{"server_info", "item_lookup", "quest_lookup", "npc_lookup", "where_drops", "dungeon_info", "recipe_lookup"}
	if len(got) != len(want) {
		t.Errorf("tools = %v, want exactly %v", got, want)
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing tool %s", w)
		}
	}
	// Every tool of the full server that is not a knowledge tool is refused, by name, so a tool added
	// to the full server without updating CharacterTools fails here.
	st, ct := mcp.NewInMemoryTransports()
	full, _ := server.New(server.Config{}).Connect(context.Background(), st, nil)
	defer full.Close()
	fc, _ := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(context.Background(), ct, nil)
	defer fc.Close()
	all, err := fc.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tl := range all.Tools {
		if got[tl.Name] {
			continue
		}
		if text, isErr := call(t, cs, tl.Name, nil); !isErr || !strings.Contains(text, "not available in knowledge-only mode") {
			t.Errorf("%s = %q (error %v), want a refusal", tl.Name, text, isErr)
		}
	}
}

func TestLookups(t *testing.T) {
	t.Parallel()
	h := newHost(t, files(t, 4))
	d := open(t, h, Limits{})
	cs := connect(t, d)
	cases := []struct {
		tool string
		args map[string]any
		err  bool
		want []string
	}{
		{"item_lookup", map[string]any{"id": 2}, false, []string{"Wolf Pelt", "item_level: 6"}},
		{"item_lookup", map[string]any{"name": "wolf pelt"}, false, []string{"Item 2 Wolf Pelt"}},
		{"item_lookup", map[string]any{"name": "linen"}, false, []string{"Several items match", "1  Linen Cloth", "3  Linen Bandage"}},
		{"item_lookup", map[string]any{"name": "nothing"}, true, []string{"No item is named"}},
		{"item_lookup", map[string]any{"id": 999}, true, []string{"No item has id 999"}},
		{"item_lookup", nil, true, []string{"Give an item id or a name"}},
		{"quest_lookup", map[string]any{"id": 10}, false, []string{"Quest 10", "Quest line 7 Wolf Hunt, 2 quests", "2. quest 11"}},
		{"npc_lookup", map[string]any{"id": 100}, false, []string{"Timber Wolf", "zone_name: Elwynn Forest"}},
		{"npc_lookup", map[string]any{"name": "defias"}, false, []string{"Creature 101 Defias Thug"}},
		{"where_drops", map[string]any{"name": "Wolf Pelt"}, false, []string{"Timber Wolf (creature 100, Elwynn Forest): 50.0% of 200", "object 900: 10.0%", "crafted 300"}},
		{"where_drops", map[string]any{"id": 4}, false, []string{"none recorded"}},
		{"dungeon_info", map[string]any{"name": "deadm"}, false, []string{"Deadmines (map 36)", "boss: Rhahk'Zor (creature 101)"}},
		{"dungeon_info", map[string]any{"map_id": 34}, false, []string{"The Stockade"}},
		{"recipe_lookup", map[string]any{"name": "heavy linen"}, false, []string{"Heavy Linen Bandage (First Aid)", "reagent: 2 x Linen Cloth (item 1)"}},
		{"recipe_lookup", map[string]any{"spell_id": 301}, true, []string{"No recipe matches"}},
		{"server_info", nil, false, []string{`"mode":"knowledge-only"`, `"game_build":"1.60.1.71037"`}},
	}
	for _, c := range cases {
		text, isErr := call(t, cs, c.tool, c.args)
		if isErr != c.err {
			t.Errorf("%s %v: error = %v, text %q", c.tool, c.args, isErr, text)
		}
		for _, w := range c.want {
			if !strings.Contains(text, w) {
				t.Errorf("%s %v = %q, want it to contain %q", c.tool, c.args, text, w)
			}
		}
	}
}

// TestRequestsPerLookup records what a warm lookup costs in requests at 20 ms each, and that a
// cold server stays within bounds; docs/hosted.md quotes these figures.
func TestRequestsPerLookup(t *testing.T) {
	t.Parallel()
	h := newHost(t, files(t, 3000))
	d := open(t, h, Limits{})
	cs := connect(t, d)
	t.Logf("open: %d requests", d.Requests())
	for _, c := range []struct {
		tool string
		args map[string]any
		max  int64
	}{
		{"item_lookup", map[string]any{"id": 2}, 8},
		{"item_lookup", map[string]any{"name": "Wolf Pelt"}, 20},
		{"where_drops", map[string]any{"name": "Wolf Pelt"}, 25},
		{"dungeon_info", map[string]any{"name": "deadm"}, 20},
		{"npc_lookup", map[string]any{"name": "defias"}, 14},
	} {
		before := d.Requests()
		start := time.Now()
		if text, isErr := call(t, cs, c.tool, c.args); isErr {
			t.Fatalf("%s: %s", c.tool, text)
		}
		n := d.Requests() - before
		t.Logf("%s %v: %d requests, %v", c.tool, c.args, n, time.Since(start).Round(time.Millisecond))
		if n > c.max {
			t.Errorf("%s %v made %d requests, more than %d", c.tool, c.args, n, c.max)
		}
	}
}

func TestOpenRefusals(t *testing.T) {
	t.Parallel()
	tables := files(t, 3000)
	for _, c := range []struct {
		name string
		base func(h *host) string
		lim  Limits
		want string
	}{
		{"not a URL", func(*host) string { return "ftp://x" }, Limits{}, "must be an http(s) URL"},
		{"no manifest", func(h *host) string { return h.srv.URL + "/nope" }, Limits{}, "manifest"},
		{"size cap", func(h *host) string { return h.srv.URL }, Limits{MaxResponseBytes: 2048}, "size cap"},
	} {
		h := newHost(t, tables)
		if _, err := Open(context.Background(), c.base(h), c.lim); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want %q", c.name, err, c.want)
		}
	}
	delete(tables, "recipes")
	h := newHost(t, tables)
	if _, err := Open(context.Background(), h.srv.URL, Limits{}); err == nil || !strings.Contains(err.Error(), "no recipes table") {
		t.Errorf("missing table: err = %v", err)
	}
}

func TestTimeout(t *testing.T) {
	t.Parallel()
	h := newHost(t, files(t, 4))
	h.latency.Store(0)
	// The timeout is generous for Open on a loaded machine; the stalled request blocks until the
	// client's timeout cancels it, so no wall-clock bound is asserted: if the timeout did not cut
	// the request, the call would hang and the test would fail by its own deadline.
	d := open(t, h, Limits{Timeout: time.Second})
	cs := connect(t, d)
	h.stall.Store(true)
	text, isErr := call(t, cs, "npc_lookup", map[string]any{"id": 100})
	if !isErr || !strings.Contains(text, "Lookup failed") {
		t.Errorf("slow host: %q, error %v", text, isErr)
	}
}

func TestConcurrencyCap(t *testing.T) {
	t.Parallel()
	h := newHost(t, files(t, 4))
	d := open(t, h, Limits{MaxConcurrent: 3})
	cs := connect(t, d)
	h.peak.Store(0)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			call(t, cs, "npc_lookup", map[string]any{"name": "wolf"})
		}()
	}
	wg.Wait()
	if p := h.peak.Load(); p < 2 || p > 3 {
		t.Errorf("peak in-flight requests = %d, want 2 or 3 with the cap at 3", p)
	}
}

func TestCappedBody(t *testing.T) {
	for _, c := range []struct {
		name string
		body string
		max  int64
		ok   bool
	}{{"under", "abc", 5, true}, {"at the cap", "abcde", 5, true}, {"over", "abcdef", 5, false}} {
		tr := &limitedTransport{max: c.max, sem: make(chan struct{}, 1)}
		b := &cappedBody{rc: readCloser{strings.NewReader(c.body)}, left: tr.max, release: func() { tr.sem <- struct{}{}; <-tr.sem }}
		var buf bytes.Buffer
		_, err := buf.ReadFrom(b)
		if (err == nil) != c.ok || (!c.ok && !errors.Is(err, ErrTooLarge)) {
			t.Errorf("%s: err = %v", c.name, err)
		}
	}
}

type readCloser struct{ *strings.Reader }

func (readCloser) Close() error { return nil }
