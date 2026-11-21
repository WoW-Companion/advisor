// Package eval is the fixed evaluation question set: for each synthetic character, a list of advice
// questions, each asked of the real tool function and judged by a named property (checks_test.go).
// docs/eval.md explains how to add one.
package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Tools a question can be mapped to; they are the names the MCP server exposes.
const (
	toolWhatsNext = "whats_next"
	toolBrief     = "character_brief"
	toolUpgrades  = "upgrade_finder"
	toolDigest    = "update_digest"
)

type question struct {
	Ask   string `json:"ask"`   // the question as a player would put it
	Tool  string `json:"tool"`  // the tool it is mapped to
	Check string `json:"check"` // the property that must hold, by name in checks
}

type questionSet struct {
	Characters map[string][]question `json:"characters"` // by capture file name, without .lua
}

// env is what a check is given: the character, the dataset and the capture's client, and a clock
// fixed ten minutes after the capture.
type env struct {
	c      character.Character
	ds     store.DataStore
	client character.Client
	now    time.Time
}

func loadCharacter(t *testing.T, file string) (character.Character, character.Client) {
	t.Helper()
	f, err := character.LoadFile(filepath.Join("captures", file+".lua"))
	if err != nil {
		t.Fatalf("capture %s: %v", file, err)
	}
	if len(f.Characters) != 1 {
		t.Fatalf("capture %s holds %d characters, want 1", file, len(f.Characters))
	}
	return f.Characters[0], f.ClientOf(f.Characters[0])
}

// TestQuestions asks every question of the real tools. A failure names the character, the
// question and the property that broke.
func TestQuestions(t *testing.T) {
	raw, err := os.ReadFile("questions.json")
	if err != nil {
		t.Fatal(err)
	}
	var set questionSet
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatalf("questions.json: %v", err)
	}
	ds := store.NewDir("dataset")
	files := make([]string, 0, len(set.Characters))
	for f := range set.Characters {
		files = append(files, f)
	}
	sort.Strings(files)
	total := 0
	for _, file := range files {
		c, client := loadCharacter(t, file)
		e := &env{c: c, ds: ds, client: client, now: time.Unix(capturedAt+600, 0)}
		for i, q := range set.Characters[file] {
			total++
			ck, ok := checks[q.Check]
			if !ok {
				t.Errorf("%s question %d %q: no property named %q", file, i+1, q.Ask, q.Check)
				continue
			}
			if ck.tool != q.Tool {
				t.Errorf("%s question %d %q: property %q belongs to %s, not %s", file, i+1, q.Ask, q.Check, ck.tool, q.Tool)
				continue
			}
			if err := ck.run(e); err != nil {
				t.Errorf("%s question %d %q [%s] broke property %q: %v", file, i+1, q.Ask, q.Tool, q.Check, err)
			}
		}
	}
	t.Logf("%d questions over %d characters", total, len(files))
}

// TestSetShape guards the size of the set: six characters (Priest and Rogue at levels 12, 24 and
// 40), 8 to 12 questions each, 48 or more in all, every property used and no question twice.
func TestSetShape(t *testing.T) {
	raw, err := os.ReadFile("questions.json")
	if err != nil {
		t.Fatal(err)
	}
	var set questionSet
	if err := json.Unmarshal(raw, &set); err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"priest12": true, "priest24": true, "priest40": true, "rogue12": true, "rogue24": true, "rogue40": true}
	total := 0
	used := map[string]bool{}
	for file, qs := range set.Characters {
		if !want[file] {
			t.Errorf("unexpected character %q", file)
		}
		delete(want, file)
		if len(qs) < 8 || len(qs) > 12 {
			t.Errorf("%s has %d questions, want 8 to 12", file, len(qs))
		}
		seen := map[string]bool{}
		for _, q := range qs {
			if seen[q.Check] {
				t.Errorf("%s asks property %q twice", file, q.Check)
			}
			seen[q.Check], used[q.Check] = true, true
		}
		total += len(qs)
	}
	for f := range want {
		t.Errorf("no questions for %s", f)
	}
	if total < 48 {
		t.Errorf("%d questions, want at least 48", total)
	}
	for name := range checks {
		if !used[name] {
			t.Errorf("property %q is defined but no question uses it", name)
		}
	}
}
