package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func fixture(name string) string {
	return filepath.Join("..", "..", "testdata", "foreverscribe", name)
}

func TestDescribe(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"version":"2026.11.1","game_build":12345}`), 0o644)
	bad := filepath.Join(dir, "bad.lua")
	os.WriteFile(bad, []byte("ForeverScribeDB = {"), 0o644)

	cases := []struct {
		name string
		cfg  Config
		want func(Info) bool
	}{
		{"nothing configured", Config{}, func(i Info) bool {
			return i.Dataset == "no dataset configured" && i.SavedVariables == "no SavedVariables file configured" && i.Characters == nil
		}},
		{"dataset", Config{Dataset: dir}, func(i Info) bool {
			return i.Dataset == "ok" && *i.ManifestVersion == "2026.11.1" && *i.GameBuild == "12345"
		}},
		{"no manifest", Config{Dataset: t.TempDir()}, func(i Info) bool { return i.Dataset == "no manifest found" }},
		{"two characters", Config{SavedVariables: fixture("two_characters.lua")}, func(i Info) bool {
			return i.SavedVariables == "ok" && *i.Characters == 2 && *i.CharactersCapturedAt == "2023-11-18T09:33:20Z" && i.Client == nil
		}},
		{"client block", Config{SavedVariables: fixture("tree_priest.lua")}, func(i Info) bool {
			return *i.Client == ClientInfo{"9.9.9", "99999", "99999"}
		}},
		{"missing file", Config{SavedVariables: filepath.Join(dir, "none.lua")}, func(i Info) bool { return i.SavedVariables == "file not found" }},
		{"parse error", Config{SavedVariables: bad}, func(i Info) bool { return strings.HasPrefix(i.SavedVariables, "could not parse: line 1") }},
	}
	for _, c := range cases {
		if info := Describe(c.cfg); !c.want(info) || info.Version != Version {
			t.Errorf("%s: %+v", c.name, info)
		}
	}
}

// exercise lists the tools and calls each over an already connected session.
func exercise(t *testing.T, cs *mcp.ClientSession) {
	t.Helper()
	ctx := context.Background()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	if strings.Join(names, ",") != "character_brief,list_characters,server_info" {
		t.Errorf("tools = %v", names)
	}
	call := func(name string, args map[string]any) (string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return res.Content[0].(*mcp.TextContent).Text, res.IsError
	}
	if text, isErr := call("server_info", nil); isErr || !strings.Contains(text, `"build":"99999"`) {
		t.Errorf("server_info = %s", text)
	}
	if text, isErr := call("list_characters", nil); isErr || !strings.Contains(text, `"name":"Testpriest"`) {
		t.Errorf("list_characters = %s", text)
	}
	text, isErr := call("character_brief", map[string]any{"character": "testpriest"})
	for _, want := range []string{"Spec: Test Discipline (14 points)", "1 point unspent", "bank not captured yet", "rested 6 XP", "Client: 9.9.9 (build 99999"} {
		if isErr || !strings.Contains(text, want) {
			t.Errorf("character_brief lacks %q:\n%s", want, text)
		}
	}
	if text, isErr := call("character_brief", map[string]any{"character": "Nobody"}); !isErr || !strings.Contains(text, "No captured character") {
		t.Errorf("unknown character = %q, error %v", text, isErr)
	}
}

func TestToolsOverMemoryTransport(t *testing.T) {
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	s := New(Config{SavedVariables: fixture("tree_priest.lua")})
	ss, err := s.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	exercise(t, cs)
}

func TestToolsOverHTTP(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(HTTPHandler(New(Config{SavedVariables: fixture("tree_priest.lua")})))
	defer srv.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, &mcp.StreamableClientTransport{Endpoint: srv.URL}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	exercise(t, cs)
}

func TestToolsWithoutFileSaySo(t *testing.T) {
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, _ := New(Config{}).Connect(ctx, st, nil)
	defer ss.Close()
	cs, _ := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	defer cs.Close()
	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_characters"})
	if err != nil || !res.IsError || !strings.Contains(res.Content[0].(*mcp.TextContent).Text, "No SavedVariables file configured") {
		t.Errorf("res = %+v, err %v", res, err)
	}
}

func TestCompareBuilds(t *testing.T) {
	for _, c := range []struct {
		name         string
		game         string
		client       *ClientInfo
		match, warns bool
		compared     bool
	}{
		{"match", "1.60.1.71037", &ClientInfo{Version: "1.60.1", Build: "71037"}, true, false, true},
		{"mismatch", "1.60.1.71037", &ClientInfo{Version: "1.60.2", Build: "71500"}, false, true, true},
		{"no client", "1.60.1.71037", nil, false, false, false},
	} {
		info := Info{GameBuild: &c.game, Client: c.client}
		compareBuilds(&info)
		if (info.DatasetMatchesClient != nil) != c.compared || (c.compared && *info.DatasetMatchesClient != c.match) ||
			(info.DatasetWarning != nil) != c.warns {
			t.Errorf("%s: %+v", c.name, info)
		}
	}
}
