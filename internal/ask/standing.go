package ask

import (
	"regexp"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// standingWords make a question one about reputation: a faction's standing, what a standing unlocks, or how to earn it.
var standingWords = regexp.MustCompile(`(?i)\b(reputations?|reps?|standings?|friendly|honou?red|revered|exalted|unfriendly|hostile|hated|grind(?:ing)? reps?)\b`)

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
	if named := c.ReputationsNamed(text); len(named) > 0 {
		if len(named) == 1 {
			if p := named[0].Progress(); p != "" {
				b.WriteString("The character's standing (from the capture): " + p + ".\n")
			}
		} else {
			b.WriteString("The character's standing with the factions the question names (from the capture):\n")
			for _, r := range named {
				if p := r.Progress(); p != "" {
					b.WriteString("- " + p + "\n")
				}
			}
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
