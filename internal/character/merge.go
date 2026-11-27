package character

import (
	"sort"
	"strings"
)

func sameCharacter(a, b Character) bool {
	if strings.EqualFold(a.Key, b.Key) {
		return true
	}
	return a.Name != "" && strings.EqualFold(a.Name, b.Name) && (a.Realm == "" || b.Realm == "" || strings.EqualFold(a.Realm, b.Realm))
}

func newer(a, b Character) bool { // a leads b: strictly newer capture, ties to a
	switch {
	case a.CapturedAt == nil:
		return b.CapturedAt == nil
	case b.CapturedAt == nil:
		return true
	}
	return *a.CapturedAt >= *b.CapturedAt
}

// MergeCharacters returns the newer capture, with each part it lacks filled from the other.
func MergeCharacters(a, b Character) Character {
	if !newer(a, b) {
		a, b = b, a
	}
	out := a
	if out.Level == nil {
		out.Level = b.Level
	}
	for _, p := range []struct {
		d *string
		s string
	}{{&out.Class, b.Class}, {&out.Race, b.Race}, {&out.Faction, b.Faction}, {&out.Hearth, b.Hearth}, {&out.Realm, b.Realm}, {&out.Addon, b.Addon}} {
		if *p.d == "" {
			*p.d = p.s
		}
	}
	for _, p := range []struct {
		d **int
		s *int
	}{{&out.Money, b.Money}, {&out.XP, b.XP}, {&out.XPMax, b.XPMax}, {&out.RestedXP, b.RestedXP}} {
		if *p.d == nil {
			*p.d = p.s
		}
	}
	if out.Client.Empty() {
		out.Client = b.Client
	}
	if out.Location == nil {
		out.Location = b.Location
	}
	if len(out.Equipment) == 0 {
		out.Equipment = b.Equipment
	}
	if !out.BagsCaptured && b.BagsCaptured {
		out.Bags, out.BagsCaptured = b.Bags, true
	}
	if !out.BankCaptured && b.BankCaptured {
		out.Bank, out.BankCaptured = b.Bank, true
	}
	if !out.QuestsCaptured && b.QuestsCaptured {
		out.Quests, out.QuestsCaptured = b.Quests, true
	}
	if len(out.CompletedQuests) == 0 {
		out.CompletedQuests = b.CompletedQuests
	}
	if out.PlanPrefs == nil {
		out.PlanPrefs = b.PlanPrefs
	}
	if out.Talents == nil {
		out.Talents = b.Talents
	}
	if !out.SkillsCaptured && b.SkillsCaptured {
		out.Skills, out.SkillsCaptured, out.Professions = b.Skills, true, b.Professions
	}
	if len(out.Reputations) == 0 {
		out.Reputations = b.Reputations
	}
	if out.NPCs == nil {
		out.NPCs = b.NPCs
	}
	if out.KnownSpells == nil {
		out.KnownSpells = b.KnownSpells
	}
	if out.Recipes == nil {
		out.Recipes = b.Recipes
	}
	if a.Source != b.Source && b.Source != "" && a.Source != "" {
		out.Source = SourceWoWC + " + " + SourceFS
	}
	return out.WithRecipeProfessions()
}

// mergeFiles combines the addon's file (a) with ForeverScribe's (b); a is empty when absent.
func mergeFiles(a, b File) File {
	if !a.HasWoWC {
		return b
	}
	out := a
	out.HasFS = b.HasFS
	if out.Addon == "" {
		out.Addon = b.Addon
	}
	if out.Schema == nil {
		out.Schema = b.Schema
	}
	if out.Client.Empty() {
		out.Client = b.Client
	}
	used := make([]bool, len(b.Characters))
	for i, c := range out.Characters {
		for j, o := range b.Characters {
			if !used[j] && sameCharacter(c, o) {
				used[j] = true
				m := MergeCharacters(c, o)
				m.Key = c.Key // the addon's key leads: it is the one the overlay uses
				out.Characters[i] = m
				break
			}
		}
	}
	for j, o := range b.Characters {
		if !used[j] {
			out.Characters = append(out.Characters, o)
		}
	}
	sort.SliceStable(out.Characters, func(i, j int) bool { return out.Characters[i].Key < out.Characters[j].Key })
	return out
}
