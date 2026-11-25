package stats

import (
	"sort"

	"github.com/WoW-Companion/advisor/internal/store"
)

// Stat type ids as the dataset holds them (see internal/gear).
const (
	idAgility, idStrength, idIntellect, idSpirit, idStamina = 3, 4, 5, 6, 7
)

// Suffix is a random stat suffix of world-drop gear: its name, the stats it gives (the client's stat
// type ids) and the words for them. This is a curated game-fact table (the dataset holds item names
// and stat ids, not the suffix list); a suffix is checked against the dataset's random_suffixes table when
// there is one (see Verified).
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

// RankSuffixes orders the suffixes for a priority: only suffixes that carry the priority's lead stat and
// no stat the priority leaves out (a weight under fineWeight: Spirit for a Destruction Warlock), best
// first by the sum of the weights of the stats they give (so Stamina + Intellect beats Intellect + Spirit
// when Stamina outweighs Spirit, and a pair beats the single stat). A tie keeps the table order.
func (p Prof) RankSuffixes() []Suffix {
	lead := p.top()
	var out []Suffix
	score := map[string]float64{}
	for _, s := range Suffixes {
		has, sum, useless := false, 0.0, false
		for _, id := range s.Stats {
			has = has || StatNames[id] == lead
			sum += p.Weights[StatNames[id]]
			useless = useless || p.Weights[StatNames[id]] < fineWeight
		}
		if has && !useless {
			out = append(out, s)
			score[s.Key] = sum
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return score[out[i].Key] > score[out[j].Key] })
	return out
}

// SuffixRows is the dataset's random_suffixes table, or nil when the dataset has none (or no dataset):
// then the curated table above is trusted as it stands.
func SuffixRows(ds store.DataStore) []store.Row {
	if ds == nil {
		return nil
	}
	rows, err := ds.Filter("random_suffixes", store.Range{Column: "suffix_id", Min: int64(0), Max: int64(1) << 31})
	if err != nil {
		return nil
	}
	return rows
}

func asInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int16:
		return int64(n), true
	case int:
		return int64(n), true
	}
	return 0, false
}

// suffixSets groups the random_suffixes rows named name by suffix id into the set of stat ids each one
// gives. The dataset holds one row per stat of a suffix (the stat column a number), though a row whose
// stat column is a list is read too.
func suffixSets(rows []store.Row, name string) map[int64]map[int64]bool {
	out := map[int64]map[int64]bool{}
	for _, r := range rows {
		if n, _ := r["name"].(string); n != name {
			continue
		}
		id, _ := asInt(r["suffix_id"])
		if out[id] == nil {
			out[id] = map[int64]bool{}
		}
		if l, ok := r["stats"].([]any); ok {
			for _, v := range l {
				if n, ok := asInt(v); ok {
					out[id][n] = true
				}
			}
		} else if n, ok := asInt(r["stats"]); ok {
			out[id][n] = true
		}
	}
	return out
}

// Verified reports whether the random_suffixes rows hold the suffix with exactly the stats it says.
// With no rows the curated table is trusted. Item names are never matched: elixirs and scrolls share
// the words.
func Verified(s Suffix, rows []store.Row) bool {
	if len(rows) == 0 {
		return true
	}
	for _, have := range suffixSets(rows, s.Name) {
		if len(have) != len(s.Stats) {
			continue
		}
		all := true
		for _, st := range s.Stats {
			all = all && have[st]
		}
		if all {
			return true
		}
	}
	return false
}

// VerifiedTop is the top n suffixes for the priority that the random_suffixes rows confirm.
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
