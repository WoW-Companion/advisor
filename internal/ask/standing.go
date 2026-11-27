package ask

import (
	"regexp"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// standingWords make a question one about reputation: a faction's standing, what a standing unlocks, or how to earn it.
var standingWords = regexp.MustCompile(`(?i)\b(reputation|rep|standing|friendly|honou?red|revered|exalted|unfriendly|hostile|hated|grind(?:ing)? rep)\b`)

// NoStandingGrindFacts is said for a reputation question: the game data holds no faction, no standing a vendor or an item asks for and
// no reputation grind route, so the advisor names none.
const NoStandingGrindFacts = "Reputation: the game data holds no faction reputation, no standing that an item, vendor or recipe asks for, and no reputation grind route. " +
	"Say so for any such part of the question; never name a standing an item needs, a number of points a quest or kill gives, or a grind route from memory."

// StandingFacts is the reputation section of the facts for a reputation question: the player's own standing and points to the next
// standing for every faction the question names (or for every captured faction when it names none, up to maxStandings), and what the
// game data does not hold. "" for a question that is not about reputation.
func StandingFacts(c character.Character, text string) string {
	if !standingWords.MatchString(text) {
		return ""
	}
	var b strings.Builder
	b.WriteString(NoStandingGrindFacts + "\n")
	if r, ok := c.ReputationNamed(text); ok {
		if p := r.Progress(); p != "" {
			b.WriteString("The character's standing (from the capture): " + p + ".\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}
	if len(c.Reputations) == 0 {
		b.WriteString("The character's reputations were not captured, so their standing with any faction is not known.")
		return b.String()
	}
	n := 0
	for _, r := range c.Reputations {
		if p := r.Progress(); p != "" && n < maxStandings {
			b.WriteString("- " + p + "\n")
			n++
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// maxStandings caps the factions listed when a reputation question names none.
const maxStandings = 8
