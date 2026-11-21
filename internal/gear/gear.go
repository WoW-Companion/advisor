// Package gear finds equipment upgrades for a captured character from the dataset's items and
// loot_sources tables. The rules are in docs/gear.md.
//
// The dataset records which stat types an item has, not their sizes. With stat types known, an
// item is scored from its item level scaled by how much the spec's weights value those types;
// otherwise from item level and quality alone, and the answer says so.
package gear

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// EquipWindow is how many levels above the character an item may require.
const EquipWindow = 3

// Source level window: a creature is "in range" when its levels reach into
// [level-SourceBelow, level+SourceAbove].
const (
	SourceBelow = 5
	SourceAbove = 3
)

// MaxSources is how many sources a candidate lists.
const MaxSources = 3

// Basis of a candidate's score.
const (
	BasisStats     = "stats"
	BasisItemLevel = "item level"
)

//go:embed weights.json
var weightsJSON []byte

// statIDs maps the weights file's names to the client's item stat type ids.
var statIDs = map[string]int64{
	"agility": 3, "strength": 4, "intellect": 5, "spirit": 6, "stamina": 7, "hit": 31, "crit": 32,
	"haste": 36, "expertise": 37, "attack_power": 38, "versatility": 40, "mp5": 43, "spell_power": 45,
	"mastery": 49,
}

type specWeights struct {
	Rationale string             `json:"rationale"`
	Weights   map[string]float64 `json:"weights"`
}

type weightsFile struct {
	Note  string                 `json:"note"`
	Specs map[string]specWeights `json:"specs"`
}

var weights = func() weightsFile {
	var w weightsFile
	if err := json.Unmarshal(weightsJSON, &w); err != nil {
		panic("gear: weights.json: " + err.Error())
	}
	return w
}()

// Weights returns the advisor's default weights for a class token ("PRIEST") and a talent group
// name ("Test Discipline" matches "Discipline"); ok is false when none apply.
func Weights(class, group string) (spec string, w map[int64]float64, rationale string, ok bool) {
	class = strings.ToUpper(class)
	g := strings.ToLower(group)
	for key, sw := range weights.Specs {
		c, name, _ := strings.Cut(key, "/")
		if c != class || !strings.Contains(g, strings.ToLower(name)) {
			continue
		}
		w = map[int64]float64{}
		for stat, v := range sw.Weights {
			w[statIDs[stat]] = v
		}
		return name, w, sw.Rationale, true
	}
	return "", nil, "", false
}

// Item is the part of an items row the advisor uses.
type Item struct {
	ID            int64
	Name          string
	ItemLevel     int
	RequiredLevel int
	Quality       int
	Slot          int
	StatTypes     []int64
	StatsKnown    bool
}

// Source is one place an item drops or is rewarded. Level and chance fields are nil when unknown.
type Source struct {
	Kind string // creature, object, quest reward
	ID   int64
	Name string // empty when the dataset has no name for it
	// Everloot is true when the name comes from Everloot's drop rows (npcs.name_source).
	Everloot bool
	MinLevel *int
	MaxLevel *int
	Chance   *float64 // percent, from drops / kills
	InRange  bool     // a creature whose levels reach the character's level range
}

// Candidate is an item that beats what the character wears in a slot.
type Candidate struct {
	Item    Item
	Slot    string
	Score   float64
	Delta   float64 // score minus the equipped item's; the full score for an empty slot
	Basis   string  // BasisStats or BasisItemLevel
	Empty   bool    // the slot is empty
	Sources []Source
}

// Options narrows a search. The zero value searches every slot and keeps every candidate.
type Options struct {
	Slot  string // one capture slot name; empty for all
	Limit int    // most candidates per slot, best first; 0 for all
}

// Result is what a search found.
type Result struct {
	Slots map[string][]Candidate
	// ItemLevelOnly is true when no candidate was scored from stats.
	ItemLevelOnly bool
	Spec          string // spec the weights are for; empty when none apply
	Rationale     string
	Reason        string // why scoring fell back to item level, when it did
}

