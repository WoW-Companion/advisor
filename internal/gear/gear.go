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
	"github.com/WoW-Companion/advisor/internal/vendors"
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
	// The older gear stats a Classic item carries: bonus healing and damage (items from before Spell Power),
	// the defence ratings of a tank's gear, and the armor a green item adds on top of its base armor.
	"spell_healing": 41, "spell_damage": 42, "dodge": 13, "parry": 14, "block": 15, "extra_armor": 50,
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
	Class         int // the items table's class_id and subclass_id (2 is a weapon)
	Subclass      int
	Bonding       int
	StatTypes     []int64
	// Armor is the item's armor, the dataset's `armor` column (item level, armor type, quality and slot decide
	// it); 0 when there is none.
	Armor int
	// Stats is the amount of each stat (type id to amount) from the dataset's stat values; nil when the
	// dataset has types only.
	Stats      map[int64]int `json:"-"`
	StatsKnown bool
	// Captured is the item's own stats by name (agility, stamina, armor, ...) as the addon recorded
	// them, enchants not included; nil when none were captured.
	Captured map[string]float64 `json:"-"`
	// Enchant is the enchant id on the item, 0 for none or unknown.
	Enchant int `json:"-"`
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
	// Vendor is the cheapest vendor that sells the item, from Everloot's vendor offers; nil when none.
	Vendor *vendors.Offer `json:"vendor,omitempty"`
	// Craft is whether the character's skill can make the item; nil when no recipe makes it.
	Craft *Craft `json:"craft,omitempty"`
	// Need is the weapon skill the item needs and the character can learn but has not; nil otherwise.
	Need *SkillNeed `json:"need,omitempty"`
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
	// Bags are the items already in the bags that beat what is worn: free, so they come first.
	Bags []BagUpgrade `json:"bags,omitempty"`
	// Unknown maps a slot to the name of the worn item the dataset does not hold and the capture gave no item level for:
	// nothing is compared there.
	Unknown map[string]string `json:"unknown,omitempty"`
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
		Quality: int(i64(row["quality"])), Slot: int(i64(row["slot"])), Class: int(i64(row["class_id"])), Subclass: int(i64(row["subclass_id"])), Bonding: int(i64(row["bonding"])),
		StatsKnown: row["stats_status"] == "known",
	}
	it.Name, _ = row["name"].(string)
	if l, ok := row["stat_types"].([]any); ok {
		for _, v := range l {
			it.StatTypes = append(it.StatTypes, i64(v))
		}
	}
	it.Stats = stats.ItemStats(row)
	it.Armor = int(i64(row["armor"]))
	return it
}

