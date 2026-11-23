// Package profile keeps each character's play style and talent plan on the player's own machine
// (the user config directory, never a repo), and checks a plan against the captured talent tree.
//
// A plan holds node names and target ranks only. Guide text is never stored here.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
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
	MaxPicks  = 120
	MaxField  = 120
	MaxURLLen = 300
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
	Profile *character.Profile    `json:"profile,omitempty"`
	Plan    *character.TalentPlan `json:"plan,omitempty"`
}

type file struct {
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
	f.Characters[key] = e
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
	c.Profile, c.Plan = e.Profile, e.Plan
	return c
}

// Clean checks a profile: role and focus are from the allowed lists (case-insensitive, lower-cased
// on save), the spec is short text and the guide is an https URL.
func Clean(p character.Profile) (character.Profile, error) {
	p.Role, p.Focus = strings.ToLower(strings.TrimSpace(p.Role)), strings.ToLower(strings.TrimSpace(p.Focus))
	p.Spec, p.GuideURL = strings.TrimSpace(p.Spec), strings.TrimSpace(p.GuideURL)
	p.StatPref = strings.ToLower(strings.TrimSpace(p.StatPref))
	switch {
	case !slices.Contains(Roles, p.Role):
		return p, fmt.Errorf("role must be one of %s", strings.Join(Roles, ", "))
	case !slices.Contains(Foci, p.Focus):
		return p, fmt.Errorf("focus must be one of %s", strings.Join(Foci, ", "))
	case !slices.Contains(StatPrefs, p.StatPref):
		return p, fmt.Errorf("stat_pref must be one of %s", strings.Join(StatPrefs[1:], ", "))
	case len(p.Spec) > MaxField:
		return p, errors.New("spec is too long")
	}
	if p.GuideURL != "" {
		u, err := url.Parse(p.GuideURL)
		if err != nil || u.Scheme != "https" || u.Host == "" || len(p.GuideURL) > MaxURLLen {
			return p, errors.New("guide_url must be an https URL")
		}
	}
	return p, nil
}

// Check is the result of validating a plan against the captured tree.
type Check struct {
	Plan    character.TalentPlan
	Dropped []string // entries not in the captured tree, or with a bad rank, with the reason
}

// ValidatePlan keeps the picks that name a node of the captured class tree (matched by name,
// case-insensitively, and written back with the captured spelling) and a rank from 1 to its max
// rank; the rest are dropped and listed. A repeated node keeps its first place and its highest
// rank. An error means nothing usable is left.
func ValidatePlan(c character.Character, p character.TalentPlan) (Check, error) {
	out := Check{Plan: character.TalentPlan{Spec: strings.TrimSpace(p.Spec), GuideURL: strings.TrimSpace(p.GuideURL), Date: p.Date}}
	if u, err := url.Parse(out.Plan.GuideURL); err != nil || u.Scheme != "https" || u.Host == "" || len(out.Plan.GuideURL) > MaxURLLen {
		return out, errors.New("the plan needs the guide's https URL")
	}
	if out.Plan.Spec == "" || len(out.Plan.Spec) > MaxField {
		return out, errors.New("the plan needs the spec name")
	}
	if len(p.Picks) == 0 || len(p.Picks) > MaxPicks {
		return out, fmt.Errorf("the plan needs 1 to %d picks", MaxPicks)
	}
	known := map[string]character.Talent{}
	if c.Talents != nil {
		for _, t := range c.Talents.Trees {
			if t.IsProfession() {
				continue
			}
			for _, tl := range t.Talents {
				known[strings.ToLower(tl.Name)] = tl
			}
		}
	}
	at := map[string]int{} // node -> index in out.Plan.Picks
	for _, pk := range p.Picks {
		tl, ok := known[strings.ToLower(strings.TrimSpace(pk.Node))]
		switch {
		case !ok:
			out.Dropped = append(out.Dropped, fmt.Sprintf("%q is not in the captured talent tree", pk.Node))
			continue
		case pk.Rank < 1 || tl.MaxRank != nil && pk.Rank > *tl.MaxRank:
			out.Dropped = append(out.Dropped, fmt.Sprintf("%s: rank %d is outside its range", tl.Name, pk.Rank))
			continue
		}
		if i, dup := at[tl.Name]; dup {
			out.Plan.Picks[i].Rank = max(out.Plan.Picks[i].Rank, pk.Rank)
			continue
		}
		at[tl.Name] = len(out.Plan.Picks)
		out.Plan.Picks = append(out.Plan.Picks, character.PlanPick{Node: tl.Name, Rank: pk.Rank})
	}
	if len(out.Plan.Picks) == 0 {
		return out, errors.New("none of the plan's nodes is in the captured talent tree")
	}
	return out, nil
}

// Ask is the short multiple-choice question the model puts to a player who has no saved play
// style, before it gives any talent advice. The answer is saved with set_character_profile.
func Ask() string {
	return "No play style is saved for this character yet. Ask the player once, as a short multiple-choice question, then call set_character_profile with the answer:\n" +
		"1. What do you want this character to do? " + strings.Join(Roles, " / ") + "\n" +
		"2. What are you mostly doing? " + strings.Join(Foci, " / ") + "\n" +
		"3. Which spec tree (optional), and a guide URL if you have one (optional; the default is the Icy Veins WoW: Forever guide)."
}

// Overview is the Icy Veins WoW: Forever class overview page, where the guide for a spec is found.
func Overview(class string) string {
	slug := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(class)), " ", "-")
	if slug == "" {
		return ""
	}
	return "https://www.icy-veins.com/wow-forever/" + slug + "-class-overview"
}
