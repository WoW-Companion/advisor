package ask

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/store"
)

// disenchantAsk is a question about disenchanting an item: whether it can be, and what it gives.
var disenchantAsk = regexp.MustCompile(`(?i)\bdis-?enchant\w*|\bde'?(?:d|ing)\b|\bshatter\w*\b|\b(?:turn|break|crack)\w*\b.{0,20}\binto\b.{0,20}\b(?:dust|essences?|shards?)\b`)

var (
	disenchantClass   = map[int64]string{2: "weapon", 4: "armor"}
	disenchantQuality = map[int64]string{2: "uncommon", 3: "rare", 4: "epic"}
)

// DisenchantFacts is what disenchanting gives for each item the question links by [name], from the item row and the dataset's disenchant bands (item
// class, quality and item level): the materials with their chance and count. An item that is not armor or a weapon of uncommon quality or better cannot be
// disenchanted; an item no band covers, or none the dataset rates, gets no material from memory. "" for a question that is not about disenchanting.
func DisenchantFacts(ds store.DataStore, text string) string {
	if ds == nil || !disenchantAsk.MatchString(text) {
		return ""
	}
	bands, err := ds.Filter("disenchant_yields", store.Range{Column: "band_id", Min: int64(0)})
	if err != nil {
		bands = nil
	}
	lines := []string{"Disenchanting (from the item rows and the disenchant bands; answer from these, not from memory): " + bandCoverage(bands) +
		"; the data holds no Enchanting skill needed, and no flag for the few items the game will not disenchant"}
	for _, m := range itemInBrackets.FindAllStringSubmatch(text, maxLootItems) {
		name := strings.TrimSpace(m[1])
		hits, _, err := names.Find(ds, names.Items, name, 1)
		var row store.Row
		if err == nil && len(hits) == 1 && strings.EqualFold(hits[0].Name, name) {
			row, _ = ds.Get("items", hits[0].ID)
		}
		lines = append(lines, "- "+name+": "+disenchantLine(ds, bands, row))
	}
	return strings.Join(lines, "\n")
}

// bandCoverage says which qualities and item levels the dataset's rated bands cover.
func bandCoverage(bands []store.Row) string {
	type span struct{ lo, hi int64 }
	cover := map[string]span{}
	for _, b := range bands {
		if str(b["state"]) == "none" {
			continue
		}
		k := str(b["quality"]) + " " + str(b["item_class"])
		s, ok := cover[k]
		lo, hi := i64(b["min_item_level"]), i64(b["max_item_level"])
		if !ok || lo < s.lo {
			s.lo = lo
		}
		if !ok || hi > s.hi {
			s.hi = hi
		}
		cover[k] = s
	}
	if len(cover) == 0 {
		return "the dataset holds no disenchant band, so what an item gives is not known"
	}
	var parts []string
	for k, s := range cover {
		parts = append(parts, fmt.Sprintf("%s (item level %d to %d)", k, s.lo, s.hi))
	}
	sort.Strings(parts)
	return "only uncommon or better armor and weapons can be disenchanted; the dataset's bands cover " + strings.Join(parts, ", ") + " and nothing else"
}

func disenchantLine(ds store.DataStore, bands []store.Row, row store.Row) string {
	if row == nil {
		return "the dataset has no row for this item, so whether it can be disenchanted and what it gives are not known"
	}
	class, quality := i64(row["class_id"]), i64(row["quality"])
	if disenchantClass[class] == "" || quality < 2 {
		return "cannot be disenchanted: only armor and weapons of uncommon quality or better can be"
	}
	if quality > 4 {
		return "the dataset has no disenchant band for a legendary item, so what it gives is not known"
	}
	il := i64(row["item_level"])
	type mat struct {
		name           string
		chance, per    float64
		minCnt, maxCnt int64
	}
	var mats []mat
	state := ""
	for _, b := range bands {
		if str(b["item_class"]) != disenchantClass[class] || str(b["quality"]) != disenchantQuality[quality] || il < i64(b["min_item_level"]) || il > i64(b["max_item_level"]) || str(b["state"]) == "none" {
			continue
		}
		state = str(b["state"])
		name := fmt.Sprintf("item %d", i64(b["material_item_id"]))
		if it, err := ds.Get("items", i64(b["material_item_id"])); err == nil && str(it["name"]) != "" {
			name = str(it["name"])
		}
		mats = append(mats, mat{name, f64(b["chance"]), f64(b["per_disenchant"]), i64(b["min_count"]), i64(b["max_count"])})
	}
	if len(mats) == 0 {
		return fmt.Sprintf("can be disenchanted (%s %s, item level %d), but the dataset has no rated band for it, so what it gives is not known; say so and name no material", disenchantQuality[quality], disenchantClass[class], il)
	}
	sort.Slice(mats, func(i, j int) bool { return mats[i].chance > mats[j].chance })
	var parts []string
	for _, m := range mats {
		parts = append(parts, fmt.Sprintf("%s %.0f%% of the time (%d to %d)", m.name, m.chance*100, m.minCnt, m.maxCnt))
	}
	return fmt.Sprintf("can be disenchanted (%s %s, item level %d), giving %s; the band is %s", disenchantQuality[quality], disenchantClass[class], il, strings.Join(parts, "; "), state)
}
