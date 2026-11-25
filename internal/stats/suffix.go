package stats

import (
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/store"
)

// Stat type ids as the dataset holds them (see internal/gear).
const (
	idAgility, idStrength, idIntellect, idSpirit, idStamina = 3, 4, 5, 6, 7
)

// Suffix is a random stat suffix of world-drop gear: its name, the stats it gives (the client's stat
// type ids) and the words for them. This is a curated game-fact table (the dataset holds item names
// and stat ids, not the suffix list); a suffix is only offered when the dataset holds an item with that
// name, and with those stats where the item's stats are known (see Verified).
type Suffix struct {
	Key   string
	Name  string
	Stats []int64
	Words string
}

// Suffixes is every suffix the advice can name, in the order a tie falls back to.
var Suffixes = []Suffix{
	{"eagle", "of the Eagle", []int64{idStamina, idIntellect}, "Stamina + Intellect"},
	{"owl", "of the Owl", []int64{idIntellect, idSpirit}, "Intellect + Spirit"},
	{"whale", "of the Whale", []int64{idStamina, idSpirit}, "Stamina + Spirit"},
	{"bear", "of the Bear", []int64{idStrength, idStamina}, "Strength + Stamina"},
	{"tiger", "of the Tiger", []int64{idStrength, idAgility}, "Strength + Agility"},
	{"monkey", "of the Monkey", []int64{idAgility, idStamina}, "Agility + Stamina"},
	{"falcon", "of the Falcon", []int64{idAgility, idIntellect}, "Agility + Intellect"},
	{"wolf", "of the Wolf", []int64{idAgility, idSpirit}, "Agility + Spirit"},
	{"boar", "of the Boar", []int64{idStrength, idSpirit}, "Strength + Spirit"},
	{"gorilla", "of the Gorilla", []int64{idStrength, idIntellect}, "Strength + Intellect"},
	{"strength", "of Strength", []int64{idStrength}, "Strength"},
	{"agility", "of Agility", []int64{idAgility}, "Agility"},
	{"stamina", "of Stamina", []int64{idStamina}, "Stamina"},
	{"intellect", "of Intellect", []int64{idIntellect}, "Intellect"},
	{"spirit", "of Spirit", []int64{idSpirit}, "Spirit"},
}

// SuffixByKey is the suffix with the key, "eagle".
func SuffixByKey(key string) (Suffix, bool) {
	for _, s := range Suffixes {
		if s.Key == key {
			return s, true
		}
	}
	return Suffix{}, false
}

// StatNames maps the dataset's stat type ids to stat names.
var StatNames = map[int64]string{idAgility: Agility, idStrength: Strength, idIntellect: Intellect, idSpirit: Spirit, idStamina: Stamina}

// RankSuffixes orders the suffixes for a priority: only suffixes that carry the priority's lead stat, best first
// by the sum of the weights of the stats they give (so Stamina + Intellect beats Intellect + Spirit
// when Stamina outweighs Spirit, and a pair beats the single stat). A tie keeps the table order.
func (p Prof) RankSuffixes() []Suffix {
	lead := p.top()
	var out []Suffix
	score := map[string]float64{}
	for _, s := range Suffixes {
		has, sum := false, 0.0
		for _, id := range s.Stats {
			has = has || StatNames[id] == lead
			sum += p.Weights[StatNames[id]]
		}
		if has {
			out = append(out, s)
			score[s.Key] = sum
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return score[out[i].Key] > score[out[j].Key] })
	return out
}

// Verified reports whether the dataset rows hold an item with the suffix in its name (and, where that
// item's stats are known, the stats the suffix says). A suffix nothing in the dataset confirms is left
// out rather than trusted.
func Verified(s Suffix, rows []store.Row) bool {
	for _, r := range rows {
		name, _ := r["name"].(string)
		if !strings.HasSuffix(name, " "+s.Name) {
			continue
		}
		if r["stats_status"] != "known" {
			return true
		}
		have := map[int64]bool{}
		if l, ok := r["stat_types"].([]any); ok {
			for _, v := range l {
				if n, ok := v.(int64); ok {
					have[n] = true
				}
			}
		}
		ok := true
		for _, st := range s.Stats {
			ok = ok && have[st]
		}
		if ok {
			return true
		}
	}
	return false
}

// VerifiedTop is the top n suffixes for the priority that the dataset rows confirm.
func (p Prof) VerifiedTop(rows []store.Row, n int) []Suffix {
	var out []Suffix
	for _, s := range p.RankSuffixes() {
		if len(out) == n {
			break
		}
		if Verified(s, rows) {
			out = append(out, s)
		}
	}
	return out
}