// Upgrades returns, per capture slot, the items the character can use that beat what they wear,
// best first. group is the character's top talent group name; empty when unknown.
func Upgrades(c character.Character, ds store.DataStore, group string) (map[string][]Candidate, error) {
	r, err := Find(c, ds, group, Options{})
	return r.Slots, err
}

func i64(v any) int64 { n, _ := v.(int64); return n }

func itemOf(row store.Row) Item {
	it := Item{
		ID: i64(row["id"]), ItemLevel: int(i64(row["item_level"])), RequiredLevel: int(i64(row["required_level"])),
		Quality: int(i64(row["quality"])), Slot: int(i64(row["slot"])),
		StatsKnown: row["stats_status"] == "known",
	}
	it.Name, _ = row["name"].(string)
	if l, ok := row["stat_types"].([]any); ok {
		for _, v := range l {
			it.StatTypes = append(it.StatTypes, i64(v))
		}
	}
	return it
}

// score is the item's value; byStats needs it.StatsKnown and weights.
func score(it Item, w map[int64]float64, byStats bool) float64 {
	s := float64(it.Quality)
	if !byStats {
		return float64(it.ItemLevel) + s
	}
	if len(it.StatTypes) == 0 {
		return float64(it.ItemLevel)*0.5 + s
	}
	sum := 0.0
	for _, t := range it.StatTypes {
		sum += w[t]
	}
	return float64(it.ItemLevel)*(0.5+0.5*sum/float64(len(it.StatTypes))) + s
}

// Find is Upgrades with options and the basis of the answer.
func Find(c character.Character, ds store.DataStore, group string, opt Options) (Result, error) {
	res := Result{Slots: map[string][]Candidate{}, ItemLevelOnly: true}
	if ds == nil {
		return res, errors.New("no dataset configured")
	}
	if c.Level == nil {
		return res, errors.New("the capture has no character level")
	}
	rl, ok := rules[strings.ToUpper(c.Class)]
	if !ok {
		return res, fmt.Errorf("no gear rules for class %q yet", c.Class)
	}
	var w map[int64]float64
	if spec, ww, why, ok := Weights(c.Class, group); ok {
		res.Spec, res.Rationale, w = spec, why, ww
	} else if group == "" {
		res.Reason = "spec unknown (no talent points captured)"
	} else {
		res.Reason = fmt.Sprintf("no stat weights for %q", group)
	}

	rows, err := ds.Filter("items", store.Range{Column: "required_level", Min: 0, Max: *c.Level + EquipWindow})
	if err != nil {
		return res, fmt.Errorf("items table: %w", err)
	}
	bySlot := map[string][]Item{}
	anyStats := false
	for _, row := range rows {
		it := itemOf(row)
		if it.ItemLevel <= 0 || it.Quality <= 0 {
			continue
		}
		if !rl.canUse(int8(i64(row["class_id"])), int8(i64(row["subclass_id"])), int8(it.Slot)) ||
			!allowsClass(int32(i64(row["allowable_class_mask"])), rl.ID) {
			continue
		}
		// A required level of 0 is what the client table holds for many high-level items (the real
		// dataset has 2,330 equippable ones, up to item level 83), so 0 is "unknown", not "none":
		// offer such an item only when its item level is within the window itself.
		if it.RequiredLevel == 0 && it.ItemLevel > *c.Level+EquipWindow {
			continue
		}
		anyStats = anyStats || it.StatsKnown
		for _, slot := range rl.fitsSlots(int8(it.Slot)) {
			if opt.Slot == "" || opt.Slot == slot {
				bySlot[slot] = append(bySlot[slot], it)
			}
		}
	}
	if !anyStats && res.Reason == "" {
		res.Reason = "the dataset has no stats for these items"
	}

	for slot, items := range bySlot {
		cands := rank(c, ds, slot, items, w)
		if opt.Limit > 0 && len(cands) > opt.Limit {
			cands = cands[:opt.Limit]
		}
		for i := range cands {
			cands[i].Sources = sources(ds, cands[i].Item.ID, *c.Level)
			if cands[i].Basis == BasisStats {
				res.ItemLevelOnly = false
			}
		}
		if len(cands) > 0 {
			res.Slots[slot] = cands
		}
	}
	if res.ItemLevelOnly && res.Reason == "" {
		res.Reason = "no candidate had stats to weigh"
	}
	return res, nil
}

