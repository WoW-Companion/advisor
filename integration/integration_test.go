//go:build integration

// Package integration runs every advisor tool against a real built dataset, with synthetic captures
// made from the dataset's own ids, and prints a table of what each tool returned. It is not part of
// make verify: run it with make integration (docs/eval.md).
package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/hosted"
	"github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/server"
	"github.com/WoW-Companion/advisor/internal/store"
)

// DatasetEnv names the built dataset directory (<dir>/manifest.json and the Parquet tables).
const DatasetEnv = "WOWC_INTEGRATION_DATASET"

type observation struct {
	tool, char string
	latency    time.Duration
	isErr      bool
	n          int    // items in the answer's main list
	text       string // the answer's text, for the first lines of the report
	fields     map[string]bool
}

// leaves records, per JSON path, whether the value was non-null and non-zero (true) or null, zero,
// empty or false (false). Array elements share one path.
func leaves(path string, v any, out map[string]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, e := range x {
			if path == ".slots" || path == ".groups" || path == ".lines" { // keyed by slot, group or section
				k = "*"
			}
			leaves(path+"."+k, e, out)
		}
	case []any:
		for _, e := range x {
			leaves(path+"[]", e, out)
		}
		if len(x) == 0 {
			out[path+"[]"] = out[path+"[]"] || false
		}
	case nil:
		out[path] = out[path] || false
	case float64:
		out[path] = out[path] || x != 0
	case string:
		out[path] = out[path] || x != ""
	case bool:
		out[path] = out[path] || x
	}
}

// mainCount is how many results a tool's answer holds: the elements of every list and map in it,
// not counting the text rendering. An answer with no list or map at all (server_info) counts as one.
func mainCount(m map[string]any) int {
	collections := false
	var count func(v any) int
	count = func(v any) int {
		switch x := v.(type) {
		case []any:
			collections = true
			return len(x)
		case map[string]any:
			collections = true
			n := 0
			for _, e := range x {
				n += count(e)
			}
			return n
		case float64: // a count, as in update_digest's lines per section
			return int(x)
		}
		return 0
	}
	n := 0
	for k, v := range m {
		if _, isNum := v.(float64); k != "text" && !isNum {
			n += count(v)
		}
	}
	if !collections {
		return 1
	}
	return n
}

func call(ctx context.Context, cs *mcp.ClientSession, tool, char string, args map[string]any) observation {
	o := observation{tool: tool, char: char, fields: map[string]bool{}}
	start := time.Now()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	o.latency = time.Since(start)
	if err != nil {
		o.isErr, o.text = true, err.Error()
		return o
	}
	if len(res.Content) > 0 {
		if t, ok := res.Content[0].(*mcp.TextContent); ok {
			o.text = t.Text
		}
	}
	o.isErr = res.IsError
	var m map[string]any
	if res.StructuredContent != nil {
		raw, _ := json.Marshal(res.StructuredContent)
		json.Unmarshal(raw, &m)
	} else {
		json.Unmarshal([]byte(o.text), &m)
	}
	if m != nil && !o.isErr {
		o.n = mainCount(m)
		leaves("", m, o.fields)
		if t, ok := m["text"].(string); ok {
			o.text = t
		}
	} else if !o.isErr && o.text != "" {
		o.n = 1
	}
	return o
}

