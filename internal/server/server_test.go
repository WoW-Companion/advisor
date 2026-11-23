package server

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
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
	if strings.Join(names, ",") != "character_brief,doctor,dungeon_plan,farm_spots,get_character_profile,inventory_review,list_characters,profession_plan,quest_chain,quest_plan,server_info,set_character_profile,talent_plan,talent_tree,update_digest,upgrade_finder,upgrade_paths,whats_next" {
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
	if text, isErr := call("doctor", nil); isErr || !strings.Contains(text, `"ok":false`) || !strings.Contains(text, `"name":"SavedVariables","status":"pass"`) {
		t.Errorf("doctor = %s", text)
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
	if text, isErr := call("whats_next", map[string]any{"character": "testpriest", "limit": 3}); isErr || !strings.Contains(text, `"steps":`) || !strings.Contains(text, "What's next for Testpriest") {
		t.Errorf("whats_next = %s", text)
	}
	if text, isErr := call("upgrade_finder", map[string]any{"character": "testpriest"}); !isErr || !strings.Contains(text, "No dataset configured") {
		t.Errorf("upgrade_finder without dataset = %q, error %v", text, isErr)
	}
	if text, isErr := call("upgrade_paths", map[string]any{"character": "testpriest", "item": "Hood"}); !isErr || !strings.Contains(text, "No dataset configured") {
		t.Errorf("upgrade_paths without dataset = %q, error %v", text, isErr)
	}
	if text, isErr := call("upgrade_finder", map[string]any{"character": "testpriest", "slot": "knees"}); !isErr || !strings.Contains(text, "Unknown slot") {
		t.Errorf("upgrade_finder bad slot = %q, error %v", text, isErr)
	}
	if text, isErr := call("talent_plan", map[string]any{"character": "testpriest"}); !isErr || !strings.Contains(text, "No dataset configured") {
		t.Errorf("talent_plan without dataset = %q, error %v", text, isErr)
	}
	if text, isErr := call("dungeon_plan", map[string]any{"character": "testpriest"}); !isErr || !strings.Contains(text, "No dataset configured") {
		t.Errorf("dungeon_plan without dataset = %q, error %v", text, isErr)
	}
	if text, isErr := call("inventory_review", map[string]any{"character": "testpriest"}); isErr || !strings.Contains(text, "Bag slots freed: 0") || !strings.Contains(text, "No dataset configured") {
		t.Errorf("inventory_review without dataset = %q, error %v", text, isErr)
	}
	if text, isErr := call("update_digest", map[string]any{"character": "testpriest"}); !isErr || !strings.Contains(text, "cannot derive the digest path") {
		t.Errorf("update_digest outside a game folder = %q, error %v", text, isErr)
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

// The digest is derived from the SavedVariables path, written into the installed slot addon, and
// refused when the addon is not there.
func TestWriteDigest(t *testing.T) {
	game := t.TempDir()
	sv := filepath.Join(game, "WTF", "Account", "ACCT", "SavedVariables", "ForeverScribe.lua")
	os.MkdirAll(filepath.Dir(sv), 0o755)
	data, _ := os.ReadFile(fixture("tree_priest.lua"))
	os.WriteFile(sv, data, 0o644)
	cfg := Config{SavedVariables: sv}

	if _, err := WriteDigest(cfg, ""); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("without the addon: %v", err)
	}
	if _, err := os.Stat(filepath.Join(game, "Interface")); err == nil {
		t.Fatal("created the addon folder")
	}
	slot := filepath.Join(game, "Interface", "AddOns", "WoWCompanion_Digest")
	os.MkdirAll(slot, 0o755)
	out, err := WriteDigest(cfg, "testpriest")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(slot, "Digest.lua"))
	if out.Path != filepath.Join(slot, "Digest.lua") || !strings.Contains(out.Text, "/reload") ||
		!strings.Contains(string(got), `["Testpriest"] = {`) || !strings.Contains(string(got), `schema = "2.0"`) || out.Lines["Testpriest"]["notes"] == 0 {
		t.Errorf("out = %+v\n%s", out, got)
	}
	override := filepath.Join(t.TempDir(), "d.lua")
	if _, err := WriteDigest(Config{SavedVariables: fixture("tree_priest.lua"), DigestPath: override}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(override); err != nil {
		t.Errorf("--digest-path ignored: %v", err)
	}
}

// The digest holds every captured character whatever name is asked for, and an uncaptured name
// writes nothing and says what to do.
func TestWriteDigestEveryCharacter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Digest.lua")
	cfg := Config{SavedVariables: fixture("two_characters.lua"), DigestPath: path}

	_, err := WriteDigest(cfg, "Nobody")
	if want := "Nobody isn't in your SavedVariables yet. In game, /reload on that character so ForeverScribe saves it, then run this again."; err == nil || err.Error() != want {
		t.Errorf("unknown name: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("an unknown name wrote the file")
	}
	for _, name := range []string{"", "testbeta"} {
		out, err := WriteDigest(cfg, name)
		if err != nil || len(out.Characters) != 2 || len(out.Lines) != 2 || !strings.Contains(out.Text, "/reload") {
			t.Fatalf("WriteDigest(%q) = %+v, %v", name, out, err)
		}
	}
	got, _ := os.ReadFile(path)
	for _, k := range []string{`["Testalpha"] = {`, `["Testbeta"] = {`} {
		if !strings.Contains(string(got), k) {
			t.Errorf("no %s in\n%s", k, got)
		}
	}
}

// Missing data files get valid empty stubs; existing ones are left alone, with or without characters.
func TestEnsureDataFiles(t *testing.T) {
	game := t.TempDir()
	sv := filepath.Join(game, "WTF", "Account", "ACCT", "SavedVariables", "ForeverScribe.lua")
	cfg := Config{SavedVariables: sv}
	if err := EnsureDataFiles(cfg); err != nil {
		t.Fatalf("no addon folder: %v", err)
	}
	slot := filepath.Join(game, "Interface", "AddOns", "WoWCompanion_Digest")
	if _, err := os.Stat(slot); err == nil {
		t.Fatal("created the addon folder")
	}
	os.MkdirAll(slot, 0o755)
	if err := EnsureDataFiles(cfg); err != nil { // no SavedVariables file at all
		t.Fatal(err)
	}
	for file, want := range map[string]string{"Digest.lua": "WoWCompanionDigest = {", "Plan.lua": "WoWCompanionPlan = {"} {
		b, err := os.ReadFile(filepath.Join(slot, file))
		lua := string(b)
		if err != nil || !strings.HasPrefix(lua, "-- Data only") || !strings.Contains(lua, want) || !strings.Contains(lua, "characters = {") ||
			strings.Contains(lua, "function") || strings.Contains(lua, "os.") {
			t.Errorf("%s stub: %v\n%s", file, err, lua)
		}
	}
	plan := filepath.Join(slot, "Plan.lua")
	os.WriteFile(plan, []byte("-- mine\n"), 0o644)
	os.Remove(filepath.Join(slot, "Digest.lua"))
	if err := EnsureDataFiles(cfg); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(plan); string(b) != "-- mine\n" {
		t.Errorf("an existing Plan.lua was replaced: %q", b)
	}
	if _, err := os.Stat(filepath.Join(slot, "Digest.lua")); err != nil {
		t.Errorf("the missing Digest.lua was not written: %v", err)
	}
	// A refused digest (no characters captured) still leaves the stubs behind.
	os.Remove(plan)
	os.MkdirAll(filepath.Dir(sv), 0o755)
	os.WriteFile(sv, []byte("ForeverScribeDB = {}"), 0o644)
	if _, err := WriteDigest(cfg, ""); err == nil {
		t.Fatal("expected no characters")
	}
	if _, err := os.Stat(plan); err != nil {
		t.Errorf("Plan.lua not stubbed: %v", err)
	}
}

// Under SkipUnchanged a file is left alone only when the advice is the same and its generated time
// is not older than the newest capture; a newer capture or different advice rewrites it.
func TestWriteDigestSkipUnchanged(t *testing.T) {
	game := t.TempDir()
	sv := filepath.Join(game, "WTF", "Account", "ACCT", "SavedVariables", "ForeverScribe.lua")
	os.MkdirAll(filepath.Dir(sv), 0o755)
	data, _ := os.ReadFile(fixture("tree_priest.lua"))
	os.WriteFile(sv, data, 0o644)
	slot := filepath.Join(game, "Interface", "AddOns", "WoWCompanion_Digest")
	os.MkdirAll(slot, 0o755)
	cfg := Config{SavedVariables: sv, SkipUnchanged: true}
	gen := regexp.MustCompile(`generated = \d+,`)
	setGen := func(p, g string) string {
		b, _ := os.ReadFile(p)
		s := gen.ReplaceAllString(string(b), "generated = "+g+",")
		os.WriteFile(p, []byte(s), 0o644)
		return s
	}
	if _, err := WriteDigest(cfg, ""); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Digest.lua", "Plan.lua"} {
		p := filepath.Join(slot, name)
		// Same advice, no newer capture: untouched.
		want := setGen(p, "99999999999")
		WriteDigest(cfg, "")
		if got, _ := os.ReadFile(p); string(got) != want {
			t.Errorf("%s rewritten without a newer capture", name)
		}
		// Same advice, newer capture than generated: rewritten with a fresh time.
		setGen(p, "1")
		WriteDigest(cfg, "")
		if got, _ := os.ReadFile(p); strings.Contains(string(got), "generated = 1,") {
			t.Errorf("%s kept its old generated time", name)
		}
		// Changed advice: rewritten.
		b, _ := os.ReadFile(p)
		changed := strings.Replace(string(b), "generated = ", "extra = 1,\n  generated = ", 1)
		setGen(p, "99999999999")
		os.WriteFile(p, []byte(strings.Replace(changed, gen.FindString(changed), "generated = 99999999999,", 1)), 0o644)
		WriteDigest(cfg, "")
		if got, _ := os.ReadFile(p); strings.Contains(string(got), "extra = 1,") {
			t.Errorf("%s not rewritten for changed advice", name)
		}
	}
}
