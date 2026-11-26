package levelup

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// classCities is the capital that holds a class trainer, by faction then class token. A city only:
// the dataset holds no class trainers, so a named NPC comes from the player's own captures or their
// QuestieDB, never from this table. A class with no entry here (a faction that cannot play it) gets no line.
var classCities = map[string]map[string]string{
	"horde": {"WARRIOR": "Undercity", "MAGE": "Undercity", "PRIEST": "Undercity", "WARLOCK": "Undercity", "ROGUE": "Undercity",
		"HUNTER": "Orgrimmar", "SHAMAN": "Orgrimmar", "DRUID": "Thunder Bluff", "PALADIN": "Silvermoon City"},
	"alliance": {"WARRIOR": "Stormwind City", "PALADIN": "Stormwind City", "ROGUE": "Stormwind City", "PRIEST": "Stormwind City",
		"MAGE": "Stormwind City", "WARLOCK": "Stormwind City", "HUNTER": "Ironforge", "SHAMAN": "Ironforge", "DRUID": "Darnassus"},
}

// flightsTo is the flight point with a direct flight to the city, by city then the zone it stands in.
var flightsTo = map[string]map[string]string{
	"undercity":       {"hillsbrad foothills": "Tarren Mill", "silverpine forest": "The Sepulcher"},
	"orgrimmar":       {"the barrens": "the Crossroads"},
	"stormwind city":  {"westfall": "Sentinel Hill", "redridge mountains": "Lakeshire", "duskwood": "Darkshire"},
	"ironforge":       {"loch modan": "Thelsamar", "wetlands": "Menethil Harbor"},
	"silvermoon city": {"ghostlands": "Tranquillien"},
}

// TrainPlace is where the character should go to train class spells.
type TrainPlace struct {
	City    string
	From    string   // the flight point to fly from, when a known direct flight reaches the city
	Trainer *Trainer // the class trainer's NPC and position, only when a capture or QuestieDB names one
	Seen    bool     // the trainer is one the player opened
	Here    bool     // the character is already in the city
}

// ClassCity is the capital for the faction's class trainers, "" when none is known.
func ClassCity(faction, class string) string {
	return classCities[strings.ToLower(faction)][strings.ToUpper(class)]
}

// TrainAt chooses the place to train class spells: a class trainer the player opened in the city or
// where they stand, else the class's trainer in the faction's city from QuestieDB, else the city alone.
// It never names an NPC or a position that no source holds.
func TrainAt(c character.Character, qx *questie.Index) (TrainPlace, bool) {
	city := ClassCity(c.Faction, c.Class)
	if city == "" {
		return TrainPlace{}, false
	}
	var at questie.Where
	zone := ""
	if p := c.LastKnownPosition(); p != nil {
		at = questie.Where{MapID: p.MapID, Zone: p.Zone, X: p.X, Y: p.Y}
		zone = p.Zone
	}
	if zone == "" && c.Location != nil {
		zone = c.Location.Zone // the capture names the zone but gave no position
	}
	pl := TrainPlace{City: city, Here: strings.EqualFold(zone, city)}
	if t, ok := SeenClassTrainer(c); ok && (strings.EqualFold(t.Zone, city) || zone != "" && strings.EqualFold(t.Zone, zone)) {
		pl.City, pl.Trainer, pl.Seen = t.Zone, &t, true
		pl.Here = strings.EqualFold(zone, t.Zone)
	} else if t, ok := cityTrainer(c, qx, city, at); ok {
		pl.Trainer = &t
	}
	if !pl.Here {
		pl.From = flightsTo[strings.ToLower(pl.City)][strings.ToLower(zone)]
	}
	return pl, true
}

// cityTrainer is the character's class trainer for their faction that QuestieDB places in the city or in
// the zone they stand in; a trainer anywhere else (a starting area) is never offered.
func cityTrainer(c character.Character, qx *questie.Index, city string, at questie.Where) (Trainer, bool) {
	var best Trainer
	found := false
	for _, t := range qx.ClassTrainers(c.Class) {
		if !learn.FactionOK(qx.NpcFaction(t.ID), c.Faction) {
			continue
		}
		p, ok := qx.NpcPlace(t.ID, at)
		if !ok || !strings.EqualFold(p.Zone, city) && !(at.Zone != "" && strings.EqualFold(p.Zone, at.Zone)) {
			continue
		}
		if !found {
			best, found = Trainer{t.Name, p.Zone, p.X, p.Y, p.Map}, true
		}
	}
	return best, found
}

// Line is the plain fact: "Train at: Undercity (fly from Tarren Mill)".
func (p TrainPlace) Line(class string) string {
	class = strings.ToLower(class)
	var b strings.Builder
	b.WriteString("Train at: " + p.City)
	switch {
	case p.Trainer != nil && p.Seen:
		fmt.Fprintf(&b, ": %s (%.0f, %.0f), the class trainer you visited", p.Trainer.Name, p.Trainer.X, p.Trainer.Y)
	case p.Trainer != nil:
		fmt.Fprintf(&b, ": %s (%.0f, %.0f)", p.Trainer.Name, p.Trainer.X, p.Trainer.Y)
	default:
		fmt.Fprintf(&b, ", your %s trainer there", class)
	}
	switch {
	case p.Here:
		b.WriteString(" (you are here)")
	case p.From != "":
		b.WriteString(" (fly from " + p.From + ")")
	}
	return b.String()
}

// FlightFrom is the flight point in zone with a direct flight to the city; "" when none is known.
func FlightFrom(city, zone string) string {
	return flightsTo[strings.ToLower(city)][strings.ToLower(zone)]
}
