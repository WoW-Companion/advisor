package learn

import "testing"

func TestContinentOfNamesTheZonesTheDatasetHolds(t *testing.T) {
	for zone, want := range map[string]string{"Mount Hyjal": "Kalimdor", "Kalimdor": "Kalimdor", "Uldaman": "Eastern Kingdoms",
		"Eastern Kingdoms": "Eastern Kingdoms", "Zephras Isle": ""} {
		if got := ContinentOf(zone); got != want {
			t.Errorf("ContinentOf(%q) = %q, want %q", zone, got, want)
		}
	}
}

func TestCrossing(t *testing.T) {
	for _, tc := range []struct{ side, from, to, want string }{
		{"Horde", "Eastern Kingdoms", "Kalimdor", "Zeppelin to Orgrimmar"},
		{"Alliance", "Kalimdor", "Eastern Kingdoms", "Boat to Menethil Harbor"},
		{"horde", "Eastern Kingdoms", "Kalimdor", "Zeppelin to Orgrimmar"}, // a side spelled in lower case
		{"Horde", "Eastern Kingdoms", "Eastern Kingdoms", ""},
		{"Horde", "", "Kalimdor", ""},
		{"", "Eastern Kingdoms", "Kalimdor", ""},
		{"Horde", "Eastern Kingdoms", "Outland", "Travel to the Dark Portal in the Blasted Lands and step through"},
		{"Alliance", "Kalimdor", "Outland", "Boat to Menethil Harbor, then the Dark Portal in the Blasted Lands"},
		{"Horde", "Outland", "Eastern Kingdoms", "Step through the Dark Portal in Hellfire Peninsula"},
		{"Horde", "Outland", "Kalimdor", "Step through the Dark Portal in Hellfire Peninsula, then Zeppelin to Orgrimmar"},
	} {
		if got := Crossing(tc.side, tc.from, tc.to); got != tc.want {
			t.Errorf("Crossing(%q, %q, %q) = %q, want %q", tc.side, tc.from, tc.to, got, tc.want)
		}
	}
}

func TestContinentOfNamesTheStartingAreas(t *testing.T) {
	for area, want := range map[string]string{"Deathknell": "Eastern Kingdoms", "Valley of Trials": "Kalimdor", "Shadowglen": "Kalimdor", "Northshire Valley": "Eastern Kingdoms"} {
		if got := ContinentOf(area); got != want {
			t.Errorf("ContinentOf(%q) = %q, want %q", area, got, want)
		}
	}
}

func TestIsRidingQuery(t *testing.T) {
	for q, want := range map[string]bool{"Riding": true, "apprentice riding": true, "Riding Skill": true, "Enchant Gloves - Riding Skill": false,
		"Formula: Enchant Gloves - Riding Skill": false, "Riding Gloves": true, "Linen Bandage": false} {
		if got := isRidingQuery(q); got != want {
			t.Errorf("isRidingQuery(%q) = %v, want %v", q, got, want)
		}
	}
}

func TestLandmassSeparatesTheIslandsOffKalimdor(t *testing.T) {
	for zone, want := range map[string]string{"Azuremyst Isle": "Azuremyst Isle", "The Exodar": "Azuremyst Isle", "Bloodmyst Isle": "Azuremyst Isle",
		"Darkshore": "Kalimdor", "Wetlands": "Eastern Kingdoms", "Zephras Isle": ""} {
		if got := Landmass(zone); got != want {
			t.Errorf("Landmass(%q) = %q, want %q", zone, got, want)
		}
	}
}
