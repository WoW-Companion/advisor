package ask

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/store"
)

// setAsk is a question that touches an item set: its pieces, its name, or whether an item belongs to one.
var setAsk = regexp.MustCompile(`(?i)\b(?:item sets?|sets?|pieces?|tier ?\d*)\b`)

// maxSetsAsked caps how many sets one answer lists, and maxSetLookups the name lookups a question costs.
const (
	maxSetsAsked  = 3
	maxSetLookups = 40
)

var setPhraseWord = regexp.MustCompile(`[\p{L}'’-]+`)

// SetFacts is the membership of each item set the question names (or the set an [item] in the question belongs to): the set's name and id and its pieces, from item_sets,
// each piece named from items where the table holds it. "" for a question that is not about a set or names none the dataset holds. The dataset holds no set bonus, so the
// line says so: the bonus is never stated.
func SetFacts(ds store.DataStore, text string) string {
	if ds == nil || !setAsk.MatchString(text) {
		return ""
	}
	var ids []int64
	seen := map[int64]bool{}
	add := func(id int64) {
		if !seen[id] && len(ids) < maxSetsAsked {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	for _, m := range itemInBrackets.FindAllStringSubmatch(text, maxLootItems) {
		hits, _, err := names.Find(ds, names.Items, strings.TrimSpace(m[1]), 1)
		if err != nil || len(hits) != 1 || !strings.EqualFold(hits[0].Name, strings.TrimSpace(m[1])) {
			continue
		}
		rows, err := ds.Filter("item_sets", store.Range{Column: "item_id", Min: hits[0].ID, Max: hits[0].ID})
		if err != nil {
			continue
		}
		for _, r := range rows {
			add(num(r["set_id"]))
		}
	}
	words := setPhraseWord.FindAllString(text, -1)
	lookups := 0
	for i := 0; i < len(words) && lookups < maxSetLookups; i++ {
		if !capital(words[i]) {
			continue
		}
		for n := min(5, len(words)-i); n >= 1 && lookups < maxSetLookups; n-- {
			if !capital(words[i+n-1]) || !nameShaped(words[i:i+n]) {
				continue
			}
			phrase := strings.Join(words[i:i+n], " ")
			if len(names.Normalize(phrase)) < 6 {
				continue
			}
			lookups++
			hits, _, err := names.Find(ds, names.ItemSets, phrase, maxSetsAsked)
			if err != nil {
				continue
			}
			for _, h := range hits {
				if k := names.Normalize(phrase); names.Normalize(h.Name) == k || strings.HasPrefix(names.Normalize(h.Name), k+" ") {
					add(h.ID)
				}
			}
		}
	}
	var lines []string
	for _, id := range ids {
		rows, err := ds.Filter("item_sets", store.Range{Column: "set_id", Min: id, Max: id})
		if err != nil || len(rows) == 0 {
			continue
		}
		var pieces []string
		missing := 0
		for _, r := range rows {
			it, err := ds.Get("items", num(r["item_id"]))
			if err != nil || it == nil || text0(it["name"]) == "" {
				missing++
				continue
			}
			piece := text0(it["name"])
			if d := gear.PieceText(it); d != "" {
				piece += " (" + d + ")"
			}
			pieces = append(pieces, piece)
		}
		sort.Strings(pieces)
		line := fmt.Sprintf("- %s (set %d): %d pieces in the dataset", text0(rows[0]["name"]), id, len(rows))
		if len(pieces) > 0 {
			line += ": " + strings.Join(pieces, ", ")
		}
		if missing > 0 {
			line += fmt.Sprintf("; %d of its items are not in the items table, so they cannot be named or judged", missing)
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		return ""
	}
	return "Item sets (from the item_sets table; answer from these, not from memory; the dataset holds no set bonus, piece-count threshold or tier, so state none):\n" + strings.Join(lines, "\n")
}

func num(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}

func text0(v any) string { s, _ := v.(string); return s }
