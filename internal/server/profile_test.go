package server

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func profileSession(t *testing.T, store string) func(string, map[string]any) (string, bool) {
	t.Helper()
	ctx := context.Background()
	st, ct := mcp.NewInMemoryTransports()
	ss, err := New(Config{SavedVariables: fixture("tree_priest.lua"), Profiles: store}).Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return func(name string, args map[string]any) (string, bool) {
		res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		return res.Content[0].(*mcp.TextContent).Text, res.IsError
	}
}

func TestProfileFlow(t *testing.T) {
	store := filepath.Join(t.TempDir(), "profiles.json")
	call := profileSession(t, store)

	// No profile: the first thing the model gets is the multiple-choice question.
	text, isErr := call("get_character_profile", map[string]any{"character": "testpriest"})
	var out ProfileOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil || isErr {
		t.Fatalf("get = %s (%v)", text, err)
	}
	if !out.NeedsProfile || !strings.HasPrefix(out.Text, "No play style is saved") || !strings.Contains(out.Text, "damage / tank / healer / balanced") {
		t.Errorf("no profile did not lead to the question: %+v", out)
	}
	if out.Overview != "https://www.icy-veins.com/wow-forever/priest-class-overview" {
		t.Errorf("overview = %q", out.Overview)
	}
	// An incomplete or unknown answer saves nothing.
	for _, args := range []map[string]any{{"role": "damage"}, {"role": "dps", "focus": "pvp"}} {
		args["character"] = "testpriest"
		if text, isErr := call("set_character_profile", args); !isErr || !strings.HasPrefix(text, "Not saved:") {
			t.Errorf("set %v = %q, error %v", args, text, isErr)
		}
	}
	if _, err := os.Stat(store); err == nil {
		t.Error("a refused answer created the store")
	}

	// The answer, with a plan that names one real node and one invented one.
	text, isErr = call("set_character_profile", map[string]any{"character": "testpriest", "role": "Healer", "focus": "dungeons", "spec": "Test Discipline",
		"plan": map[string]any{"spec": "Test Discipline", "guide_url": "https://www.icy-veins.com/wow-forever/discipline-priest-healing-pve-guide",
			"picks": []map[string]any{{"node": "test inner focus", "rank": 5}, {"node": "Invented Talent", "rank": 1}}}})
	if isErr || json.Unmarshal([]byte(text), &out) != nil {
		t.Fatalf("set = %s", text)
	}
	if out.NeedsProfile || out.Profile.Role != "healer" || len(out.Dropped) != 1 || !strings.Contains(out.Dropped[0], "Invented Talent") || len(out.Plan.Picks) != 1 || out.Plan.Picks[0].Node != "Test Inner Focus" {
		t.Errorf("set result: %+v", out)
	}

	// It is remembered by a fresh session, and a partial update keeps the rest.
	call = profileSession(t, store)
	call("set_character_profile", map[string]any{"character": "testpriest", "focus": "pvp"})
	text, _ = call("get_character_profile", map[string]any{"character": "testpriest"})
	json.Unmarshal([]byte(text), &out)
	if out.Profile == nil || out.Profile.Role != "healer" || out.Profile.Focus != "pvp" || out.Plan == nil {
		t.Errorf("remembered profile: %+v", out)
	}
	if !strings.Contains(out.Text, "Every node of the plan is at its target rank") {
		t.Errorf("text = %s", out.Text)
	}

	// A spec change makes the plan stale: no advice from the old build, and the model is told to refresh.
	call("set_character_profile", map[string]any{"character": "testpriest", "spec": "Test Holy"})
	text, _ = call("get_character_profile", map[string]any{"character": "testpriest"})
	json.Unmarshal([]byte(text), &out)
	if !strings.Contains(out.Text, "refresh the talent plan") {
		t.Errorf("stale plan not flagged: %s", out.Text)
	}

	// talent_tree gives the exact names a plan may use.
	if text, isErr := call("talent_tree", map[string]any{"character": "testpriest"}); isErr || !strings.Contains(text, "Test Inner Focus") {
		t.Errorf("talent_tree = %s", text)
	}
}
