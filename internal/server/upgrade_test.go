package server

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/parquet-go/parquet-go"
)

type upItem struct {
	ID            int64   `parquet:"id"`
	Name          string  `parquet:"name"`
	ItemLevel     int32   `parquet:"item_level"`
	RequiredLevel int32   `parquet:"required_level"`
	Quality       int32   `parquet:"quality"`
	ClassID       int32   `parquet:"class_id"`
	SubclassID    int32   `parquet:"subclass_id"`
	Slot          int32   `parquet:"slot"`
	ClassMask     int32   `parquet:"allowable_class_mask"`
	StatTypes     []int16 `parquet:"stat_types,list"`
	StatsStatus   string  `parquet:"stats_status"`
	Bonding       int32   `parquet:"bonding"`
}

type upLoot struct {
	ItemID int64  `parquet:"item_id"`
	Kind   string `parquet:"source_kind"`
	ID     int64  `parquet:"source_id"`
	Kills  int32  `parquet:"kills"`
	Drops  int32  `parquet:"drops"`
}

type upNPC struct {
	ID   int64  `parquet:"id"`
	Name string `parquet:"name"`
}

func writeTable[T any](t *testing.T, dir, name string, rows []T) {
	t.Helper()
	if err := parquet.WriteFile(filepath.Join(dir, name+".parquet"), rows); err != nil {
		t.Fatal(err)
	}
}

// The overlay asks with brief=true; the result must pass the tool's output schema.
func TestUpgradeFinderBriefOverMCP(t *testing.T) {
	dir := t.TempDir()
	writeTable(t, dir, "items", []upItem{{1, "Test Hood", 60, 1, 3, 4, 1, 1, -1, []int16{5, 7}, "known", 0}})
	writeTable(t, dir, "loot_sources", []upLoot{{1, "creature", 500, 10, 2}})
	writeTable(t, dir, "npcs", []upNPC{{500, "Test Boss"}})
	if _, err := os.Stat(filepath.Join(dir, "items.parquet")); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := New(Config{SavedVariables: fixture("tree_priest.lua"), Dataset: dir}).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for _, brief := range []bool{true, false} {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "upgrade_finder", Arguments: map[string]any{"character": "testpriest", "brief": brief}})
		if err != nil {
			t.Fatalf("brief=%v: %v", brief, err)
		}
		text := res.Content[0].(*mcp.TextContent).Text
		if res.IsError || strings.Contains(strings.ToLower(text), "validating") {
			t.Errorf("brief=%v: tool error: %s", brief, text)
		}
	}
}
