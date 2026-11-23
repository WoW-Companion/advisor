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
	itemdata "github.com/WoW-Companion/advisor/internal/items"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/stats"
	"github.com/WoW-Companion/advisor/internal/store"
)

// EquipWindow is how many levels above the character an item may require (known or estimated).
const EquipWindow = 2

// EstimateOffset is how far below its item level an item's required level is estimated to sit when
// the dataset holds 0 (unknown) for it: required level ~ max(1, item level - EstimateOffset).
const EstimateOffset = 5

// EstimateRequiredLevel is the required level assumed for an item whose dataset value is unknown.
func EstimateRequiredLevel(itemLevel int) int { return max(1, itemLevel-EstimateOffset) }

// EstimateGap is how far the estimate may exceed a known required level before the known one is
// distrusted: the dataset holds required level 1 for about 80 equippable items of item level 30 to
// 70 (scaling and event items), which are no use to a level-5 character. Real level-60 gear stays
// within it (item level 83 needs 60).
const EstimateGap = 20

// unequippableBonding are the bonding values of quest items (the client's bonding 4 and 5).
func unequippableBonding(b int) bool { return b == 4 || b == 5 }

// qualityArtifact is the quality no ordinary character equips as an upgrade.
const qualityArtifact = 6

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

// HasStat reports whether the item carries the named stat (a name of the stats package).
func (it Item) HasStat(stat string) bool {
	id, ok := statIDs[stat]
	if !ok {
		return false
	}
	for _, t := range it.StatTypes {
		if t == id {
			return true
		}
	}
	return false
}

// idWeights maps stat names to the client's item stat type ids; a name that is not an item stat is
// left out.
func idWeights(byName map[string]float64) map[int64]float64 {
	w := map[int64]float64{}
	for stat, v := range byName {
		if id, ok := statIDs[stat]; ok {
			w[id] = v
		}
	}
	return w
}

// Item is the part of an items row the advisor uses.
type Item struct {
	ID            int64
	Name          string
	ItemLevel     int
	RequiredLevel int // the dataset's; 0 is unknown
	ReqLevel      int // what the search used: RequiredLevel, or an estimate when that is 0
	Estimated     bool
	Quality       int
	Slot          int
	Bonding       int
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
	Soon    bool    // the item's ReqLevel is above the character's level (within EquipWindow)
	Sources []Source
}

