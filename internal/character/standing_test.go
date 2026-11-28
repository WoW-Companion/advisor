package character

import "testing"

func TestReputationsNamedByFullAndShortName(t *testing.T) {
	st := 5
	var reps []Reputation
	for i, n := range []string{"Stormwind", "Gnomeregan Exiles", "Timbermaw Hold", "Brood of Nozdormu", "The Defilers", "The League of Arathor", "Argent Dawn", "Argent Crusade", "Darnassus"} {
		reps = append(reps, Reputation{FactionID: i, Name: n, Standing: &st})
	}
	c := Character{Reputations: reps}
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"my Stormwind reputation", []string{"Stormwind"}},
		{"honored with timbermaw hold?", []string{"Timbermaw Hold"}},
		{"How do I reach Honored with Timbermaw?", []string{"Timbermaw Hold"}},
		{"Gnomeregan standing", []string{"Gnomeregan Exiles"}},
		{"Nozdormu rep", []string{"Brood of Nozdormu"}},
		{"Defilers rep", []string{"The Defilers"}},
		{"Arathor standing", []string{"The League of Arathor"}},
		{"Argent rep", []string{"Argent Dawn", "Argent Crusade"}},
		{"Stormwind and Timbermaw", []string{"Stormwind"}}, // a full name wins over a short one
		{"Thunder Clap rank", nil},
		{"the hold of my bags, a dawn raid", nil}, // a word of a name, not its first or last long word
	} {
		var got []string
		for _, r := range c.ReputationsNamed(tc.text) {
			got = append(got, r.Name)
		}
		if len(got) != len(tc.want) {
			t.Errorf("%q: %v, want %v", tc.text, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%q: %v, want %v", tc.text, got, tc.want)
			}
		}
	}
	if r, ok := c.ReputationNamed("Timbermaw"); !ok || r.Name != "Timbermaw Hold" {
		t.Errorf("ReputationNamed = %v %v", r, ok)
	}
}
