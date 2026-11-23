package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The test binary doubles as the advisor: with this variable set it runs main, so the test
// drives a real `serve --stdio` process without building one.
func TestMain(m *testing.M) {
	if os.Getenv("WOWC_TEST_AS_MAIN") == "1" {
		main()
		return
	}
	os.Exit(m.Run())
}

// TestServeStdioContract is the contract every client relies on: initialize, then tools/list,
// over stdio, with the exact command the setup snippets name.
func TestServeStdioContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.Command(os.Args[0], "serve", "--stdio", "--savedvariables",
		filepath.Join("..", "..", "testdata", "foreverscribe", "tree_priest.lua"))
	cmd.Env = append(os.Environ(), "WOWC_TEST_AS_MAIN=1")
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "contract"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err) // initialize failed
	}
	defer cs.Close()
	if got := cs.InitializeResult().ServerInfo.Name; got != "wowc-advisor" {
		t.Errorf("server name = %q", got)
	}
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	if got := strings.Join(names, ","); got != "character_brief,doctor,dungeon_plan,farm_spots,get_character_profile,inventory_review,list_characters,profession_plan,quest_chain,quest_plan,server_info,set_character_profile,talent_plan,talent_tree,update_digest,upgrade_finder,upgrade_paths,whats_next" {
		t.Errorf("tools = %s", got)
	}
}
