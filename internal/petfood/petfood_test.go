package petfood

import "testing"

func TestOf(t *testing.T) {
	for _, tc := range []struct {
		name, diet string
		ok         bool
	}{
		{"Haunch of Meat", Meat, true}, {"raw boar ribs", "", false}, {"Boar Ribs", Meat, true}, {"Darnassian Bleu", Cheese, true},
		{" Spongy Morel ", Fungus, true}, {"Shiny Red Apple", Fruit, true}, {"Linen Cloth", "", false}, {"Hunter's Mark", "", false},
	} {
		if d, ok := Of(tc.name); d != tc.diet || ok != tc.ok {
			t.Errorf("Of(%q) = %q, %v; want %q, %v", tc.name, d, ok, tc.diet, tc.ok)
		}
	}
}
