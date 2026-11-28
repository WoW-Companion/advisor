package character

import (
	"fmt"
	"strings"
	"unicode"
)

// standingNames are the eight reputation standings, indexed by the game's standing id (1 Hated to 8 Exalted).
var standingNames = [...]string{"", "Hated", "Hostile", "Unfriendly", "Neutral", "Friendly", "Honored", "Revered", "Exalted"}

// standingFloors are the reputation points a standing starts at (the game's public thresholds), indexed by standing id;
// Exalted holds 1000 points before the bar is full.
var standingFloors = [...]int{0, -42000, -6000, -3000, 0, 3000, 9000, 21000, 42000}

const exaltedCap = 43000

// StandingName is the name of a reputation standing id, "" for an id outside 1 to 8.
func StandingName(id int) string {
	if id < 1 || id >= len(standingNames) {
		return ""
	}
	return standingNames[id]
}

// Progress says where the character stands with the faction: the standing, the points inside it and the points to the next
// standing, from the capture's own bar (min, max and value) and, where the capture lacks them, the game's public thresholds.
// It says what is not known and never a guess: "" when the capture holds no standing.
func (r Reputation) Progress() string {
	if r.Standing == nil || StandingName(*r.Standing) == "" {
		return ""
	}
	id := *r.Standing
	out := fmt.Sprintf("%s: %s", r.Name, StandingName(id))
	if r.Value == nil {
		return out + " (the points inside the standing are not captured)"
	}
	lo, hi := standingFloors[id], exaltedCap
	if id < 8 {
		hi = standingFloors[id+1]
	}
	if r.Min != nil && r.Max != nil && *r.Max > *r.Min {
		lo, hi = *r.Min, *r.Max
	}
	if id == 8 {
		return out + fmt.Sprintf(" (%d of %d points: nothing further to earn)", max(*r.Value-lo, 0), hi-lo)
	}
	left := max(hi-*r.Value, 0)
	return out + fmt.Sprintf(" (%d of %d points in the standing; %d more to %s)", max(*r.Value-lo, 0), hi-lo, left, StandingName(id+1))
}

// ReputationNamed is the character's reputation with the faction the text names, or false: the first of ReputationsNamed.
func (c Character) ReputationNamed(text string) (Reputation, bool) {
	if l := c.ReputationsNamed(text); len(l) > 0 {
		return l[0], true
	}
	return Reputation{}, false
}

// ReputationsNamed are the character's reputations with the factions the text names. A faction whose full name the text holds
// (case-insensitive) is the answer alone; failing that, the factions the short name a player says stands for: the first word of
// the name without a leading "The" ("Timbermaw" for Timbermaw Hold, "Gnomeregan" for Gnomeregan Exiles, "Thorium" for Thorium
// Brotherhood) or its last long word ("Nozdormu" for Brood of Nozdormu, "Arathor"), as a whole word.
func (c Character) ReputationsNamed(text string) []Reputation {
	t := strings.ToLower(text)
	var full []Reputation
	for _, r := range c.Reputations {
		if r.Name != "" && strings.Contains(t, strings.ToLower(r.Name)) {
			full = append(full, r)
		}
	}
	if len(full) > 0 {
		return full
	}
	words := map[string]bool{}
	for _, w := range strings.FieldsFunc(t, func(r rune) bool { return !unicode.IsLetter(r) && r != '\'' }) {
		words[w] = true
	}
	var short []Reputation
	for _, r := range c.Reputations {
		parts := strings.Fields(strings.ToLower(r.Name))
		the := len(parts) > 1 && parts[0] == "the"
		if the {
			parts = parts[1:]
		}
		if len(parts) < 2 && !the { // a one-word name is matched in full above
			continue
		}
		first, last := parts[0], parts[len(parts)-1]
		if len(first) >= 5 && words[first] || len(last) >= 6 && words[last] {
			short = append(short, r)
		}
	}
	return short
}
