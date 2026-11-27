package character

import (
	"fmt"
	"strings"
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

// ReputationNamed is the character's reputation with the faction whose name holds the text (case-insensitive), or false.
func (c Character) ReputationNamed(text string) (Reputation, bool) {
	t := strings.ToLower(text)
	for _, r := range c.Reputations {
		if r.Name != "" && strings.Contains(t, strings.ToLower(r.Name)) {
			return r, true
		}
	}
	return Reputation{}, false
}
