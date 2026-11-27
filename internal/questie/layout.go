package questie

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
)

// Layout is the field numbering of the installed QuestieDB: which numbered field of a quest, NPC,
// object or item row holds which named value. The numbers are read at run time from the database's
// own src/meta/*Meta.lua files (a name-to-number table), never kept here as the truth: a build moves
// fields around (Forever has extraObjectives at 29, where the classic layout has 27). Only when a
// meta file is absent does the reader fall back to the classic numbering, and Source says which.
type Layout struct {
	Source string // "meta" when every table came from the install, "classic" when none did, else "mixed"
	tables map[string]map[string]int
}

// classic is the numbering used when the install has no meta file for a table.
var classic = map[string]map[string]int{
	"quest": {"name": 1, "startedBy": 2, "finishedBy": 3, "requiredLevel": 4, "questLevel": 5, "requiredRaces": 6,
		"requiredClasses": 7, "objectivesText": 8, "triggerEnd": 9, "objectives": 10, "sourceItemId": 11, "requiredSourceItems": 21, "preQuestGroup": 12,
		"preQuestSingle": 13, "exclusiveTo": 16, "zoneOrSort": 17, "nextQuestInChain": 22, "parentQuest": 25,
		"extraObjectives": 27, "requiredMaxLevel": 32},
	"npc":    {"name": 1, "rank": 6, "spawns": 7, "friendlyToFaction": 13, "subName": 14, "npcFlags": 15, "minLevel": 4, "maxLevel": 5},
	"object": {"name": 1, "spawns": 4},
	"item":   {"name": 1, "npcDrops": 2, "objectDrops": 3, "requiredLevel": 10},
}

// metaFiles names each table's meta file under src/meta.
var metaFiles = map[string]string{"quest": "questMeta.lua", "npc": "npcMeta.lua", "object": "objectMeta.lua", "item": "itemMeta.lua"}

var metaEntry = regexp.MustCompile(`\[?\s*['"]?([A-Za-z_][A-Za-z0-9_]*)['"]?\s*\]?\s*=\s*(\d+)\s*[,}]`)

// parseMeta reads a "['name'] = N" table as text; the file is never run. nil when it holds no entry.
func parseMeta(path string) map[string]int {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	out := map[string]int{}
	for _, m := range metaEntry.FindAllStringSubmatch(string(b), -1) {
		n, _ := strconv.Atoi(m[2])
		if _, dup := out[m[1]]; !dup && n > 0 {
			out[m[1]] = n
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ReadLayout reads the key maps of the QuestieDB that st.TOC belongs to.
func ReadLayout(st Status) Layout {
	dir := filepath.Dir(st.TOC)
	l := Layout{tables: map[string]map[string]int{}}
	fromMeta := 0
	for tbl, file := range metaFiles {
		var m map[string]int
		for _, base := range []string{dir, filepath.Join(dir, "support", "Forever")} {
			if m = parseMeta(filepath.Join(base, "src", "meta", file)); m != nil {
				break
			}
		}
		if m != nil {
			fromMeta++
		} else {
			m = classic[tbl]
		}
		l.tables[tbl] = m
	}
	switch fromMeta {
	case 0:
		l.Source = "classic"
	case len(metaFiles):
		l.Source = "meta"
	default:
		l.Source = "mixed"
	}
	return l
}

// at is the field number of name in table tbl, "" when the layout does not have it.
func (l Layout) at(tbl, name string) string {
	if n, ok := l.tables[tbl][name]; ok {
		return strconv.Itoa(n)
	}
	if n, ok := classic[tbl][name]; ok && l.tables[tbl] != nil && !l.claimed(tbl, n) {
		return strconv.Itoa(n)
	}
	return ""
}

// claimed reports whether the layout gives field n another name.
func (l Layout) claimed(tbl string, n int) bool {
	for _, v := range l.tables[tbl] {
		if v == n {
			return true
		}
	}
	return false
}

func (l Layout) sc(tbl, name string) int64 {
	n, _ := strconv.Atoi(l.at(tbl, name))
	return int64(n)
}

// questRestrictions are the quest fields that mean "a requirement this reader cannot check".
var questRestrictions = []string{"requiredSkill", "requiredMinRep", "requiredMaxRep", "specialFlags", "requiredSpell",
	"requiredSpecialization", "requiredMinRepValue", "requiredMaxRepValue"}

// classicRestricted are the numbers those fields have when the install names no such field.
var classicRestricted = []int64{18, 19, 20, 24, 30, 31, 34, 35}

// restricted lists the quest field numbers that hold a requirement.
func (l Layout) restricted() []int64 {
	var out []int64
	seen := map[int64]bool{}
	for _, n := range questRestrictions {
		if v := l.sc("quest", n); v > 0 && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for _, v := range classicRestricted {
		if !seen[v] && !l.claimed("quest", int(v)) {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// wantedFields are the list-valued fields the reader decodes, by row type; "S" holds the scalars.
func (l Layout) wantedFields() map[string]map[string]bool {
	w := map[string]map[string]bool{"Quest": {"S": true}, "Npc": {"S": true}, "Object": {"S": true}, "Item": {"S": true}}
	add := func(typ, tbl string, names ...string) {
		for _, n := range names {
			if f := l.at(tbl, n); f != "" {
				w[typ][f] = true
			}
		}
	}
	add("Quest", "quest", "startedBy", "finishedBy", "objectivesText", "triggerEnd", "objectives", "preQuestGroup",
		"preQuestSingle", "exclusiveTo", "extraObjectives")
	add("Npc", "npc", "spawns")
	add("Object", "object", "spawns")
	add("Item", "item", "npcDrops", "objectDrops")
	return w
}
