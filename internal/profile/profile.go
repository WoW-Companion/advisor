// Package profile keeps each character's play style on the player's own machine
// (the user config directory, never a repo).
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/WoW-Companion/advisor/internal/character"
)

// Env overrides the store's path.
const Env = "WOWC_PROFILES"

// Allowed values, first is listed first in the question the model asks.
var (
	Roles = []string{"damage", "tank", "healer", "balanced"}
	Foci  = []string{"leveling", "dungeons", "pvp", "mixed"}
	// StatPrefs are the optional stat preferences ("" is balanced): see the stats package.
	StatPrefs = []string{"", "balanced", "damage", "survival"}
)

// Limits on what is stored.
const (
	MaxField = 120
)

// Path is the store file: $WOWC_PROFILES, else <user config dir>/wowc-advisor/profiles.json; empty
// when the system has no config directory.
func Path() string {
	if p := os.Getenv(Env); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "wowc-advisor", "profiles.json")
}

// Entry is everything stored for one character.
type Entry struct {
	Profile *character.Profile `json:"profile,omitempty"`
}

// Schema is the version of the file's layout. Changes are additive only: a field is added, never
// renamed, retyped or removed, so an older release reads a newer file by ignoring what it does not
// know (see docs/releasing.md).
const Schema = 1

type file struct {
	Schema     int              `json:"schema,omitempty"`
	Characters map[string]Entry `json:"characters"`
}

var mu sync.Mutex // one writer at a time inside this process

// Get reads one character's entry; the zero Entry when there is none or the file is unreadable.
func Get(path, key string) Entry {
	e, _ := read(path)
	return e.Characters[key]
}

func read(path string) (file, error) {
	f := file{Characters: map[string]Entry{}}
	if path == "" {
		return f, errors.New("this system has no user config directory")
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return f, nil
	}
	if err != nil {
		return f, err
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return file{Characters: map[string]Entry{}}, fmt.Errorf("%s: %w", path, err)
	}
	if f.Characters == nil {
		f.Characters = map[string]Entry{}
	}
	return f, nil
}

// Update reads the character's entry, lets fn change it and writes the file back atomically
// (mode 0600). A file that cannot be parsed is an error, never overwritten.
func Update(path, key string, fn func(*Entry) error) (Entry, error) {
	mu.Lock()
	defer mu.Unlock()
	f, err := read(path)
	if err != nil {
		return Entry{}, err
	}
	e := f.Characters[key]
	if err := fn(&e); err != nil {
		return Entry{}, err
	}
	if e.Profile == nil {
		delete(f.Characters, key) // nothing left to keep: the character has no entry
	} else {
		f.Characters[key] = e
	}
	f.Schema = max(f.Schema, Schema)
	out, _ := json.MarshalIndent(f, "", "  ")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Entry{}, err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0o600); err != nil {
		return Entry{}, err
	}
	return e, os.Rename(tmp, path)
}

// Attach returns c with its stored profile and plan; c is unchanged when path holds neither.
func Attach(path string, c character.Character) character.Character {
	e := Get(path, c.Key)
	c.Profile = e.Profile
	return c
}

// Clean checks a profile: role and focus are empty ("Not set") or from the allowed lists
// (case-insensitive, lower-cased on save) and the spec is short text. An empty role or focus is
// asked for again when a question needs it.
func Clean(p character.Profile) (character.Profile, error) {
	p.Role, p.Focus = strings.ToLower(strings.TrimSpace(p.Role)), strings.ToLower(strings.TrimSpace(p.Focus))
	p.Spec = strings.TrimSpace(p.Spec)
	p.StatPref = strings.ToLower(strings.TrimSpace(p.StatPref))
	switch {
	case p.Role != "" && !slices.Contains(Roles, p.Role):
		return p, fmt.Errorf("role must be one of %s", strings.Join(Roles, ", "))
	case p.Focus != "" && !slices.Contains(Foci, p.Focus):
		return p, fmt.Errorf("focus must be one of %s", strings.Join(Foci, ", "))
	case !slices.Contains(StatPrefs, p.StatPref):
		return p, fmt.Errorf("stat_pref must be one of %s", strings.Join(StatPrefs[1:], ", "))
	case len(p.Spec) > MaxField:
		return p, errors.New("spec is too long")
	}
	return p, nil
}

// Incomplete is true when a saved play style lacks its role or its focus, so a question that needs
// them must ask. A nil profile is incomplete too.
func Incomplete(p *character.Profile) bool {
	return p == nil || p.Role == "" || p.Focus == ""
}

// Ask is the short multiple-choice question the model puts to a player who has no saved play
// style, before it gives any talent advice. The answer is saved with set_character_profile.
func Ask() string {
	return "No play style is saved for this character yet. Ask the player once, as a short multiple-choice question, then call set_character_profile with the answer:\n" +
		"1. What do you want this character to do? " + strings.Join(Roles, " / ") + "\n" +
		"2. What are you mostly doing? " + strings.Join(Foci, " / ") + "\n" +
		"3. Which spec tree (optional)."
}
