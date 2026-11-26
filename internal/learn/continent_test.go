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
