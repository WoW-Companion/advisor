// Package classreagent names the items a class's own spells use up, by item name: a warlock's Soul Shards, a druid's seeds, a paladin's
// Symbols. The game data holds no spell reagent table (recipe_reagents lists profession recipes only), so which class an item is the
// reagent of is a short hand-written list, kept to the reagents every Classic player knows; an item it does not list is not claimed.
package classreagent

import "strings"

// Reagent is the class whose spells use the item up, and the spells.
type Reagent struct {
	Class  string // the capture's class token: "DRUID"
	Spells string // the spells that use it, for the keep line
}

var byName = map[string]Reagent{
	"soul shard":            {"WARLOCK", "Create Soulstone, Create Healthstone, Summon Voidwalker, Summon Succubus, Summon Incubus, Summon Felhunter and Enslave Demon"},
	"infernal stone":        {"WARLOCK", "Inferno"},
	"demonic figurine":      {"WARLOCK", "Ritual of Doom"},
	"maple seed":            {"DRUID", "Rebirth"},
	"stranglethorn seed":    {"DRUID", "Rebirth"},
	"ashwood seed":          {"DRUID", "Rebirth"},
	"hornbeam seed":         {"DRUID", "Rebirth"},
	"ironwood seed":         {"DRUID", "Rebirth"},
	"wild berries":          {"DRUID", "Gift of the Wild"},
	"wild thornroot":        {"DRUID", "Gift of the Wild"},
	"symbol of kings":       {"PALADIN", "Greater Blessings"},
	"symbol of divinity":    {"PALADIN", "Divine Intervention"},
	"rune of teleportation": {"MAGE", "Teleport spells"},
	"rune of portals":       {"MAGE", "Portal spells"},
	"arcane powder":         {"MAGE", "Arcane Brilliance"},
	"light feather":         {"PRIEST", "Levitate"},
	"holy candle":           {"PRIEST", "Prayer spells"},
	"sacred candle":         {"PRIEST", "Prayer spells"},
	"flash powder":          {"ROGUE", "Vanish"},
	"blinding powder":       {"ROGUE", "Blind"},
	"ankh":                  {"SHAMAN", "Reincarnation"},
}

// Of is the class reagent an item name is, matched on the whole name in any case; ok is false for any other item.
func Of(name string) (r Reagent, ok bool) {
	r, ok = byName[strings.ToLower(strings.TrimSpace(name))]
	return r, ok
}

// stones are the items a warlock's Create spells make.
var stones = []string{"healthstone", "soulstone", "firestone", "spellstone"}

// MadeBy is the class whose Create spell makes an item, by name ("Major Healthstone" is a warlock's); "" for any other item.
func MadeBy(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	for _, s := range stones {
		if strings.HasSuffix(n, s) {
			return "WARLOCK"
		}
	}
	return ""
}