func connect(t *testing.T, s *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	if _, err := s.Connect(ctx, st, nil); err != nil {
		t.Fatal(err)
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "integration"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func TestIntegration(t *testing.T) {
	dir := os.Getenv(DatasetEnv)
	if dir == "" {
		t.Skip(DatasetEnv + " is not set; run make integration")
	}
	manifest, err := store.ReadManifest(dir)
	if err != nil || manifest == nil {
		t.Fatalf("no manifest in %s: %v", dir, err)
	}
	build := fmt.Sprint(manifest["game_build"])
	ds := store.NewDir(dir)
	tmp := t.TempDir()
	caps, err := writeCaptures(tmp, ds, build)
	if err != nil {
		t.Fatal(err)
	}
	game := filepath.Join(tmp, "game")
	slot := filepath.Join(game, "Interface", "AddOns", "WoWCompanion_Digest")
	if err := os.MkdirAll(slot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	var obs []observation
	names := make([]string, 0, len(caps))
	for n := range caps {
		names = append(names, n)
	}
	sort.Strings(names)

	farmItem := probeFarmItem(ds)
	chainQuest := probeChainQuest(ds)
	for _, name := range names {
		sv := filepath.Join(game, "WTF", "Account", "TEST", "SavedVariables", "ForeverScribe.lua")
		os.MkdirAll(filepath.Dir(sv), 0o755)
		data, _ := os.ReadFile(caps[name])
		os.WriteFile(sv, data, 0o644)
		cs := connect(t, server.New(server.Config{Dataset: dir, SavedVariables: sv, Cache: filepath.Join(tmp, "cache")}))
		c := map[string]any{"character": name}
		for _, tc := range []struct {
			tool string
			args map[string]any
		}{
			{"list_characters", nil}, {"character_brief", c}, {"whats_next", c}, {"upgrade_finder", c}, {"update_digest", c},
			{"profession_plan", c}, {"talent_plan", c}, {"dungeon_plan", c}, {"inventory_review", c},
			{"farm_spots", map[string]any{"character": name, "item": farmItem}},
			{"quest_chain", map[string]any{"quest": chainQuest}}, {"doctor", nil},
		} {
			obs = append(obs, call(ctx, cs, tc.tool, name, tc.args))
		}
	}
	obs = append(obs, knowledgeOnly(t, ctx, dir, ds)...)
	report(t, obs)
}

// probeFarmItem is an item id with a creature drop row, so farm_spots has something to find.
func probeFarmItem(ds store.DataStore) string {
	rows, err := ds.Filter("loot_sources", store.Range{Column: "source_kind", Min: "creature", Max: "creature"}, store.Range{Column: "drops", Min: int64(5)})
	if err != nil || len(rows) == 0 {
		return "1"
	}
	return fmt.Sprint(num(pick(rows, 1)[0]["item_id"]))
}

// probeChainQuest is a quest that sits in a quest line.
func probeChainQuest(ds store.DataStore) string {
	rows, err := ds.Filter("quest_lines", store.Range{Column: "position", Min: int64(2)})
	if err != nil || len(rows) == 0 {
		return "1"
	}
	return fmt.Sprint(num(pick(rows, 1)[0]["quest_id"]))
}

// knowledgeOnly serves the dataset over HTTP with range support, the way a host would, and calls
// the knowledge-only lookups through it.
func knowledgeOnly(t *testing.T, ctx context.Context, dir string, ds store.DataStore) []observation {
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	t.Cleanup(srv.Close)
	d, err := hosted.Open(ctx, srv.URL, hosted.Limits{})
	if err != nil {
		t.Errorf("knowledge-only open: %v", err)
		return nil
	}
	cs := connect(t, hosted.New(d))
	itemID, _ := strconv.ParseInt(probeFarmItem(ds), 10, 64)
	questID, _ := strconv.ParseInt(probeChainQuest(ds), 10, 64)
	var out []observation
	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"server_info", nil},
		{"item_lookup", map[string]any{"id": itemID}},
		{"item_lookup", map[string]any{"name": firstName(ds, "items")}},
		{"quest_lookup", map[string]any{"id": questID}},
		{"npc_lookup", map[string]any{"name": firstName(ds, "npcs")}},
		{"where_drops", map[string]any{"id": itemID}},
		{"dungeon_info", map[string]any{"name": firstName(ds, "instances")}},
		{"recipe_lookup", map[string]any{"name": firstName(ds, "recipes")}},
	} {
		out = append(out, call(ctx, cs, tc.tool, "knowledge-only", tc.args))
	}
	return out
}

func report(t *testing.T, obs []observation) {
	var b strings.Builder
	b.WriteString("\nINTEGRATION RESULTS\n")
	fmt.Fprintf(&b, "%-18s %-15s %-9s %6s %10s\n", "tool", "character", "result", "items", "latency")
	nulls := map[string]map[string]bool{} // tool -> path -> ever non-null
	for _, o := range obs {
		res := "non-empty"
		switch {
		case o.isErr:
			res = "ERROR"
		case o.n == 0:
			res = "empty"
		}
		fmt.Fprintf(&b, "%-18s %-15s %-9s %6d %10s\n", o.tool, o.char, res, o.n, o.latency.Round(time.Millisecond))
		if os.Getenv("WOWC_INTEGRATION_TEXT") != "" && (strings.Contains(os.Getenv("WOWC_INTEGRATION_TEXT"), o.char)) {
			fmt.Fprintf(&b, "%s\n", o.text)
		}
		if o.isErr {
			fmt.Fprintf(&b, "    error: %s\n", firstLine(o.text))
			t.Errorf("%s on %s: %s", o.tool, o.char, firstLine(o.text))
		}
		if nulls[o.tool] == nil {
			nulls[o.tool] = map[string]bool{}
		}
		for p, ok := range o.fields {
			nulls[o.tool][p] = nulls[o.tool][p] || ok
		}
		if o.latency > time.Second {
			t.Errorf("%s on %s took %s, over 1 s", o.tool, o.char, o.latency)
		}
	}
	b.WriteString("\nFIELDS NULL, ZERO OR EMPTY IN EVERY ANSWER\n")
	tools := make([]string, 0, len(nulls))
	for tl := range nulls {
		tools = append(tools, tl)
	}
	sort.Strings(tools)
	for _, tl := range tools {
		var dead []string
		for p, ok := range nulls[tl] {
			if !ok {
				dead = append(dead, p)
			}
		}
		sort.Strings(dead)
		if len(dead) > 0 {
			fmt.Fprintf(&b, "%-18s %s\n", tl, strings.Join(dead, " "))
		}
	}
	fmt.Print(b.String())
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// firstName is a name from the middle of a table, for a lookup by name.
func firstName(ds store.DataStore, table string) string {
	rows, err := ds.Filter(table, store.Range{Column: "name", Min: "A"})
	if err != nil || len(rows) == 0 {
		return "x"
	}
	return text(rows[len(rows)/2]["name"])
}

// TestUnobtainableCensus counts, over the real items table, what internal/items drops, per pattern,
// with example names, so a person can see nothing real was caught.
func TestUnobtainableCensus(t *testing.T) {
	dir := os.Getenv(DatasetEnv)
	if dir == "" {
		t.Skip(DatasetEnv + " is not set; run make integration")
	}
	rows, err := store.NewDir(dir).Filter("items", store.Range{Column: "id", Min: 0, Max: 1 << 31})
	if err != nil {
		t.Fatal(err)
	}
	counts, examples, total := map[string]int{}, map[string][]string{}, 0
	for _, r := range rows {
		name, _ := r["name"].(string)
		if label := items.Match(name); label != "" {
			total++
			counts[label]++
			if len(examples[label]) < 10 {
				examples[label] = append(examples[label], name)
			}
		}
	}
	t.Logf("unobtainable census: %d of %d items", total, len(rows))
	for _, p := range items.Patterns {
		t.Logf("  %-12q %5d  e.g. %q", p.Label, counts[p.Label], examples[p.Label])
	}
}
