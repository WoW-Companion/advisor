// Package compat guards one rule: what this release writes to disk, the previous release can still
// read. The "previous reader" structs below are frozen copies of the 0.7.54 readers; they must never
// gain a field, and the fixtures in testdata/compat are what the current writers produce. When a
// writer changes (additively), regenerate the fixtures with UPDATE_COMPAT=1 and say so in the review.
package compat

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/provider"
)

// The 0.7.54 readers, frozen: no schema field, only what that release knew.
type prevProfileFile struct {
	Characters map[string]struct {
		Profile *character.Profile `json:"profile,omitempty"`
	} `json:"characters"`
}

type prevProviders struct {
	Clients []struct {
		Kind     string `json:"kind"`
		Provider string `json:"provider,omitempty"`
		BaseURL  string `json:"base_url,omitempty"`
		Model    string `json:"model,omitempty"`
		Tier     string `json:"tier,omitempty"`
		Agent    string `json:"agent,omitempty"`
	} `json:"clients"`
}

// current writes the files with this release's writers into a fresh folder.
func current(t *testing.T) (profiles, providers []byte) {
	t.Helper()
	dir := t.TempDir()
	pp := filepath.Join(dir, "profiles.json")
	if _, err := profile.Update(pp, "Thrall-Doomhammer", func(e *profile.Entry) error {
		e.Profile = &character.Profile{Role: "damage", Focus: "leveling", Spec: "Enhancement"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rp := filepath.Join(dir, "providers.json")
	err := provider.Save(rp, provider.Settings{Clients: []provider.ClientRef{{Kind: "api", Provider: "anthropic", Tier: "balanced"}, {Kind: "cli", Agent: "claude"}}})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := os.ReadFile(pp)
	b, _ := os.ReadFile(rp)
	return a, b
}

func TestFixturesMatchWhatTheCurrentWritersProduce(t *testing.T) {
	profiles, providers := current(t)
	for name, got := range map[string][]byte{"profiles.json": profiles, "providers.json": providers} {
		path := filepath.Join("..", "..", "testdata", "compat", name)
		if os.Getenv("UPDATE_COMPAT") != "" {
			if err := os.WriteFile(path, got, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(got) {
			t.Errorf("%s changed: the writer's output no longer matches testdata/compat/%s.\nIf the change is additive, regenerate with UPDATE_COMPAT=1; otherwise older releases may not read it.\ngot:\n%s", name, name, got)
		}
	}
}

func TestPreviousReleaseReadsCurrentFiles(t *testing.T) {
	read := func(name string, into any) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "compat", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(b, into); err != nil {
			t.Fatalf("the previous release cannot read %s: %v", name, err)
		}
	}
	var pf prevProfileFile
	read("profiles.json", &pf)
	p := pf.Characters["Thrall-Doomhammer"].Profile
	if p == nil || p.Role != "damage" || p.Focus != "leveling" || p.Spec != "Enhancement" {
		t.Errorf("profile read by the previous release: %+v", p)
	}
	var pv prevProviders
	read("providers.json", &pv)
	if len(pv.Clients) != 2 || pv.Clients[0].Provider != "anthropic" || pv.Clients[1].Agent != "claude" {
		t.Errorf("providers read by the previous release: %+v", pv)
	}
}

func TestFilesCarryASchemaNumber(t *testing.T) {
	profiles, providers := current(t)
	for name, b := range map[string][]byte{"profiles": profiles, "providers": providers} {
		var m struct {
			Schema int `json:"schema"`
		}
		if err := json.Unmarshal(b, &m); err != nil || m.Schema < 1 {
			t.Errorf("%s has no schema number (%v)", name, err)
		}
	}
}

// A newer release's file, with a field this one has never heard of, still loads.
func TestUnknownFieldsAreIgnored(t *testing.T) {
	dir := t.TempDir()
	pp := filepath.Join(dir, "profiles.json")
	os.WriteFile(pp, []byte(`{"schema":9,"future":{"x":1},"characters":{"A-B":{"profile":{"role":"tank","focus":"pvp","new_thing":true},"plan2":[1]}}}`), 0o600)
	if e := profile.Get(pp, "A-B"); e.Profile == nil || e.Profile.Role != "tank" {
		t.Errorf("a newer profiles file did not load: %+v", e)
	}
	rp := filepath.Join(dir, "providers.json")
	os.WriteFile(rp, []byte(`{"schema":9,"extra":[],"clients":[{"kind":"cli","agent":"codex","wow":1}]}`), 0o600)
	s, err := provider.Load(rp)
	if err != nil || len(s.Clients) != 1 || s.Clients[0].Agent != "codex" {
		t.Errorf("a newer providers file did not load: %+v %v", s, err)
	}
}