// rank scores the items against what the character wears in slot and keeps the upgrades.
func rank(c character.Character, ds store.DataStore, slot string, items []Item, w map[int64]float64) []Candidate {
	eq, worn := c.Equipment[slot]
	worn = worn && (eq.ItemID != nil && *eq.ItemID != 0 || eq.ItemLevel != nil)
	var cur Item
	curKnown := false
	if worn {
		if eq.ItemID != nil && *eq.ItemID != 0 {
			if row, err := ds.Get("items", int64(*eq.ItemID)); err == nil {
				cur, curKnown = itemOf(row), true
			}
		}
		if eq.ItemLevel != nil {
			cur.ItemLevel = *eq.ItemLevel
		}
		if eq.Quality != nil {
			cur.Quality = *eq.Quality
		}
	}
	var out []Candidate
	for _, it := range items {
		byStats := w != nil && it.StatsKnown && (!worn || curKnown && cur.StatsKnown)
		cand := Candidate{Item: it, Slot: slot, Basis: BasisItemLevel, Empty: !worn}
		if byStats {
			cand.Basis = BasisStats
		}
		cand.Score = score(it, w, byStats)
		cand.Delta = cand.Score
		if worn {
			cand.Delta -= score(cur, w, byStats)
			if cand.Delta <= 0 {
				continue
			}
		}
		out = append(out, cand)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Delta != out[j].Delta {
			return out[i].Delta > out[j].Delta
		}
		return out[i].Item.ID < out[j].Item.ID
	})
	return out
}

// FromEverloot is true when an npcs.name_source value (nil on a client row) says Everloot named the
// creature.
func FromEverloot(v any) bool {
	s, _ := v.(string)
	return strings.Contains(strings.ToLower(s), "everloot")
}

// sources lists where an item comes from, creatures in the character's level range first, then by
// drop chance. A missing table or no rows gives none.
func sources(ds store.DataStore, itemID int64, level int) []Source {
	rows, err := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: itemID, Max: itemID})
	if err != nil {
		return nil
	}
	var out []Source
	for _, row := range rows {
		s := Source{Kind: fmt.Sprint(row["source_kind"]), ID: i64(row["source_id"])}
		if kills := i64(row["kills"]); kills > 0 {
			p := 100 * float64(i64(row["drops"])) / float64(kills)
			s.Chance = &p
		}
		if s.Kind == "creature" {
			if n, err := ds.Get("npcs", s.ID); err == nil {
				s.Name, _ = n["name"].(string)
				s.Everloot = FromEverloot(n["name_source"])
				if v, ok := n["min_level"].(int64); ok {
					l := int(v)
					s.MinLevel = &l
				}
				if v, ok := n["max_level"].(int64); ok {
					l := int(v)
					s.MaxLevel = &l
				}
				lo, hi := s.MinLevel, s.MaxLevel
				if lo == nil {
					lo = hi
				}
				if hi == nil {
					hi = lo
				}
				s.InRange = lo != nil && *hi >= level-SourceBelow && *lo <= level+SourceAbove
			}
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.InRange != b.InRange {
			return a.InRange
		}
		if (a.Chance != nil) != (b.Chance != nil) {
			return a.Chance != nil
		}
		if a.Chance != nil && *a.Chance != *b.Chance {
			return *a.Chance > *b.Chance
		}
		return a.ID < b.ID
	})
	if len(out) > MaxSources {
		out = out[:MaxSources]
	}
	return out
}
