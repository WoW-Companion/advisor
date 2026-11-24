package advice

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Level band for a quest the character has not picked up: the character's level -2 to +2.
const (
	AvailBelow = 2
	AvailAbove = 2
	// MaxAvailable caps how many "Pick up" steps WhatsNext adds.
	MaxAvailable = 3
)

// PickUpPrefix starts the title of a step for a quest that is not yet in the log.
const PickUpPrefix = "Pick up: "

// classIDs and raceIDs map the capture's tokens to the game's ids; a mask bit is 1<<(id-1).
var classIDs = map[string]int{"WARRIOR": 1, "PALADIN": 2, "HUNTER": 3, "ROGUE": 4, "PRIEST": 5, "DEATHKNIGHT": 6,
	"SHAMAN": 7, "MAGE": 8, "WARLOCK": 9, "MONK": 10, "DRUID": 11, "DEMONHUNTER": 12, "EVOKER": 13}

var raceIDs = map[string]int{"human": 1, "orc": 2, "dwarf": 3, "nightelf": 4, "scourge": 5, "undead": 5, "tauren": 6,
	"gnome": 7, "troll": 8, "goblin": 9, "bloodelf": 10, "draenei": 11, "worgen": 22, "pandaren": 24}

// Available is the quests a character could pick up now. Unchecked names each rule the dataset
// could not support, so an empty list is never read as "nothing to pick up".
type Available struct {
	Quests    []Step
	Unchecked []string
}

func asInt(v any) (int, bool) {
	n, ok := v.(int64)
	return int(n), ok
}

func maskAllows(row store.Row, col string, id int) (allowed, checked bool) {
	m, ok := asInt(row[col])
	if !ok || id == 0 {
		return true, false
	}
	return m <= 0 || m&(1<<(id-1)) != 0, true
}

// AvailableQuests finds quests the character has not picked up but could: level within the band,
// class and race allowed, the previous step of its quest line completed, not completed, not in the
// log, not a DNT or deprecated placeholder, and in a zone the log already has quests in (the
// last-saved zone, or else the zones of the log's quests, is "nearby"). Ranked by level
// fit, then a shared quest line, then a shared zone. ds may be nil.
func AvailableQuests(c character.Character, ds store.DataStore) Available {
	var out Available
	if ds == nil || c.Level == nil {
		return out
	}
	lo, hi := *c.Level-AvailBelow, *c.Level+AvailAbove
	rows, err := ds.Filter("quests", store.Range{Column: "level", Min: int64(lo), Max: int64(hi)})
	if err != nil {
		out.Unchecked = append(out.Unchecked, "the dataset has no quest level column, so no quest can be placed in the level band")
		return out
	}
	if len(rows) == 0 {
		out.Unchecked = append(out.Unchecked, fmt.Sprintf("the dataset holds no quest of level %d to %d", lo, hi))
		return out
	}
	done := map[int64]bool{}
	for _, id := range c.CompletedQuests {
		done[int64(id)] = true
	}
	logged := map[int64]bool{}
	logZones, logLines := map[string]int{}, map[int64]bool{}
	here := map[string]bool{}
	if l := c.Location; l != nil {
		for _, z := range []string{l.Zone, l.MapName} {
			if z != "" {
				here[z] = true
			}
		}
	}
	if p := c.LastKnownPosition(); p != nil && p.Zone != "" {
		here[p.Zone] = true
	}
	for _, q := range c.Quests {
		logged[int64(q.QuestID)] = true
		if z := lookup(q, ds).zone; z != "" {
			logZones[z]++
		}
		if ch := quests.Of(ds, int64(q.QuestID)); ch.Found {
			logLines[ch.LineID] = true
		}
	}
	if len(logZones) == 0 && len(here) == 0 {
		out.Unchecked = append(out.Unchecked, "the capture has no last-saved zone and no zone for the quests in the log, so nothing can be called nearby")
		return out
	}
	type scored struct {
		step       Step
		diff       int
		line, zone bool
	}
	var found []scored
	classID, raceID := classIDs[strings.ToUpper(c.Class)], raceIDs[strings.ToLower(strings.ReplaceAll(c.Race, " ", ""))]
	var classSeen, raceSeen bool
	for _, r := range rows {
		id, _ := r["id"].(int64)
		name, _ := r["name"].(string)
		if id == 0 || done[id] || logged[id] || (name != "" && items.Unobtainable(name)) {
			continue
		}
		if ok, chk := maskAllows(r, "allowable_class_mask", classID); chk {
			classSeen = true
			if !ok {
				continue
			}
		}
		if ok, chk := maskAllows(r, "allowable_race_mask", raceID); chk {
			raceSeen = true
			if !ok {
				continue
			}
		}
		zone, _ := r["zone_name"].(string)
		if logZones[zone] == 0 && !here[zone] {
			continue
		}
		ch := quests.Of(ds, id)
		if ch.Found && ch.Position > 1 && !done[ch.Steps[ch.Position-2].QuestID] {
			continue // an earlier step of its quest line is not done yet
		}
		lvl, _ := asInt(r["level"])
		d := lvl - *c.Level
		if d < 0 {
			d = -d
		}
		label := name
		if label == "" {
			label = fmt.Sprintf("quest %d", id)
		}
		reason := fmt.Sprintf("level %d quest, you are %d; not in your log", lvl, *c.Level)
		if zone != "" {
			reason += ", in " + zone
		}
		s := Step{Kind: KindQuest, Title: PickUpPrefix + label, Reason: reason, Zone: zone, QuestID: int(id), Name: label, Level: lvl, NPC: quests.GiverName(ds, id),
			Sources: []string{fmt.Sprintf("dataset: quests id %d", id)}}
		sc := scored{step: s, diff: d, line: ch.Found && logLines[ch.LineID], zone: logZones[zone] > 0 || here[zone]}
		switch {
		case sc.line:
			sc.step.Reason += ", continues a quest line you are on"
		case here[zone] && logZones[zone] == 0:
			sc.step.Reason += ", where you were last saved"
		case sc.zone:
			sc.step.Reason += fmt.Sprintf(", on the way: %d %s in your log %s there", logZones[zone], plural(logZones[zone], "quest", "quests"), plural(logZones[zone], "is", "are"))
		}
		found = append(found, sc)
	}
	if !classSeen {
		out.Unchecked = append(out.Unchecked, "the dataset has no class restrictions for quests (class not checked)")
	}
	if !raceSeen {
		out.Unchecked = append(out.Unchecked, "the dataset has no race restrictions for quests (race not checked)")
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		switch {
		case a.diff != b.diff:
			return a.diff < b.diff
		case a.line != b.line:
			return a.line
		case a.zone != b.zone:
			return a.zone
		}
		return a.step.Title < b.step.Title
	})
	for _, f := range found {
		out.Quests = append(out.Quests, f.step)
	}
	return out
}
