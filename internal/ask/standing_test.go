package ask

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

// twelve captured reputations: the answer lists eight at most, so the ninth or later must be found by name.
func twelveFactions() character.Character {
	var c character.Character
	for i, n := range []string{"Stormwind", "Ironforge", "Darnassus", "Gnomeregan Exiles", "Exodar", "Orgrimmar", "Undercity", "Thunder Bluff", "Darkspear Trolls", "Timbermaw Hold", "Argent Dawn", "Cenarion Circle"} {
		st, v := 5, 3000+i
		c.Reputations = append(c.Reputations, character.Reputation{FactionID: i, Name: n, Standing: &st, Value: &v, Min: new(int), Max: new(int)})
	}
	return c
}

func TestStandingFactsFindTheFactionByItsShortName(t *testing.T) {
	c := twelveFactions()
	own := c.Reputations[9].Progress()
	for _, q := range []string{"How do I get Honored with Timbermaw?", "What is my Timbermaw Hold reputation?"} {
		f := StandingFacts(c, q)
		if !strings.Contains(f, own) || strings.Count(f, "\n- ") != 0 || strings.Contains(f, "Stormwind:") {
			t.Errorf("%q:\n%s", q, f)
		}
	}
	f := StandingFacts(c, "Where do I stand with Argent and the Cenarion standing?")
	for _, i := range []int{10, 11} {
		if !strings.Contains(f, c.Reputations[i].Progress()) {
			t.Errorf("faction %d missing:\n%s", i, f)
		}
	}
}

func TestStandingFactsAnswerThePluralAndTheCap(t *testing.T) {
	c := twelveFactions()
	for _, q := range []string{"What are my reputations?", "show my reps", "all my standings"} {
		f := StandingFacts(c, q)
		if !strings.HasPrefix(f, NoStandingGrindFacts) || strings.Count(f, "\n- ") != maxStandings {
			t.Errorf("%q: %d lines\n%s", q, strings.Count(f, "\n- "), fmt.Sprint(f))
		}
	}
	if f := StandingFacts(c, "What is the weather like in Elwynn?"); f != "" {
		t.Errorf("not a reputation question: %q", f)
	}
}
