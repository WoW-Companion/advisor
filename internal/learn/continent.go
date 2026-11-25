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
		"Stranglethorn Vale", "Deadwind Pass", "Eversong Woods", "Silvermoon City", "Ghostlands", "Isle of Quel'Danas")
	add("Kalimdor", "Durotar", "Orgrimmar", "Mulgore", "Thunder Bluff", "Teldrassil", "Darnassus", "Darkshore",
		"Ashenvale", "Stonetalon Mountains", "The Barrens", "Northern Barrens", "Southern Barrens", "Desolace",
		"Feralas", "Dustwallow Marsh", "Thousand Needles", "Tanaris", "Un'Goro Crater", "Silithus", "Winterspring",
		"Felwood", "Moonglade", "Azshara", "Azuremyst Isle", "The Exodar", "Bloodmyst Isle")
	add("Outland", "Hellfire Peninsula", "Zangarmarsh", "Terokkar Forest", "Nagrand", "Blade's Edge Mountains",
		"Netherstorm", "Shadowmoon Valley", "Shattrath City")
	return m
}()

// continentOf is the zone's continent, or "" when the zone is not known.
func continentOf(zone string) string { return continents[strings.ToLower(strings.TrimSpace(zone))] }
