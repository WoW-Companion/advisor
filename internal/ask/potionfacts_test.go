package ask

import (
	"strings"
	"testing"
)

func TestLimitFactsPotionRulesAreNotItemStackSizes(t *testing.T) {
	for _, tc := range []struct {
		q          string
		potion     bool
		stackLimit bool
	}{
		{"how long is the cooldown on a major healing potion?", true, false},
		{"can i stack a guardian elixir with a battle elixir", true, false},
		{"can i potion again after a mana potion", true, false},
		{"does a flask replace my elixirs", true, false},
		{"how many healing potions stack in one slot", false, true},
		{"what is the best elixir for a warrior", false, false},
	} {
		got := LimitFacts(tc.q)
		if has := strings.Contains(got, "Potions, elixirs and flasks:"); has != tc.potion {
			t.Errorf("%q: potion fact %v, want %v", tc.q, has, tc.potion)
		}
		if has := strings.Contains(got, "Stack size:"); has != tc.stackLimit {
			t.Errorf("%q: stack size fact %v, want %v", tc.q, has, tc.stackLimit)
		}
	}
}
