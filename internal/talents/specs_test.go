package talents

import (
	"reflect"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
)

func TestSpecsFallBackToCaptureAndMatch(t *testing.T) {
	c := character.Character{Class: "Warrior", Talents: &character.Talents{Trees: []character.TalentTree{{Name: "Fury"}, {Name: "Arms"}, {Name: "Protection"}, {Name: "Mining", Key: "professions"}}}}
	got := Specs(nil, c)
	if want := []string{"Arms", "Fury", "Protection"}; !reflect.DeepEqual(got, want) {
		t.Errorf("specs = %v, want %v", got, want)
	}
	for in, want := range map[string]string{"fury": "Fury", " Arms ": "Arms", "Furry": "", "": "", "Fury Warrior": ""} {
		if g := MatchSpec(got, in); g != want {
			t.Errorf("MatchSpec(%q) = %q, want %q", in, g, want)
		}
	}
}

func TestNormalizeSpec(t *testing.T) {
	specs := []string{"Arms", "Fury", "Protection"}
	for _, tc := range []struct{ in, want string }{
		{"Fury", "Fury"}, {"fury", "Fury"}, {" FURY ", "Fury"}, {"Fury warrior", "Fury"}, {"fury Warrior", "Fury"},
		{"prot", "Protection"}, {"Ar", "Arms"}, {"", ""},
		{"A", "Arms"}, {"Arcane", "!"}, {"warrior", "!"}, {"F ury", "!"},
	} {
		got, err := NormalizeSpec(specs, "Warrior", tc.in)
		if tc.want == "!" {
			if err == nil || !strings.Contains(err.Error(), "Arms, Fury, Protection") {
				t.Errorf("%q: %q, %v; want a refusal that lists the specs", tc.in, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("%q: %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	if _, err := NormalizeSpec([]string{"Fury", "Furor"}, "", "fur"); err == nil {
		t.Error("an ambiguous prefix was accepted")
	}
	if got, err := NormalizeSpec(nil, "Warrior", " Fury "); err != nil || got != "Fury" {
		t.Errorf("no specs to check against: %q, %v", got, err)
	}
}
