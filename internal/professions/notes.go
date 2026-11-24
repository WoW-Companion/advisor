package professions

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
)

// Tier is one rank of a profession: the skill range it covers and the character level that
// opens it.
type Tier struct {
	Name     string
	SkillMin int
	SkillCap int
	MinLevel int
}

// Pairing is two professions that go well together, with the reason in one line.
type Pairing struct{ A, B, Text string }

// Feeder is a profession that supplies another.
type Feeder struct{ From, To, Text string }

// Notes are the curated profession notes (the dataset's profession_notes table): tiers with their
// level gates, pairings, feeders and special cases.
type Notes struct {
	Tiers     []Tier
	Pairings  []Pairing
	Feeders   []Feeder
	Special   map[string]string // profession -> note
	Secondary map[string]bool
}

// defaultTiers are used when the dataset carries no notes: the four early ranks and the character
// level each needs.
var defaultTiers = []Tier{
	{"Apprentice", 1, 75, 5}, {"Journeyman", 75, 150, 10}, {"Expert", 150, 225, 20}, {"Artisan", 225, 300, 35},
}

func asFloat(v any) float64 {
	f, _ := v.(float64)
	return f
}

func asI64(v any) int64 {
	n, _ := v.(int64)
	return n
}

// LoadNotes reads the curated notes; a dataset without them yields the default tiers and nothing else.
func LoadNotes(ds store.DataStore) Notes {
	n := Notes{Special: map[string]string{}, Secondary: map[string]bool{}}
	if ds != nil {
		rows, _ := ds.Filter("profession_notes", store.Range{Column: "section"})
		for _, r := range rows {
			subj, rel, text := asText(r["subject"]), asText(r["related"]), asText(r["text"])
			switch asText(r["section"]) {
			case "professions":
				n.Secondary[strings.ToLower(subj)] = rel == "secondary"
			case "ranks":
				if lo, hi, lv := asInt(r["skill_min"]), asInt(r["skill_cap"]), asInt(r["min_level"]); lo != nil && hi != nil && lv != nil {
					n.Tiers = append(n.Tiers, Tier{subj, *lo, *hi, *lv})
				}
			case "pairings":
				n.Pairings = append(n.Pairings, Pairing{subj, rel, text})
			case "feeders":
				n.Feeders = append(n.Feeders, Feeder{subj, rel, text})
			case "special_cases":
				n.Special[strings.ToLower(subj)] = text
			}
		}
	}
	if len(n.Tiers) == 0 {
		n.Tiers = defaultTiers
	}
	return n
}

// TierAt is the tier a skill lies in (the last one at or above the cap).
func (n Notes) TierAt(skill int) Tier {
	for _, t := range n.Tiers {
		if skill < t.SkillCap {
			return t
		}
	}
	return n.Tiers[len(n.Tiers)-1]
}

// CapAt is the highest skill cap a character of this level may train to, and its tier.
func (n Notes) CapAt(level int) (int, Tier) {
	best := n.Tiers[0]
	for _, t := range n.Tiers {
		if t.MinLevel <= level && t.SkillCap >= best.SkillCap {
			best = t
		}
	}
	return best.SkillCap, best
}

// NextTier is the tier after cap, when there is one.
func (n Notes) NextTier(cap int) (Tier, bool) {
	for _, t := range n.Tiers {
		if t.SkillMin >= cap {
			return t, true
		}
	}
	return Tier{}, false
}
