package gear

import (
	"fmt"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/stats"
)

// captureStatNames maps the addon's short stat keys to the stats package's names; any other key
// keeps its own name.
var captureStatNames = map[string]string{
	"agi": stats.Agility, "str": stats.Strength, "sta": stats.Stamina, "int": stats.Intellect,
	"spi": stats.Spirit, "armor": stats.Armor,
}

// capturedStats renames the addon's stat keys; nil for none.
func capturedStats(m map[string]float64) map[string]float64 {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]float64, len(m))
	for k, v := range m {
		if n, ok := captureStatNames[k]; ok {
			k = n
		}
		out[k] += v
	}
	return out
}

// namedWeights are the class/spec weights by stat name, armor included when the class weighs it.
func namedWeights(c character.Character, group string) map[string]float64 {
	if p, ok := stats.For(c, group); ok {
		return p.ItemWeights()
	}
	if _, w, _, ok := Weights(c.Class, group); ok {
		byName := map[string]float64{}
		for name, id := range statIDs {
			if v, ok := w[id]; ok {
				byName[name] = v
			}
		}
		return byName
	}
	return nil
}

// statMap is the item's stats by name: the captured ones first, else the dataset's amounts; nil when
// neither says how much of each stat the item has.
func (it Item) statMap() map[string]float64 {
	if len(it.Captured) > 0 {
		return it.Captured
	}
	if !it.StatsKnown || len(it.Stats) == 0 {
		return nil
	}
	out := map[string]float64{}
	for name, id := range statIDs {
		if n, ok := it.Stats[id]; ok {
			out[name] = float64(n)
		}
	}
	return out
}

// statSum is the sum of weight times amount over the item's stats.
func statSum(m, w map[string]float64) float64 {
	var s float64
	for k, v := range m {
		s += w[k] * v
	}
	return s
}

// enchantNote says an enchant's stats were left out: the dataset has no enchant table.
func enchantNote(it Item, where string) []string {
	if it.Enchant == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%s enchant %d ignored: not in the dataset", where, it.Enchant)}
}

// fallbackValue orders items with no stats: item level first, then quality only to break a tie at
// equal item level, then required level.
func fallbackValue(it Item) float64 {
	return float64(it.ItemLevel)*1e6 + float64(it.Quality)*1e3 + float64(it.ReqLevel)
}
