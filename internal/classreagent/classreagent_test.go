package classreagent

import "testing"

func TestOf(t *testing.T) {
	for _, tc := range []struct {
		name, class string
	}{
		{"Soul Shard", "WARLOCK"}, {"maple seed", "DRUID"}, {"Symbol of Kings", "PALADIN"}, {" Flash Powder ", "ROGUE"},
		{"Soul Dust", ""}, {"Mining Pick", ""}, {"", ""},
	} {
		r, ok := Of(tc.name)
		if r.Class != tc.class || ok != (tc.class != "") {
			t.Errorf("Of(%q) = %+v, %v; want class %q", tc.name, r, ok, tc.class)
		}
	}
}

func TestMadeBy(t *testing.T) {
	for name, want := range map[string]string{"Healthstone": "WARLOCK", "Major Soulstone": "WARLOCK", "Firestone": "WARLOCK", "Whetstone": "", "Soul Dust": ""} {
		if got := MadeBy(name); got != want {
			t.Errorf("MadeBy(%q) = %q, want %q", name, got, want)
		}
	}
}
