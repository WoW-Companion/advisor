package digest

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// MaxWhy is the longest "why" line, in characters (schema 1.6).
const MaxWhy = 70

// The class-quest line used when the dataset names no reward: it claims nothing it cannot back.
const classQuestWhy = "Class quest — these are worth doing first"

var rawID = regexp.MustCompile(`(?i)\b(?:quest|npc|item|spell|id)[ :#=]*\d{2,}|\d{5,}`)

// whyOf returns s when it is a usable "why": at most MaxWhy characters, whole (never cut, never
// "..."), and free of ids. Otherwise it returns "", and the entry carries no why.
func whyOf(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > MaxWhy || strings.Contains(s, "...") || strings.Contains(s, "…") || rawID.MatchString(s) {
		return ""
	}
	return s
}

// firstWhy is the first candidate that is a usable why.
func firstWhy(cands ...string) string {
	for _, s := range cands {
		if w := whyOf(s); w != "" {
			return w
		}
	}
	return ""
}

// questWhy is the why of a quest line, built only from what the line already holds: its kind, its
// unlocks, its open objectives and its level fit. The dataset names no quest rewards, so none is claimed.
func questWhy(c character.Character, l Line) string {
	fit := ""
	if c.Level != nil {
		fit = wording.Fit(l.Level, *c.Level)
	}
	switch l.Kind {
	case KindTurnIn:
		if len(l.Unlocks) > 0 {
			next, _, _ := strings.Cut(l.Unlocks[0], " (")
			return firstWhy("Unlocks "+next, "Unlocks a follow-up quest", "Done — hand it in on your way")
		}
		return "Done — hand it in on your way"
	case KindProgress:
		for _, o := range l.Objectives {
			if o.HasCount && o.Have >= o.Need {
				continue
			}
			t := o.Text
			if o.HasCount {
				return firstWhy(fmt.Sprintf("Still to do: %s (%d/%d)", t, o.Have, o.Need), "Still to do: "+t)
			}
			return firstWhy("Still to do: " + t)
		}
		return ""
	case KindPickup:
		if l.Class {
			return classQuestWhy
		}
		return firstWhy(fit)
	case KindGiver:
		return firstWhy(fit)
	}
	return ""
}

// talentWhy is the why of the unspent-points line: the plan's next pick, or how to get one.
func talentWhy(c character.Character, pick string) string {
	if pick != "" {
		return firstWhy("Your plan's next pick: "+pick, "Your plan has a pick for this point")
	}
	if style(c) == "" {
		return "Choose a play style to get a pick"
	}
	return ""
}

// trainingWhy is "New: <ability>" for the first newly learned ability.
func trainingWhy(spells []TrainSpell) string {
	for _, s := range spells {
		if s.New && s.Name != "" {
			return firstWhy("New: " + s.Name)
		}
	}
	return ""
}

// upgradeWhy is the stat or slot gain of an upgrade: the item levels gained, or an empty slot filled.
func upgradeWhy(c character.Character, slot string, cd gear.Candidate) string {
	name := wording.Slot(slot)
	if cd.Empty {
		return firstWhy("Fills your empty " + name + " slot")
	}
	if cur, ok := c.Equipment[slot]; ok && cur.ItemLevel != nil {
		if gain := int(cd.Item.ItemLevel) - *cur.ItemLevel; gain > 0 {
			return firstWhy(fmt.Sprintf("+%d item %s for your %s slot", gain, wording.Plural(gain, "level", "levels"), name))
		}
	}
	return ""
}
