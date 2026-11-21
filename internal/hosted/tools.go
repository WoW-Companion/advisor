package hosted

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/server"
	"github.com/WoW-Companion/advisor/internal/store"
)

// NotAvailable is what a character tool answers in knowledge-only mode.
const NotAvailable = "not available in knowledge-only mode: this server holds game data only, and reads no character files"

// CharacterTools are the full server's tools that read the character capture or local files.
var CharacterTools = []string{"list_characters", "character_brief", "whats_next", "upgrade_finder", "profession_plan",
	"talent_plan", "dungeon_plan", "farm_spots", "inventory_review", "update_digest", "doctor"}

const (
	maxMatches = 10 // candidates a name lookup lists
	maxDrops   = 15 // drop sources where_drops lists
	maxRows    = 20 // related rows a lookup lists
)

// Info is the server_info result.
type Info struct {
	Version         string `json:"version"`
	Mode            string `json:"mode"`
	Dataset         string `json:"dataset" jsonschema:"dataset URL, without credentials"`
	ManifestVersion string `json:"manifest_version"`
	GameBuild       string `json:"game_build"`
	Tables          int    `json:"tables"`
	NameIndexItems  int    `json:"name_index_items"`
	NameIndexBytes  int64  `json:"name_index_bytes" jsonschema:"estimated heap held by the item name index"`
	TimeoutMS       int64  `json:"request_timeout_ms"`
	MaxResponse     int64  `json:"max_response_bytes"`
	MaxConcurrent   int    `json:"max_concurrent_requests"`
}

// Describe reports on this server and its dataset.
func (d *Dataset) Describe() Info {
	return Info{Version: server.Version, Mode: "knowledge-only", Dataset: d.Base, ManifestVersion: d.Manifest.DatasetVersion,
		GameBuild: d.Manifest.GameBuild, Tables: len(d.Manifest.Tables), NameIndexItems: d.Names.Len(), NameIndexBytes: d.Names.Bytes(),
		TimeoutMS: d.Limits.Timeout.Milliseconds(), MaxResponse: d.Limits.MaxResponseBytes, MaxConcurrent: d.Limits.MaxConcurrent}
}

type noInput struct{}

// ItemInput is the item_lookup and where_drops argument; give an id or a name.
type ItemInput struct {
	ID   int64  `json:"id,omitempty" jsonschema:"item id"`
	Name string `json:"name,omitempty" jsonschema:"item name, exact or part of it"`
}

// IDNameInput is the quest_lookup and npc_lookup argument.
type IDNameInput struct {
	ID   int64  `json:"id,omitempty" jsonschema:"id"`
	Name string `json:"name,omitempty" jsonschema:"name, or part of it (npcs only)"`
}

// DungeonInput is the dungeon_info argument.
type DungeonInput struct {
	MapID int64  `json:"map_id,omitempty" jsonschema:"map id"`
	Name  string `json:"name,omitempty" jsonschema:"dungeon or raid name, or part of it"`
}

// RecipeInput is the recipe_lookup argument.
type RecipeInput struct {
	SpellID int64  `json:"spell_id,omitempty" jsonschema:"recipe spell id"`
	Name    string `json:"name,omitempty" jsonschema:"recipe name, or part of it"`
}

