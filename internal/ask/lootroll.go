package ask

import (
	"regexp"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/store"
)

// lootWords make a question one about rolling for a drop.
var lootWords = regexp.MustCompile(`(?i)\b(need or greed|need|greed|roll|rolls|rolling|pass)\b`)

// itemInBrackets is an item named the way the game links it: [Name].
var itemInBrackets = regexp.MustCompile(`\[([^\]]{2,60})\]`)

// maxLootItems caps how many named items get a loot rule in one answer.
const maxLootItems = 3

// LootRollFacts is the loot rule for each item the question links by [name]: what the character rolls and how it binds, from the dataset row
// and the class rules. "" for a question that is not about rolling or that links no item. An item the dataset does not hold says so.
func LootRollFacts(c character.Character, ds store.DataStore, text string) string {
	if ds == nil || !lootWords.MatchString(text) {
		return ""
	}
	level := 0
	if c.Level != nil {
		level = *c.Level
	}
	var lines []string
	for _, m := range itemInBrackets.FindAllStringSubmatch(text, maxLootItems) {
		name := strings.TrimSpace(m[1])
		hits, _, err := names.Find(ds, names.Items, name, 1)
		var row store.Row
		if err == nil && len(hits) == 1 && strings.EqualFold(hits[0].Name, name) {
			row, _ = ds.Get("items", hits[0].ID)
		}
		lines = append(lines, "- "+name+": "+gear.LootRoll(c.Class, level, row).Text())
	}
	if len(lines) == 0 {
		return ""
	}
	return "Loot rolls (from the item rows and the class's gear rules; answer from these, not from memory):\n" + strings.Join(lines, "\n")
}
