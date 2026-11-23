package ask

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/profile"
)

const priestCapture = "../../testdata/foreverscribe/tree_priest.lua"

func priestKey(t *testing.T) string {
	t.Helper()
	f, err := character.LoadFile(priestCapture)
	if err != nil || len(f.Characters) == 0 {
		t.Fatal(err)
	}
	return f.Characters[0].Key
}

func TestPlayStyleNeededReturnsChoicesWithoutTheAgent(t *testing.T) {
	cfg := Config{SavedVariables: priestCapture} // no agent command: any AI call would fail
	key := priestKey(t)
	q := Question{Character: key, Text: "Where should my talent point go?", Ref: "talents"}
	n, ok := PlayStyleNeeded(cfg, q)
	if !ok || n.Needs != "play_style" || n.Question != q.Text || n.Ref != "talents" {
		t.Fatalf("needs = %+v, %v", n, ok)
	}
	if len(n.Roles) != 4 || n.Roles[0].Value != "damage" || len(n.Activities) != 4 || n.Activities[2].Label != "PvP" || len(n.Specs) == 0 {
		t.Errorf("options = %+v", n)
	}
	if _, ok := PlayStyleNeeded(cfg, Question{Character: key, Text: "What should I do next?"}); ok {
		t.Error("a quest question asked for a play style")
	}
}

func TestPlayStyleSavedMeansNoChoices(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profiles.json")
	t.Setenv(profile.Env, path)
	cfg := Config{SavedVariables: priestCapture}
	key := priestKey(t)
	q := Question{Character: key, Text: "Which talents should I take?"}
	if _, ok := PlayStyleNeeded(cfg, q); !ok {
		t.Fatal("no choices before a save")
	}
	if _, err := profile.Update(path, key, func(e *profile.Entry) error {
		e.Profile = &character.Profile{Role: "healer", Focus: "dungeons"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := PlayStyleNeeded(cfg, q); ok {
		t.Error("choices offered after the play style was saved")
	}
}

func TestRefExpandsTheSubjectAndExactAsksGetIt(t *testing.T) {
	cfg := Config{SavedVariables: syntheticCapture}
	key := "Testrealm:Testmage"
	typed := Question{Character: key, Text: "how do i download the GAME data"}
	if got := ResolveRef(cfg, typed); got != "note:no-data" {
		t.Fatalf("typed ask ref = %q", got)
	}
	if got := ResolveRef(cfg, Question{Character: key, Text: "How do I download the game data and then more?"}); got != "" {
		t.Errorf("a different question got ref %q", got)
	}
	if got := ResolveRef(cfg, Question{Character: key, Text: "x", Ref: "quest:5"}); got != "quest:5" {
		t.Errorf("a given ref was replaced: %q", got)
	}
	p := cfg.Prompt(Question{Character: key, Text: "How do I download the game data?", Ref: "note:no-data"})
	if !strings.Contains(p, "Subject: Game data not downloaded yet") || !strings.Contains(p, "Place in the plan") {
		t.Errorf("subject not expanded: %s", p)
	}
	if p := cfg.Prompt(Question{Character: key, Text: "What now?"}); strings.Contains(p, "Subject:") {
		t.Error("a question without a ref got a subject")
	}
}

func TestStatQuestionNeedsAPlayStyleWithTheHook(t *testing.T) {
	cfg := Config{SavedVariables: priestCapture}
	key := priestKey(t)
	n, ok := PlayStyleNeeded(cfg, Question{Character: key, Text: "Do I have enough Stamina?"})
	if !ok || n.Needs != "play_style" || n.Hook != "Choose a play style for a stat check" {
		t.Errorf("needs = %+v, %v", n, ok)
	}
	if n, _ := PlayStyleNeeded(cfg, Question{Character: key, Text: "Which talents should I take?"}); n.Hook != "" {
		t.Errorf("hook on a talent question: %q", n.Hook)
	}
}
