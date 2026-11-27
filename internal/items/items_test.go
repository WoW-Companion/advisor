package items

import "testing"

func TestUnobtainable(t *testing.T) {
	for _, tc := range []struct {
		name  string
		label string // "" for an obtainable item
	}{
		{"DNT HEAD", "DNT"},
		{"  dnt Sword", "DNT"},
		{"Deprecated Battle Chain Pauldrons", "Deprecated"},
		{"deprecated Thing", "Deprecated"},
		{"[PH] Mace", "[PH]"},
		{"PH Cloak", "PH "},
		{"OLD Dagger", "OLD"},
		{"zzOld Shield", "zz"},
		{"ZZ Boots", "ZZ"},
		{"Test Ring", "Test "},
		{"TEST-Helm", "TEST"},
		{"QA Staff", "QA "},
		{"PVP TEST Alliance Ear", "PVP TEST"},
		{"PVP TEST Horde Ear", "PVP TEST"},
		{"Monster - Sword, Axe", "Monster -"},
		{"Unused Wand", "Unused"},
		{"Sword (DNT)", "(DNT)"},
		{"Sword [DNT]", "[DNT]"},
		{"Blade of DEPRECATED", "DEPRECATED"},
		{"A Placeholder Item", "PLACEHOLDER"},
		// Real items the words merely appear in.
		{"Old Blanchy's Blanket", ""},
		{"Old Towsel", ""},
		{"PVP Trinket", ""},
		{"Testament of Light", ""},
		{"Test of Endurance", ""},
		{"Phoenix Feather", ""},
		{"Dntfoo", ""},
		{"Zzzyxas' Insignia", ""},
		{"Unusual Gem", ""},
		{"Qaldor's Blade", ""},
		{"Thunderfury, Blessed Blade of the Windseeker", ""},
		{"", ""},
	} {
		if got := Match(tc.name); got != tc.label {
			t.Errorf("Match(%q) = %q, want %q", tc.name, got, tc.label)
		}
		if Unobtainable(tc.name) != (tc.label != "") {
			t.Errorf("Unobtainable(%q) disagrees with Match", tc.name)
		}
	}
}
