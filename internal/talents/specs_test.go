package talents

import (
	"reflect"
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