// Result is what every lookup returns: the rows found and their plain-text rendering.
type Result struct {
	Rows []store.Row `json:"rows"`
	Text string      `json:"text"`
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

func i64(v any) int64 { n, _ := v.(int64); return n }

func str(v any) string { s, _ := v.(string); return s }

// render prints a row as `key: value` pairs in key order, leaving out nulls and lists.
func render(label string, r store.Row) string {
	keys := make([]string, 0, len(r))
	for k, v := range r {
		if _, list := v.([]any); v != nil && !list {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(label)
	for _, k := range keys {
		fmt.Fprintf(&b, "\n  %s: %v", k, r[k])
	}
	return b.String()
}

func fail(err error) (*mcp.CallToolResult, Result, error) {
	return toolError("Lookup failed: " + err.Error()), Result{Rows: []store.Row{}}, nil
}

func missing(msg string) (*mcp.CallToolResult, Result, error) {
	return toolError(msg), Result{Rows: []store.Row{}}, nil
}

func found(rows []store.Row, parts ...string) (*mcp.CallToolResult, Result, error) {
	if rows == nil {
		rows = []store.Row{}
	}
	return nil, Result{Rows: rows, Text: strings.Join(parts, "\n")}, nil
}

// resolveItem turns an id or a name into one item id. msg is set when the answer is an error or
// a list of candidates rather than one item.
func (d *Dataset) resolveItem(in ItemInput) (id int64, msg string) {
	if in.ID > 0 {
		return in.ID, ""
	}
	if strings.TrimSpace(in.Name) == "" {
		return 0, "Give an item id or a name."
	}
	hits, more := d.Names.Find(in.Name, maxMatches)
	switch len(hits) {
	case 0:
		return 0, "No item is named '" + in.Name + "'."
	case 1:
		return hits[0].ID, ""
	}
	var b strings.Builder
	b.WriteString("Several items match '" + in.Name + "'; ask again with the id:")
	for _, h := range hits {
		fmt.Fprintf(&b, "\n  %d  %s", h.ID, h.Name)
	}
	if more {
		b.WriteString("\n  (more not shown)")
	}
	return 0, b.String()
}

// refuse answers with resolveItem's message: a candidate list is an answer, the rest are errors.
func (d *Dataset) refuse(msg string) (*mcp.CallToolResult, Result, error) {
	if strings.HasPrefix(msg, "Several items match") {
		return found(nil, msg)
	}
	return missing(msg)
}

func (d *Dataset) itemLookup(_ context.Context, _ *mcp.CallToolRequest, in ItemInput) (*mcp.CallToolResult, Result, error) {
	id, msg := d.resolveItem(in)
	if msg != "" {
		return d.refuse(msg)
	}
	row, err := d.Store.Get("items", id)
	if errors.Is(err, store.ErrNotFound) {
		return missing(fmt.Sprintf("No item has id %d.", id))
	}
	if err != nil {
		return fail(err)
	}
	return found([]store.Row{row}, render(fmt.Sprintf("Item %d %s", id, str(row["name"])), row))
}

func (d *Dataset) questLookup(_ context.Context, _ *mcp.CallToolRequest, in IDNameInput) (*mcp.CallToolResult, Result, error) {
	if in.ID <= 0 {
		return missing("Give a quest id; the dataset holds quests by id only.")
	}
	row, err := d.Store.Get("quests", in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return missing(fmt.Sprintf("No quest has id %d.", in.ID))
	}
	if err != nil {
		return fail(err)
	}
	parts := []string{render(fmt.Sprintf("Quest %d", in.ID), row)}
	if line := i64(row["quest_line_id"]); line > 0 {
		steps, err := d.Store.Filter("quest_lines", store.Range{Column: "line_id", Min: line, Max: line})
		if err == nil && len(steps) > 0 {
			parts = append(parts, fmt.Sprintf("Quest line %d %s, %d quests, in order:", line, str(steps[0]["name"]), len(steps)))
			for i, s := range steps {
				if i == maxRows {
					parts = append(parts, "  (more not shown)")
					break
				}
				parts = append(parts, fmt.Sprintf("  %d. quest %d", i+1, i64(s["quest_id"])))
			}
		}
	}
	return found([]store.Row{row}, parts...)
}

func (d *Dataset) npcLookup(_ context.Context, _ *mcp.CallToolRequest, in IDNameInput) (*mcp.CallToolResult, Result, error) {
	var rows []store.Row
	switch {
	case in.ID > 0:
		row, err := d.Store.Get("npcs", in.ID)
		if errors.Is(err, store.ErrNotFound) {
			return missing(fmt.Sprintf("No creature has id %d.", in.ID))
		}
		if err != nil {
			return fail(err)
		}
		rows = []store.Row{row}
	case strings.TrimSpace(in.Name) != "":
		// npcs have no prebuilt index: this reads the name column.
		var err error
		if rows, err = d.Store.SearchName("npcs", in.Name, maxMatches); err != nil {
			return fail(err)
		}
		if len(rows) == 0 {
			return missing("No creature is named '" + in.Name + "'.")
		}
	default:
		return missing("Give a creature id or a name.")
	}
	parts := make([]string, len(rows))
	for i, r := range rows {
		parts[i] = render(fmt.Sprintf("Creature %d %s", i64(r["id"]), str(r["name"])), r)
	}
	return found(rows, parts...)
}

func (d *Dataset) whereDrops(_ context.Context, _ *mcp.CallToolRequest, in ItemInput) (*mcp.CallToolResult, Result, error) {
	id, msg := d.resolveItem(in)
	if msg != "" {
		return d.refuse(msg)
	}
	item, err := d.Store.Get("items", id)
	if errors.Is(err, store.ErrNotFound) {
		return missing(fmt.Sprintf("No item has id %d.", id))
	}
	if err != nil {
		return fail(err)
	}
	src, err := d.Store.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
	if err != nil {
		return fail(err)
	}
	sort.SliceStable(src, func(a, b int) bool { return src[a]["rate"].(float64) > src[b]["rate"].(float64) })
	parts := []string{fmt.Sprintf("Sources of %s (item %d), best drop rate first:", str(item["name"]), id)}
	if len(src) == 0 {
		parts = append(parts, "  none recorded in the dataset")
	}
	var shown []store.Row
	for i, s := range src {
		if i == maxDrops {
			parts = append(parts, fmt.Sprintf("  (%d more not shown)", len(src)-maxDrops))
			break
		}
		shown = append(shown, s)
		kind, sid := str(s["source_kind"]), i64(s["source_id"])
		who := fmt.Sprintf("%s %d", kind, sid)
		if kind == "creature" {
			if n, err := d.Store.Get("npcs", sid); err == nil {
				who = fmt.Sprintf("%s (creature %d, %s)", str(n["name"]), sid, orUnknown(str(n["zone_name"])))
			}
		}
		line := "  " + who
		if kind == "creature" || kind == "object" {
			line += fmt.Sprintf(": %.1f%% of %d loot windows", 100*s["rate"].(float64), i64(s["kills"]))
		}
		parts = append(parts, line)
	}
	return found(shown, parts...)
}

func orUnknown(s string) string {
	if s == "" {
		return "zone unknown"
	}
	return s
}

func (d *Dataset) dungeonInfo(_ context.Context, _ *mcp.CallToolRequest, in DungeonInput) (*mcp.CallToolResult, Result, error) {
	all, err := d.Store.Filter("instances", store.Range{Column: "map_id", Min: int64(0)})
	if err != nil {
		return fail(err)
	}
	needle := strings.ToLower(strings.TrimSpace(in.Name))
	if in.MapID <= 0 && needle == "" {
		return missing("Give a map id or a dungeon name.")
	}
	var rows []store.Row
	for _, r := range all {
		if in.MapID > 0 && i64(r["map_id"]) == in.MapID || in.MapID <= 0 && strings.Contains(strings.ToLower(str(r["name"])), needle) {
			rows = append(rows, r)
		}
	}
	if len(rows) == 0 {
		return missing("No dungeon or raid matches.")
	}
	if len(rows) > maxMatches {
		rows = rows[:maxMatches]
	}
	var parts []string
	for _, r := range rows {
		m := i64(r["map_id"])
		parts = append(parts, render(fmt.Sprintf("%s (map %d)", str(r["name"]), m), r))
		bosses, err := d.Store.Filter("instance_bosses", store.Range{Column: "map_id", Min: m, Max: m})
		if err != nil {
			return fail(err)
		}
		sort.SliceStable(bosses, func(a, b int) bool { return i64(bosses[a]["order_index"]) < i64(bosses[b]["order_index"]) })
		for _, b := range bosses {
			line := "  boss: " + str(b["name"])
			if c := i64(b["creature_id"]); c > 0 {
				line += fmt.Sprintf(" (creature %d)", c)
			}
			parts = append(parts, line)
		}
	}
	return found(rows, parts...)
}

func (d *Dataset) recipeLookup(_ context.Context, _ *mcp.CallToolRequest, in RecipeInput) (*mcp.CallToolResult, Result, error) {
	var rows []store.Row
	var err error
	switch {
	case in.SpellID > 0:
		rows, err = d.Store.Filter("recipes", store.Range{Column: "spell_id", Min: in.SpellID, Max: in.SpellID})
	case strings.TrimSpace(in.Name) != "":
		// recipes have no prebuilt index: this reads the name column.
		rows, err = d.Store.SearchName("recipes", in.Name, maxMatches)
	default:
		return missing("Give a recipe spell id or a name.")
	}
	if err != nil {
		return fail(err)
	}
	if len(rows) == 0 {
		return missing("No recipe matches.")
	}
	var parts []string
	for _, r := range rows {
		spell := i64(r["spell_id"])
		parts = append(parts, render(fmt.Sprintf("Recipe %d %s (%s)", spell, str(r["name"]), str(r["skill_line_name"])), r))
		regs, err := d.Store.Filter("recipe_reagents", store.Range{Column: "spell_id", Min: spell, Max: spell})
		if err != nil {
			return fail(err)
		}
		for _, g := range regs {
			rid := i64(g["reagent_item_id"])
			name := "item"
			if it, err := d.Store.Get("items", rid); err == nil {
				name = str(it["name"])
			}
			parts = append(parts, fmt.Sprintf("  reagent: %d x %s (item %d)", i64(g["count"]), name, rid))
		}
	}
	return found(rows, parts...)
}

// New builds the knowledge-only server: the tools above and nothing that reads a character file.
// A call to a character tool is answered with NotAvailable, not listed.
func New(d *Dataset) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "wowc-advisor", Version: server.Version}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "server_info",
		Description: "Return the advisor version, the dataset URL, its game build and the request limits. This server is knowledge-only: game data, no characters."},
		func(context.Context, *mcp.CallToolRequest, noInput) (*mcp.CallToolResult, Info, error) {
			return nil, d.Describe(), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "item_lookup",
		Description: "One item by id or by name (exact, else part of it): level, required level, quality, slot, class mask, stats, prices. A name that matches several items lists them with their ids."},
		d.itemLookup)
	mcp.AddTool(s, &mcp.Tool{Name: "quest_lookup",
		Description: "One quest by id: its quest line and position, map and zone. The dataset holds no quest names or levels."},
		d.questLookup)
	mcp.AddTool(s, &mcp.Tool{Name: "npc_lookup",
		Description: "A creature by id, or by part of its name (up to 10): classification, level range, zone."},
		d.npcLookup)
	mcp.AddTool(s, &mcp.Tool{Name: "where_drops",
		Description: "Where an item comes from, by item id or name: creatures, objects, quest rewards and crafting, best drop rate first, with the creature's zone."},
		d.whereDrops)
	mcp.AddTool(s, &mcp.Tool{Name: "dungeon_info",
		Description: "A dungeon or raid by map id or part of its name: players, level range, entrance and its bosses in order."},
		d.dungeonInfo)
	mcp.AddTool(s, &mcp.Tool{Name: "recipe_lookup",
		Description: "A profession recipe by spell id or part of its name: profession, skill thresholds, the item it makes and its reagents."},
		d.recipeLookup)

	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if call, ok := req.(*mcp.CallToolRequest); ok && method == "tools/call" && call.Params != nil {
				for _, name := range CharacterTools {
					if call.Params.Name == name {
						return toolError(name + " is " + NotAvailable + "."), nil
					}
				}
			}
			return next(ctx, method, req)
		}
	})
	return s
}
