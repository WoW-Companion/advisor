package learn

import "strings"

// continents maps a zone's name to its continent. The dataset has no continent column and
// Questie's area table names zones only, so the zones with a profession trainer or a city are listed.
var continents = func() map[string]string {
	m := map[string]string{}
	add := func(c string, zones ...string) {
		for _, z := range zones {
			m[strings.ToLower(z)] = c
		}
	}
	add("Eastern Kingdoms", "Elwynn Forest", "Stormwind City", "Dun Morogh", "Ironforge", "Loch Modan", "Westfall",
		"Redridge Mountains", "Duskwood", "Wetlands", "Arathi Highlands", "Hillsbrad Foothills", "Alterac Mountains",
		"Silverpine Forest", "Tirisfal Glades", "Undercity", "Western Plaguelands", "Eastern Plaguelands",
		"The Hinterlands", "Searing Gorge", "Burning Steppes", "Badlands", "Swamp of Sorrows", "Blasted Lands",
		"Stranglethorn Vale", "Deadwind Pass", "Eversong Woods", "Silvermoon City", "Ghostlands", "Isle of Quel'Danas",
		"Eastern Kingdoms", "Uldaman", "Riverglades")
	add("Kalimdor", "Durotar", "Orgrimmar", "Mulgore", "Thunder Bluff", "Teldrassil", "Darnassus", "Darkshore",
		"Ashenvale", "Stonetalon Mountains", "The Barrens", "Northern Barrens", "Southern Barrens", "Desolace",
		"Feralas", "Dustwallow Marsh", "Thousand Needles", "Tanaris", "Un'Goro Crater", "Silithus", "Winterspring",
		"Felwood", "Moonglade", "Azshara", "Azuremyst Isle", "The Exodar", "Bloodmyst Isle", "Mount Hyjal", "Shen'dralas", "Darkspear Islands", "Kalimdor")
	add("Outland", "Hellfire Peninsula", "Zangarmarsh", "Terokkar Forest", "Nagrand", "Blade's Edge Mountains",
		"Netherstorm", "Shadowmoon Valley", "Shattrath City")
	return m
}()

// continentOf is the zone's continent, or "" when the zone is not known.
func continentOf(zone string) string { return continents[strings.ToLower(strings.TrimSpace(zone))] }

// ContinentOf is the zone's continent, or "" when the zone is not known.
func ContinentOf(zone string) string { return continentOf(zone) }

// crossings are the ways between continents, by lower-case faction, from and to: the zeppelin or boat that joins
// Eastern Kingdoms and Kalimdor, and the Dark Portal that leads to Outland from the Blasted Lands.
var crossings = map[[3]string]string{
	{"horde", "Eastern Kingdoms", "Kalimdor"}:    "Zeppelin to Orgrimmar",
	{"horde", "Kalimdor", "Eastern Kingdoms"}:    "Zeppelin to Undercity",
	{"alliance", "Eastern Kingdoms", "Kalimdor"}: "Boat to Auberdine",
	{"alliance", "Kalimdor", "Eastern Kingdoms"}: "Boat to Menethil Harbor",
}

const (
	darkPortalIn  = "the Dark Portal in the Blasted Lands"
	darkPortalOut = "the Dark Portal in Hellfire Peninsula"
)

// Crossing is the travel step between two continents for a faction: "Zeppelin to Orgrimmar", or for
// Outland the Dark Portal (after the zeppelin or boat first when the player is on Kalimdor). "" when both
// are the same continent, or either is not known: the player then walks or rides.
func Crossing(faction, from, to string) string {
	if from == "" || to == "" || from == to {
		return ""
	}
	faction = strings.ToLower(strings.TrimSpace(faction)) // the capture says "Horde", a setting may say "horde"
	switch {
	case to == "Outland":
		if from == "Eastern Kingdoms" {
			return "Travel to " + darkPortalIn + " and step through"
		}
		if hub := crossings[[3]string{faction, from, "Eastern Kingdoms"}]; hub != "" {
			return hub + ", then " + darkPortalIn
		}
	case from == "Outland":
		if to == "Eastern Kingdoms" {
			return "Step through " + darkPortalOut
		}
		if hub := crossings[[3]string{faction, "Eastern Kingdoms", to}]; hub != "" {
			return "Step through " + darkPortalOut + ", then " + hub
		}
	}
	return crossings[[3]string{faction, from, to}]
}
