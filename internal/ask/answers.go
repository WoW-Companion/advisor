package ask

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/lua"
)

// AnswersLua renders the answers as the one data-only `WoWCompanionAnswers = {...}` assignment.
func AnswersLua(answers map[string]Answer) string {
	ids := make([]string, 0, len(answers))
	for id := range answers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var b strings.Builder
	b.WriteString("-- Data only: written by wowc-advisor; overwritten on every pass. See docs/ask-format.md in the addon repo.\n")
	fmt.Fprintf(&b, "WoWCompanionAnswers = {\n  schema = %d,\n  answers = {\n", Schema)
	for _, id := range ids {
		a := answers[id]
		fmt.Fprintf(&b, "    [%s] = {\n      answered_at = %d,\n      text = %s,\n", digest.Quote(id), a.AnsweredAt, digest.Quote(a.Text))
		if len(a.Sources) > 0 {
			b.WriteString("      sources = { ")
			for _, s := range a.Sources {
				b.WriteString(digest.Quote(s) + ", ")
			}
			b.WriteString("},\n")
		}
		b.WriteString("    },\n")
	}
	b.WriteString("  },\n}\n")
	return b.String()
}

// CheckAnswers is the slot rule: the text must be exactly one `WoWCompanionAnswers = {...}`
// assignment of literals (the safe reader refuses anything else), schema 1, and every answer must
// have answered_at and text, with sources, when present, a list of strings.
func CheckAnswers(text string) error {
	vars, err := lua.Parse(text)
	if err != nil {
		return err
	}
	if len(vars) != 1 {
		return errors.New("the file must hold exactly one assignment, WoWCompanionAnswers")
	}
	root, ok := vars["WoWCompanionAnswers"].(lua.Table)
	if !ok {
		return errors.New("WoWCompanionAnswers must be a table")
	}
	if v, _ := root["schema"].(int64); v != Schema {
		return fmt.Errorf("schema must be %d", Schema)
	}
	answers, ok := root["answers"].(lua.Table)
	if !ok {
		return errors.New("answers must be a table")
	}
	for k, v := range answers {
		id, ok := k.(string)
		if !ok {
			return fmt.Errorf("answer key %v is not a string id", k)
		}
		a, ok := v.(lua.Table)
		if !ok {
			return fmt.Errorf("answer %s is not a table", id)
		}
		if _, ok := a["answered_at"].(int64); !ok {
			return fmt.Errorf("answer %s has no answered_at number", id)
		}
		if t, ok := a["text"].(string); !ok || t == "" {
			return fmt.Errorf("answer %s has no text", id)
		}
		if s, present := a["sources"]; present {
			for _, e := range lua.List(s) {
				if _, ok := e.(string); !ok {
					return fmt.Errorf("answer %s: sources must be strings", id)
				}
			}
		}
	}
	return nil
}

// ReadAnswers returns the answers a valid Answers.lua holds; a missing or invalid file holds none.
func ReadAnswers(path string) map[string]Answer {
	out := map[string]Answer{}
	raw, err := os.ReadFile(path)
	if err != nil || CheckAnswers(string(raw)) != nil {
		return out
	}
	vars, _ := lua.Parse(string(raw))
	root, _ := vars["WoWCompanionAnswers"].(lua.Table)
	answers, _ := root["answers"].(lua.Table)
	for k, v := range answers {
		t := v.(lua.Table)
		a := Answer{Text: t["text"].(string)}
		a.AnsweredAt, _ = t["answered_at"].(int64)
		for _, s := range lua.List(t["sources"]) {
			a.Sources = append(a.Sources, s.(string))
		}
		out[k.(string)] = a
	}
	return out
}

// WriteAnswers checks the rendered file against the slot rule, then replaces path atomically: a temp
// file in the same folder, then a rename. The folder must exist.
func WriteAnswers(path string, answers map[string]Answer) error {
	text := AnswersLua(answers)
	if err := CheckAnswers(text); err != nil {
		return fmt.Errorf("refusing to write an invalid Answers.lua: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".Answers-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(text)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(tmp.Name(), 0o644)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Paths derives the addon's SavedVariables file and Answers.lua from the ForeverScribe
// SavedVariables path, the way the digest path is derived: the same SavedVariables folder holds
// WoWCompanion.lua, and the answers go to <game>/Interface/AddOns/WoWCompanion_Answers/Answers.lua.
func Paths(foreverScribe string) (savedVariables, answers string, err error) {
	answers, ok := digest.SlotPath(foreverScribe, SlotAddon, "Answers.lua")
	if !ok {
		return "", "", fmt.Errorf("cannot derive the answers path from %q: expected <game>/WTF/Account/<account>/SavedVariables/<file>; use --answers-path and --addon-savedvariables", foreverScribe)
	}
	cut := strings.LastIndexAny(foreverScribe, `/\`)
	return foreverScribe[:cut+1] + "WoWCompanion.lua", answers, nil
}

// CheckInstalled is an error when the slot addon's folder is not there. Nothing is created.
func CheckInstalled(answersPath string) error {
	dir := filepath.Dir(answersPath)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("the %s addon is not installed (no folder %s); install it from the WoW Companion addon repo", SlotAddon, dir)
	}
	return nil
}