// score is the item's value; byStats needs it.StatsKnown and weights.
func score(it Item, w map[int64]float64, byStats bool) float64 {
	// Quality only breaks a tie at equal item level (it never adds up to a whole item level), and
	// never counts toward a stat score.
	tie := float64(it.Quality) * 0.01
	if !byStats {
		return float64(it.ItemLevel) + tie
	}
	if len(it.StatTypes) == 0 {
		return float64(it.ItemLevel) * 0.5
	}
	// With the amounts known, each stat counts as much as the item has of it: an item that is mostly
	// the stat the build wants fits better than one with a token amount of it. With types only, each
	// type counts once.
	var sum, total float64
	if len(it.Stats) > 0 {
		for t, n := range it.Stats {
			sum += w[t] * float64(n)
			total += float64(n)
		}
	} else {
		for _, t := range it.StatTypes {
			sum += w[t]
		}
		total = float64(len(it.StatTypes))
	}
	return float64(it.ItemLevel) * (0.5 + 0.5*sum/total)
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
	// Dual Wield is only offered once it is learned (see weapons.go).
	rl.DualWield = rl.DualWield && knowsDualWield(c, ds, strings.ToUpper(c.Class))
	var w map[int64]float64
	if p, ok := stats.For(c, group); ok {
		// The player's play style: the stat compass's weights, shared with the stat check and the
		// Auction House suffixes, so the advice agrees everywhere.
		res.Spec, res.Rationale, w = p.Label(), "Ballpark stat priorities for "+p.Title(), idWeights(p.ItemWeights())
	} else if spec, ww, why, ok := Weights(c.Class, group); ok {
		res.Spec, res.Rationale, w = spec, why, ww
	} else if group == "" {
		res.Reason = "spec unknown (no talent points captured)"
	} else {
		res.Reason = fmt.Sprintf("no stat weights for %q", group)
	}

	level := *c.Level
	wsk := LoadWeaponSkills(c, ds)
	for _, b := range BagUpgrades(c, ds, group) {
		if opt.Slot == "" || opt.Slot == b.Slot {
			res.Bags = append(res.Bags, b)
		}
	}
	rows, err := ds.Filter("items", store.Range{Column: "required_level", Min: 0, Max: level + EquipWindow})
	if err != nil {
		return res, fmt.Errorf("items table: %w", err)
	}
	lo := loadoutOf(c, ds, group)
	bySlot := map[string][]Item{}
	anyStats := false
	for _, row := range rows {
		it := itemOf(row)
		if itemdata.Unobtainable(it.Name) || it.ItemLevel <= 0 || it.Quality <= 0 || it.Quality == qualityArtifact || unequippableBonding(it.Bonding) {
			continue
		}
		if usable, _ := wsk.ItemSkill(c, it.Class, it.Subclass); !usable {
			continue // the class can never learn the weapon's skill
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
		// Judged at the level the item needs when that is above the character's: plate that needs level
		// 40 is theirs from 40, though a warrior wears mail until then.
		if !rl.canUse(int8(i64(row["class_id"])), int8(i64(row["subclass_id"])), int8(it.Slot), max(level, it.ReqLevel)) ||
			!allowsClass(int32(i64(row["allowable_class_mask"])), rl.ID) {
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
		// A worn item the dataset does not hold and the capture gives no item level for is not compared: every item would
		// "beat" it, and that is a guess.
		if name, ok := wornUnknown(c, ds, slot); ok {
			if opt.Slot == "" || opt.Slot == slot {
				if res.Unknown == nil {
					res.Unknown = map[string]string{}
				}
				res.Unknown[slot] = name
			}
			continue
		}
		ranked[slot] = rank(c, ds, slot, items, w, lo)
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
			_, cands[i].Need = wsk.ItemSkill(c, cands[i].Item.Class, cands[i].Item.Subclass)
			if !opt.NoSources {
				cands[i].Sources = sources(ds, cands[i].Item.ID, level)
				if o, ok := vendors.Best(ds, cands[i].Item.ID); ok {
					cands[i].Vendor = &o
				}
				if cr, ok := craftOf(c, ds, cands[i].Item.ID); ok {
					cands[i].Craft = &cr
				}
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

// wornUnknown is the name of the item worn in slot when the dataset has no row for it and the capture records no item
// level: what it is worth is not known, so nothing can be said to beat it.
func wornUnknown(c character.Character, ds store.DataStore, slot string) (string, bool) {
	eq, ok := c.Equipment[slot]
	if !ok || eq.ItemID == nil || *eq.ItemID == 0 || eq.ItemLevel != nil {
		return "", false
	}
	if _, err := ds.Get("items", int64(*eq.ItemID)); err == nil {
		return "", false
	}
	if eq.Name != "" {
		return eq.Name, true
	}
	return fmt.Sprintf("item %d", *eq.ItemID), true
}

// rank scores the items against what the character wears in slot and keeps the upgrades.
func rank(c character.Character, ds store.DataStore, slot string, items []Item, w map[int64]float64, lo loadout) []Candidate {
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
		if !lo.allows(slot, it) {
			continue
		}
		// A two-hander also takes the off hand: it is weighed against both slots together.
		var off *Item
		if slot == "mainhand" && it.Slot == invTwoHand && lo.off != nil {
			off = lo.off
		}
		byStats := w != nil && it.StatsKnown && (!worn || curKnown && cur.StatsKnown) && (off == nil || off.StatsKnown)
		// A placeholder for an empty slot is no upgrade. By item level alone an item that is not above what is worn is none
		// either; weighed by stats, the gain decides below, since a lower item level can carry the stats the build wants.
		if off == nil && (!byStats && worn && it.ItemLevel <= cur.ItemLevel || !worn && it.ItemLevel <= 1) {
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
			if off != nil {
				cand.Delta -= score(*off, w, byStats)
			}
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

// Inventory types and armor subclasses CanUse treats on their own: no armor type or weapon skill
// decides who may wear them.
const (
	invShirt      = 4
	invTabard     = 19
	invRelic      = 28 // a libram, idol or totem
	armorCosmetic = 5  // cosmetic armor, any class may wear it
	relicLibram   = 7
	relicIdol     = 8
	relicTotem    = 9
)

// relicFor is the relic subclass a class holds in its relic slot: librams for paladins, idols for
// druids and totems for shamans; no other class has one.
func relicFor(classID int) int {
	switch classID {
	case rules["PALADIN"].ID:
		return relicLibram
	case rules["DRUID"].ID:
		return relicIdol
	case rules["SHAMAN"].ID:
		return relicTotem
	}
	return 0
}

// Levelless reports whether the dataset row is worn gear whose required level says nothing about how
// useful it is: a shirt, a tabard or cosmetic armor. It is never "far below" a character's level.
func Levelless(row store.Row) bool {
	if row == nil || int(i64(row["class_id"])) != classArmor {
		return false
	}
	slot := int(i64(row["slot"]))
	return slot == invShirt || slot == invTabard || int(i64(row["subclass_id"])) == armorCosmetic
}

// CanUse reports whether a character of the class token at level can wear or wield the dataset item
// row, by armor type, weapon type, shield and class restriction. Shirts, tabards and cosmetic armor
// suit every class, and a libram, idol or totem suits the paladin, druid or shaman. known is false
// for an unsupported class, an item that is not worn gear (a bag, a quiver), or a row the dataset
// gives no type for.
func CanUse(class string, level int, row store.Row) (usable, known bool) {
	rl, ok := rules[strings.ToUpper(class)]
	if !ok || row == nil || i64(row["slot"]) == 0 {
		return false, false
	}
	classID := int(i64(row["class_id"]))
	if classID != classWeapon && classID != classArmor {
		return false, false // bags and quivers: no rule here says who uses them
	}
	if !allowsClass(int32(i64(row["allowable_class_mask"])), rl.ID) {
		return false, true
	}
	if Levelless(row) {
		return true, true
	}
	if classID == classArmor && int(i64(row["slot"])) == invRelic {
		return int(i64(row["subclass_id"])) == relicFor(rl.ID), true
	}
	return rl.canUse(int8(classID), int8(i64(row["subclass_id"])), int8(i64(row["slot"])), level), true
}
