package items

import "testing"

func TestRankFaction(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"Knight-Captain's Chain Girdle", "Alliance"}, {"Sergeant Major's Plate Wristguards", "Alliance"}, {"Grand Marshal's Longsword", "Alliance"},
		{"Blood Guard's Mail Grips", "Horde"}, {"Legionnaire's Mail Cinch", "Horde"}, {"First Sergeant's Silk Cuffs", "Horde"}, {" High Warlord's Cleaver", "Horde"},
		{"Sergeant's Cloak", ""}, {"Knightly Longsword", ""}, {"Knight's Boots", ""}, {"Champion's Legplates", ""}, {"Mantle of Honor", ""}, {"", ""},
	} {
		if got := RankFaction(tc.name); got != tc.want {
			t.Errorf("RankFaction(%q) = %q, want %q", tc.name, got, tc.want)
		}
	}
	for _, tc := range []struct {
		name, faction string
		want          bool
	}{
		{"Knight-Captain's Chain Girdle", "Horde", true}, {"Knight-Captain's Chain Girdle", "alliance", false}, {"Blood Guard's Mail Grips", "Alliance", true},
		{"Blood Guard's Mail Grips", "", false}, {"Knightly Longsword", "Horde", false},
	} {
		if got := WrongFaction(tc.name, tc.faction); got != tc.want {
			t.Errorf("WrongFaction(%q, %q) = %v, want %v", tc.name, tc.faction, got, tc.want)
		}
	}
}