// Options narrows a search. The zero value searches every slot and keeps every candidate.
type Options struct {
	Slot  string // one capture slot name; empty for all
	Limit int    // most candidates per slot, best first; 0 for all
	// NoSources skips the per-candidate source lookup, for a caller that finds its own.
	NoSources bool
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
		Quality: int(i64(row["quality"])), Slot: int(i64(row["slot"])), Bonding: int(i64(row["bonding"])),
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
	if p, ok := stats.For(c, group); ok {
		// The player's play style: the stat compass's weights, shared with the stat check and the
		// Auction House suffixes, so the advice agrees everywhere.
		res.Spec, res.Rationale, w = p.Label(), "Ballpark stat priorities for "+p.Title(), idWeights(p.Weights)
	} else if spec, ww, why, ok := Weights(c.Class, group); ok {
		res.Spec, res.Rationale, w = spec, why, ww
	} else if group == "" {
		res.Reason = "spec unknown (no talent points captured)"
	} else {
		res.Reason = fmt.Sprintf("no stat weights for %q", group)
	}

	level := *c.Level
	rows, err := ds.Filter("items", store.Range{Column: "required_level", Min: 0, Max: level + EquipWindow})
	if err != nil {
		return res, fmt.Errorf("items table: %w", err)
	}
	bySlot := map[string][]Item{}
	anyStats := false
	for _, row := range rows {
		it := itemOf(row)
		if itemdata.Unobtainable(it.Name) || it.ItemLevel <= 0 || it.Quality <= 0 || it.Quality == qualityArtifact || unequippableBonding(it.Bonding) {
			continue
		}
		if !rl.canUse(int8(i64(row["class_id"])), int8(i64(row["subclass_id"])), int8(it.Slot), level) ||
			!allowsClass(int32(i64(row["allowable_class_mask"])), rl.ID) {
			continue
		}
		// A required level of 0 is what the client table holds for many high-level items (the real
		// dataset has 2,330 equippable ones, up to item level 83), so 0 is "unknown", not "none":
		// the level is estimated from the item level and put through the same window.
		it.ReqLevel = it.RequiredLevel
		if est := EstimateRequiredLevel(it.ItemLevel); it.RequiredLevel == 0 || est-it.RequiredLevel > EstimateGap {
			it.ReqLevel, it.Estimated = est, true
		}
		if it.ReqLevel > level+EquipWindow {
			continue
		}
		anyStats = anyStats || it.StatsKnown
		for _, slot := range rl.fitsSlots(int8(it.Slot)) {
			if wanted(opt.Slot, slot) {
				bySlot[slot] = append(bySlot[slot], it)
			}
		}
	}
	if !anyStats && res.Reason == "" {
		res.Reason = "the dataset has no stats for these items"
	}

	ranked := map[string][]Candidate{}
	for slot, items := range bySlot {
		ranked[slot] = rank(c, ds, slot, items, w)
	}
	for _, pair := range pairedSlots {
		dedupe(ranked, c, pair[0], pair[1], opt.Limit)
	}
	for slot, cands := range ranked {
		if opt.Slot != "" && opt.Slot != slot {
			continue
		}
		if opt.Limit > 0 && len(cands) > opt.Limit {
			cands = cands[:opt.Limit]
		}
		for i := range cands {
			if !opt.NoSources {
				cands[i].Sources = sources(ds, cands[i].Item.ID, level)
			}
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

// pairedSlots are the capture slots that hold two of the same kind of item.
var pairedSlots = [][2]string{{"finger1", "finger2"}, {"trinket1", "trinket2"}}

// wanted reports whether slot must be searched for a request for only (empty for all): the slot
// itself, and the other of its pair, which the duplicate rule needs.
func wanted(only, slot string) bool {
	if only == "" || only == slot {
		return true
	}
	for _, p := range pairedSlots {
		if only == p[0] && slot == p[1] || only == p[1] && slot == p[0] {
			return true
		}
	}
	return false
}

// dedupe makes the two slots of a pair never offer the same item id: the first slot keeps its
// candidates, and the second takes the next ones instead. Neither offers the item the other wears.
func dedupe(ranked map[string][]Candidate, c character.Character, a, b string, limit int) {
	worn := func(slot string) int64 {
		if eq, ok := c.Equipment[slot]; ok && eq.ItemID != nil {
			return int64(*eq.ItemID)
		}
		return 0
	}
	first := ranked[a]
	taken := map[int64]bool{}
	if id := worn(b); id != 0 {
		taken[id] = true
	}
	var kept []Candidate
	for _, cd := range first {
		if !taken[cd.Item.ID] {
			kept = append(kept, cd)
		}
	}
	ranked[a] = kept
	taken = map[int64]bool{}
	if id := worn(a); id != 0 {
		taken[id] = true
	}
	for i, cd := range kept {
		if limit == 0 || i < limit {
			taken[cd.Item.ID] = true
		}
	}
	var second []Candidate
	for _, cd := range ranked[b] {
		if !taken[cd.Item.ID] {
			second = append(second, cd)
		}
	}
	ranked[b] = second
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
		// An item that is not above what is worn, or a placeholder for an empty slot, is no upgrade.
		if worn && it.ItemLevel <= cur.ItemLevel || !worn && it.ItemLevel <= 1 {
			continue
		}
		cand := Candidate{Item: it, Slot: slot, Basis: BasisItemLevel, Empty: !worn}
		cand.Soon = it.ReqLevel > *c.Level
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
