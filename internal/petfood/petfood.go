// Package petfood names the foods a hunter's pet eats, by item name and diet. The game data holds no pet diet (an item row has no food type, and
// npcs has a numeric beast family with no name and no diet), so which items are pet food is a short hand-written list, kept to the foods every
// Classic hunter feeds a pet (Warcraft Wiki, Pet food); an item it does not list is not claimed, and it says nothing of which family eats which diet.
package petfood

import "strings"

// Diets a pet food belongs to.
const (
	Meat   = "Meat"
	Fish   = "Fish"
	Bread  = "Bread"
	Cheese = "Cheese"
	Fruit  = "Fruit"
	Fungus = "Fungus"
)

var byDiet = map[string][]string{
	Meat: {"Tough Jerky", "Haunch of Meat", "Mutton Chop", "Wild Hog Shank", "Cured Ham Steak", "Smoked Bear Meat", "Chunk of Boar Meat", "Boar Ribs",
		"Lean Wolf Flank", "Stringy Wolf Meat", "Tender Wolf Meat", "Gooey Spider Leg", "Strider Meat", "Stringy Vulture Meat", "Crispy Bat Wing",
		"Tender Crocolisk Meat", "Heavy Crocolisk Meat", "Big Bear Meat", "Heavy Kodo Meat", "Raptor Flesh", "Crawler Meat"},
	Fish: {"Slitherskin Mackerel", "Raw Slitherskin Mackerel", "Longjaw Mud Snapper", "Raw Longjaw Mud Snapper", "Brilliant Smallfish", "Raw Brilliant Smallfish",
		"Rockscale Cod", "Raw Rockscale Cod", "Bristle Whisker Catfish", "Raw Bristle Whisker Catfish", "Cooked Glossy Mightfish", "Large Raw Mightfish", "Raw Glossy Mightfish"},
	Bread:  {"Tough Hunk of Bread", "Freshly Baked Bread", "Moist Cornbread", "Mulgore Spice Bread", "Soft Banana Bread"},
	Cheese: {"Darnassian Bleu", "Dalaran Sharp", "Fine Aged Cheddar", "Alterac Swiss", "Dwarven Mild", "Stormwind Brie"},
	Fruit:  {"Shiny Red Apple", "Goldenbark Apple", "Tel'Abim Banana", "Snapvine Watermelon"},
	Fungus: {"Red-speckled Mushroom", "Spongy Morel", "Delicious Cave Mold"},
}

var byName = func() map[string]string {
	m := map[string]string{}
	for d, l := range byDiet {
		for _, n := range l {
			m[strings.ToLower(n)] = d
		}
	}
	return m
}()

// Of is the diet an item name is food of, matched on the whole name in any case; ok is false for any other item.
func Of(name string) (diet string, ok bool) {
	diet, ok = byName[strings.ToLower(strings.TrimSpace(name))]
	return diet, ok
}
