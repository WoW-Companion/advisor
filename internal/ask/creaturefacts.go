package ask

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/store"
)

// creatureAsk is a question about a creature by name: where it is, its zone, level, faction or rank, whether it is a rare or an elite, what it drops, or when it spawns.
var creatureAsk = regexp.MustCompile(`(?i)\b(?:where|what zone|which zone|what area|what level|which level|what faction|which faction|how (?:strong|tough|hard)|how (?:do|can) i find|how to find|find|locate|drops?|dropped|loot\w*|gives?|spawn\w*|respawn\w*|rare|elite|boss|group)\b`)

// maxCreaturesAsked caps how many creatures one answer describes, maxCreatureLookups the name lookups a question costs, and maxCreatureDrops the drops listed for one.
const (
	maxCreaturesAsked   = 2
	maxCreatureLookups  = 30
	maxCreatureDrops    = 4
	creatureMinNameRune = 6
)

// CreatureFacts is what the dataset holds of each creature the question names (by an exact name in npcs): its level, zone, faction and rank where the row has them, "not held" where it has none, and
// the items players saw it drop (loot_sources, with the share of kills). "" for a question that does not ask about a creature or names none the dataset holds. The dataset holds no spawn point, respawn
// timer or elite status, so the lines say so: none is stated from memory.
func CreatureFacts(ds store.DataStore, text string) string {
	if ds == nil || !creatureAsk.MatchString(text) {
		return ""
	}
	words := setPhraseWord.FindAllString(text, -1)
	var ids []int64
	seen := map[int64]bool{}
	lookups := 0
	for i := 0; i < len(words) && lookups < maxCreatureLookups && len(ids) < maxCreaturesAsked; i++ {
		if !capital(words[i]) {
			continue
		}
		for n := min(5, len(words)-i); n >= 1 && lookups < maxCreatureLookups; n-- {
			if !capital(words[i+n-1]) || !nameShaped(words[i:i+n]) {
				continue
			}
			phrase := strings.Join(words[i:i+n], " ")
			k := names.Normalize(phrase)
			if len(k) < creatureMinNameRune {
				continue
			}
			lookups++
			hits, _, err := names.Find(ds, names.Npcs, phrase, 3)
			if err != nil {
				continue
			}
			found := false
			for _, h := range hits {
				if names.Normalize(h.Name) == k {
					found = true
					if !seen[h.ID] {
						seen[h.ID] = true
						ids = append(ids, h.ID)
					}
					break
				}
			}
			if found {
				i += n - 1
				break
			}
		}
	}
	var lines []string
	for _, id := range ids {
		row, err := ds.Get("npcs", id)
		if err != nil || row == nil {
			continue
		}
		lines = append(lines, creatureLine(ds, id, row))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Creatures named (from the npcs and loot_sources tables; answer from these, not from memory; the dataset holds no spawn point, patrol route, respawn timer, spawn condition or group size, and marks no creature a rare or an elite, so state none and call none a rare or an elite):\n" + strings.Join(lines, "\n")
}

// creatureLine is one creature: the row's facts, each said "not held" when the row has none, and the best drops.
func creatureLine(ds store.DataStore, id int64, row store.Row) string {
	level := "level not held"
	switch lo, hi := row["min_level"], row["max_level"]; {
	case lo != nil && hi != nil && num(lo) != num(hi):
		level = fmt.Sprintf("level %d-%d", num(lo), num(hi))
	case lo != nil:
		level = fmt.Sprintf("level %d", num(lo))
	case hi != nil:
		level = fmt.Sprintf("level %d", num(hi))
	}
	zone := "zone not held"
	if z := text0(row["zone_name"]); z != "" {
		zone = "zone " + z
	}
	faction := "faction not held"
	if row["faction_template_id"] != nil {
		faction = fmt.Sprintf("faction template %d", num(row["faction_template_id"]))
	}
	line := fmt.Sprintf("- %s (creature %d): %s; %s; %s; rank (rare or elite) not held", text0(row["name"]), id, level, zone, faction)
	if text0(row["name_source"]) != "" {
		line += "; it is known only as a source of drops (the name comes from players' drop reports)"
	}
	type drop struct {
		item         string
		drops, kills int64
	}
	rows, err := ds.Filter("loot_sources", store.Range{Column: "source_id", Min: id, Max: id})
	if err != nil {
		return line
	}
	var ds2 []drop
	for _, r := range rows {
		if text0(r["source_kind"]) != "creature" || num(r["kills"]) == 0 {
			continue
		}
		it, err := ds.Get("items", num(r["item_id"]))
		if err != nil || it == nil || text0(it["name"]) == "" {
			continue
		}
		ds2 = append(ds2, drop{text0(it["name"]), num(r["drops"]), num(r["kills"])})
	}
	sort.SliceStable(ds2, func(i, j int) bool {
		a, b := ds2[i], ds2[j]
		if x, y := a.drops*b.kills, b.drops*a.kills; x != y {
			return x > y
		}
		return a.item < b.item
	})
	var parts []string
	for _, d := range ds2 {
		if len(parts) == maxCreatureDrops {
			break
		}
		parts = append(parts, fmt.Sprintf("%s (about %.0f%% of %d kills reported)", d.item, 100*float64(d.drops)/float64(d.kills), d.kills))
	}
	if len(parts) > 0 {
		line += "; players saw it drop " + strings.Join(parts, ", ")
	}
	return line
}
