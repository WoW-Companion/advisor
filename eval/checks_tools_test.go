package eval

// The properties of the newer tools: profession_plan, talent_plan, dungeon_plan, inventory_review
// and doctor. Same rule as checks_test.go: the expected values are worked out here from the
// fixture specs and the capture, never from the tool's own answer.

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/doctor"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/setup"
	"github.com/WoW-Companion/advisor/internal/talents"
)

const (
	toolProfessions = "profession_plan"
	toolTalents     = "talent_plan"
	toolDungeons    = "dungeon_plan"
	toolInventory   = "inventory_review"
	toolDoctor      = "doctor"
)

func init() {
	for name, c := range map[string]check{
		"prof_steps_follow_skill":         {toolProfessions, profStepsFollowSkill},
		"prof_reagent_counts":             {toolProfessions, profReagentCounts},
		"prof_gatherer_not_invented":      {toolProfessions, profGathererNotInvented},
		"prof_bank_note":                  {toolProfessions, profBankNote},
		"prof_unknown_thresholds_flagged": {toolProfessions, profUnknownThresholds},

		"talent_spec_from_points":  {toolTalents, talentSpecFromPoints},
		"talent_next_bounded":      {toolTalents, talentNextBounded},
		"talent_band_fits_level":   {toolTalents, talentBandFitsLevel},
		"talent_unspent_stated":    {toolTalents, talentUnspentStated},
		"talent_unknown_spec_none": {toolTalents, talentUnknownSpecNone},
		"talent_stays_in_class":    {toolTalents, talentStaysInClass},

		"dungeon_level_window":          {toolDungeons, dungeonLevelWindow},
		"dungeon_unknown_level_flag":    {toolDungeons, dungeonUnknownLevelFlag},
		"dungeon_no_wrong_gear":         {toolDungeons, dungeonNoWrongGear},
		"dungeon_ranked_with_chances":   {toolDungeons, dungeonRankedWithChances},
		"dungeon_unlinked_boss_unknown": {toolDungeons, dungeonUnlinkedBoss},
		"dungeon_open_quests_only":      {toolDungeons, dungeonOpenQuestsOnly},

		"inventory_grey_is_vendor":         {toolInventory, inventoryGreyIsVendor},
		"inventory_no_wrong_armor_equip":   {toolInventory, inventoryNoWrongArmor},
		"inventory_unknown_not_guessed":    {toolInventory, inventoryUnknownNotGuessed},
		"inventory_totals_consistent":      {toolInventory, inventoryTotals},
		"inventory_materials_keep_or_bank": {toolInventory, inventoryMaterials},
		"inventory_bank_note":              {toolInventory, inventoryBankNote},

		"doctor_flags_missing_slot_addon": {toolDoctor, doctorMissingSlotAddon},
		"doctor_flags_stale_capture":      {toolDoctor, doctorStaleCapture},
		"doctor_flags_missing_dataset":    {toolDoctor, doctorMissingDataset},
		"doctor_flags_build_mismatch":     {toolDoctor, doctorBuildMismatch},
		"doctor_healthy_setup_passes":     {toolDoctor, doctorHealthy},
		"doctor_failures_carry_fix":       {toolDoctor, doctorFailuresCarryFix},
	} {
		checks[name] = c
	}
}

// ---- shared ----

func skillRank(s character.Skill) int {
	if s.Rank == nil {
		return -1
	}
	return *s.Rank
}

func bagCount(c character.Character, name string, bank bool) int {
	l := c.Bags
	if bank {
		l = c.Bank
	}
	n := 0
	for _, s := range l {
		if s.Name == name {
			n += s.Count
		}
	}
	return n
}

func yes(b *bool) bool { return b != nil && *b }

// ---- profession_plan ----

// plans is the answer for every profession the character has, as the tool asks it.
func plans(e *env) []professions.Leveling {
	var out []professions.Leveling
	for _, p := range e.c.Professions {
		out = append(out, professions.Plan(e.c, e.ds, p.Name))
	}
	return out
}

// expectedSteps are the recipe numbers a skill still gains from and can learn: orange before
// yellow before unknown, the highest learn skill first within each.
func expectedSteps(skill int) []int {
	var tiers [3][]recipeSpec
	for _, r := range recipeSpecs {
		switch {
		case r.learn > skill:
		case r.yellow == 0:
			tiers[2] = append(tiers[2], r)
		case skill < r.yellow:
			tiers[0] = append(tiers[0], r)
		case skill < r.grey:
			tiers[1] = append(tiers[1], r)
		}
	}
	var out []int
	for _, t := range tiers {
		for len(t) > 0 {
			best := 0
			for i, r := range t {
				if r.learn > t[best].learn {
					best = i
				}
			}
			out, t = append(out, t[best].k), append(t[:best], t[best+1:]...)
		}
	}
	return out
}

func profIndex(name string) int {
	for i, d := range profDefs {
		if d.name == name {
			return i
		}
	}
	return -1
}

// A plan lists exactly the recipes the skill can learn and still gains from, in tier order.
func profStepsFollowSkill(e *env) error {
	for _, pl := range plans(e) {
		d := profDefOf(pl.Profession)
		if d == nil {
			continue
		}
		want := expectedSteps(*pl.Skill)
		if len(pl.Steps) != len(want) {
			return fmt.Errorf("%s at skill %d: %d steps, want %d (recipes %v)", pl.Profession, *pl.Skill, len(pl.Steps), len(want), want)
		}
		for i, s := range pl.Steps {
			if w := spellID(profIndex(pl.Profession), want[i]); s.Recipe.SpellID != w {
				return fmt.Errorf("%s step %d is spell %d (%s, %s), want spell %d", pl.Profession, i+1, s.Recipe.SpellID, s.Recipe.Name, s.Difficulty, w)
			}
		}
	}
	return nil
}

// Every reagent says how many the recipe needs, how many the bags and bank hold, and what is missing.
func profReagentCounts(e *env) error {
	for _, pl := range plans(e) {
		d := profDefOf(pl.Profession)
		if d == nil {
			continue
		}
		for _, s := range pl.Steps {
			for _, r := range s.Reagents {
				have := bagCount(e.c, r.Name, false) + bagCount(e.c, r.Name, true)
				if r.Have != have || r.Missing != max(0, r.Need-have) {
					return fmt.Errorf("%s %q needs %d of %q: the answer has %d and misses %d, the capture holds %d", pl.Profession, s.Recipe.Name, r.Need, r.Name, r.Have, r.Missing, have)
				}
			}
			if !s.ReagentsKnown && s.Recipe.SpellID%100 != 6 {
				return fmt.Errorf("%s %q lists no reagents but the dataset holds them", pl.Profession, s.Recipe.Name)
			}
		}
	}
	return nil
}

// A profession the dataset holds no recipes for gets no invented steps, and the answer says so.
func profGathererNotInvented(e *env) error {
	for _, pl := range plans(e) {
		if profDefOf(pl.Profession) != nil {
			continue
		}
		if len(pl.Steps) != 0 {
			return fmt.Errorf("%s has no recipes in the dataset but the plan lists %d steps", pl.Profession, len(pl.Steps))
		}
		if !has(strings.Join(pl.Notes, " "), "no recipes") {
			return fmt.Errorf("%s has no recipes in the dataset and the notes say %q", pl.Profession, pl.Notes)
		}
	}
	return nil
}

// The plan says whether the bank was read, and warns when it was not.
func profBankNote(e *env) error {
	for _, pl := range plans(e) {
		if pl.BankKnown != e.c.BankCaptured {
			return fmt.Errorf("%s: BankKnown is %v, the capture's bank is captured: %v", pl.Profession, pl.BankKnown, e.c.BankCaptured)
		}
		if noted := has(strings.Join(pl.Notes, " "), "bank not captured"); noted == e.c.BankCaptured && len(pl.Steps) > 0 {
			return fmt.Errorf("%s: bank captured %v but the bank warning shows %v", pl.Profession, e.c.BankCaptured, noted)
		}
	}
	return nil
}

// A recipe with no skill-up thresholds is marked unknown and the notes say its tier is unknown.
func profUnknownThresholds(e *env) error {
	for _, pl := range plans(e) {
		if profDefOf(pl.Profession) == nil {
			continue
		}
		n := 0
		for _, s := range pl.Steps {
			if s.Recipe.YellowSkill == nil && s.Difficulty != professions.Unknown {
				return fmt.Errorf("%s %q has no thresholds but is rated %q", pl.Profession, s.Recipe.Name, s.Difficulty)
			}
			if s.Difficulty == professions.Unknown {
				n++
			}
		}
		if n == 0 || !has(strings.Join(pl.Notes, " "), "no skill-up thresholds") {
			return fmt.Errorf("%s: %d unknown-tier steps and notes %q", pl.Profession, n, pl.Notes)
		}
	}
	return nil
}

// ---- talent_plan ----

func talentPlan(e *env) talents.Result { return talents.Plan(e.c, e.ds, talents.Options{}) }

// The spec is the group with the most points; on a tie or none it is the class's default leveling spec.
func talentSpecFromPoints(e *env) error {
	r := talentPlan(e)
	g, pts := topGroup(e.c)
	if g == "" {
		if !r.SpecDefault || r.Spec == "" || r.Missing != "" {
			return fmt.Errorf("no single top spec (%d points) but the plan has spec %q (default %v, missing %q)", pts, r.Spec, r.SpecDefault, r.Missing)
		}
		return nil
	}
	if !strings.EqualFold(r.Spec, g) || !r.SpecInferred || r.Missing != "" {
		return fmt.Errorf("top group is %s but the plan's spec is %q (inferred %v, missing %q)", g, r.Spec, r.SpecInferred, r.Missing)
	}
	return nil
}

// The list never holds more points than were asked for or than the build still adds, each at rank 1 or more.
func talentNextBounded(e *env) error {
	for _, limit := range []int{0, 2} {
		r := talents.Plan(e.c, e.ds, talents.Options{Limit: limit, Spec: specOf(e)})
		want := talents.DefaultLimit
		if limit > 0 {
			want = limit
		}
		if len(r.Next) > want || len(r.Next) > r.Remaining {
			return fmt.Errorf("limit %d: %d points listed, %d remaining", want, len(r.Next), r.Remaining)
		}
		for _, p := range r.Next {
			if p.Rank < 1 {
				return fmt.Errorf("point %q has rank %d", p.Name, p.Rank)
			}
		}
	}
	return nil
}

// specOf is the character's top group, or the first spec of its class when the capture has none.
func specOf(e *env) string {
	if g, _ := topGroup(e.c); g != "" {
		return g
	}
	return classSpecs[e.c.Class][0]
}

// Every point is one the character has reached the level for.
func talentBandFitsLevel(e *env) error {
	r := talents.Plan(e.c, e.ds, talents.Options{Spec: specOf(e)})
	if r.Missing != "" {
		return fmt.Errorf("no plan for level %d: %s", level(e), r.Missing)
	}
	for _, p := range r.Next {
		if p.Level > level(e) {
			return fmt.Errorf("point %q is for level %d, above level %d", p.Name, p.Level, level(e))
		}
	}
	return nil
}

// The plan carries the captured unspent count into its data and its text.
func talentUnspentStated(e *env) error {
	r := talents.Plan(e.c, e.ds, talents.Options{Spec: specOf(e)})
	if r.Unspent == nil || *r.Unspent != unspent(e) {
		return fmt.Errorf("the capture holds %d unspent points, the plan says %v", unspent(e), r.Unspent)
	}
	if want := fmt.Sprintf("Unspent points: %d", unspent(e)); !strings.Contains(talents.Render(r), want) {
		return fmt.Errorf("the plan text lacks %q", want)
	}
	return nil
}

// A spec the dataset has no build for gets no plan, and the answer says it has no checked build.
func talentUnknownSpecNone(e *env) error {
	r := talents.Plan(e.c, e.ds, talents.Options{Spec: "Nonexistent"})
	if !strings.Contains(r.Missing, "I don't have a checked build for Nonexistent yet.") || len(r.Next) != 0 || len(r.Available) != len(classSpecs[e.c.Class]) {
		return fmt.Errorf("spec Nonexistent: Missing %q, %d points, %d builds available (want %d)", r.Missing, len(r.Next), len(r.Available), len(classSpecs[e.c.Class]))
	}
	return nil
}

// Every point is a node of the character's own class.
func talentStaysInClass(e *env) error {
	for _, spec := range classSpecs[e.c.Class] {
		r := talents.Plan(e.c, e.ds, talents.Options{Spec: spec})
		for _, p := range r.Next {
			if !has(p.Name, e.c.Class) || !has(p.Name, spec) {
				return fmt.Errorf("%s plan for a %s lists %q", spec, e.c.Class, p.Name)
			}
		}
	}
	return nil
}

// ---- dungeon_plan ----

func dungeonPlan(e *env) (dungeons.Result, error) { return dungeons.Plan(e.c, e.ds) }

// A pick is a dungeon the character has reached (its minimum is at or below their level) and whose range tops out no more than 3 levels below them;
// a dungeon with no range is never ranked, only listed as level unknown.
func dungeonLevelWindow(e *env) error {
	res, err := dungeonPlan(e)
	if err != nil {
		return err
	}
	b, lvl := level(e), level(e)
	got := map[int64]dungeons.DungeonPick{}
	for _, p := range res.Picks {
		got[p.MapID] = p
	}
	for _, d := range dungeonSpecs {
		id := int64(b*100 + d.k)
		lo, hi := b+d.lo, b+d.hi
		in := d.known && d.typ == "dungeon" && hi >= lvl-3 && lo <= lvl
		p, ok := got[id]
		if ok != in {
			return fmt.Errorf("%s (levels %d-%d, %s) for a level %d character: in picks %v, want %v", dungeonName(b, d), lo, hi, d.typ, lvl, ok, in)
		}
		if ok {
			fit := max(lvl-hi, 0)
			if p.Fit != fit || p.LevelUnknown {
				return fmt.Errorf("%s: fit %d (level unknown %v), want %d", p.Name, p.Fit, p.LevelUnknown, fit)
			}
		}
	}
	return nil
}

// The dungeons with no level range come after the picks, each flagged, and the text says so.
func dungeonUnknownLevelFlag(e *env) error {
	res, err := dungeonPlan(e)
	if err != nil {
		return err
	}
	want := 0 // the dataset holds the dungeons of every band, and a dungeon with no range fits none
	for _, d := range dungeonSpecs {
		if !d.known && d.typ == "dungeon" {
			want += bands
		}
	}
	if len(res.Unknown) != want {
		return fmt.Errorf("%d dungeons without a level range but %d listed as unknown", want, len(res.Unknown))
	}
	for _, p := range res.Unknown {
		if !p.LevelUnknown || p.MinLevel != nil || p.MaxLevel != nil {
			return fmt.Errorf("%s is listed as unknown but carries level %v-%v, flag %v", p.Name, p.MinLevel, p.MaxLevel, p.LevelUnknown)
		}
	}
	if text := dungeons.Render(e.c, res, 0); want > 0 && res.First == nil && !strings.Contains(text, "Level not known yet") {
		return fmt.Errorf("the text never says \"Level not known yet\":\n%s", text)
	}
	return nil
}

// No drop is armor of another type or an item locked to another class.
func dungeonNoWrongGear(e *env) error {
	res, err := dungeonPlan(e)
	if err != nil {
		return err
	}
	for _, p := range append(res.Picks, res.Unknown...) {
		for _, d := range p.Drops {
			r := row(e, "items", d.ItemID)
			if r == nil {
				return fmt.Errorf("%s drop %q (%d) is not in the dataset", p.Name, d.Item, d.ItemID)
			}
			if num(r, "class_id") == 4 && armorSlots[d.Slot] && num(r, "subclass_id") != armorOf[e.c.Class] {
				return fmt.Errorf("%s drops %s armor %q for a %s", p.Name, armorName[num(r, "subclass_id")], d.Item, e.c.Class)
			}
			if mask := num(r, "allowable_class_mask"); mask > 0 && mask&classBit[e.c.Class] == 0 {
				return fmt.Errorf("%s drops %q, locked to class mask %d, for a %s", p.Name, d.Item, mask, e.c.Class)
			}
		}
	}
	return nil
}

var classBit = map[string]int{"ROGUE": maskRogue, "PRIEST": maskPriest}

// Picks are ordered by upgrades, then by distance; each pick counts its distinct upgrades, and a
// drop chance is drops over kills as a percentage.
func dungeonRankedWithChances(e *env) error {
	res, err := dungeonPlan(e)
	if err != nil {
		return err
	}
	for i, p := range res.Picks {
		items := map[int64]bool{}
		for _, d := range p.Drops {
			items[d.ItemID] = true
			if d.Chance == nil || *d.Chance < 6.99 || *d.Chance > 7.01 {
				return fmt.Errorf("%s: %q drops 7 times in 100 kills but the chance is %v", p.Name, d.Item, d.Chance)
			}
		}
		if p.Upgrades != len(items) {
			return fmt.Errorf("%s counts %d upgrades but drops %d distinct items", p.Name, p.Upgrades, len(items))
		}
		if i > 0 {
			q := res.Picks[i-1]
			if q.Upgrades < p.Upgrades || q.Upgrades == p.Upgrades && q.Fit > p.Fit {
				return fmt.Errorf("%s (%d upgrades, fit %d) is ranked after %s (%d, %d)", p.Name, p.Upgrades, p.Fit, q.Name, q.Upgrades, q.Fit)
			}
		}
	}
	return nil
}

// A boss the dataset cannot tie to a creature is named as unknown, and no drop is credited to it.
func dungeonUnlinkedBoss(e *env) error {
	res, err := dungeonPlan(e)
	if err != nil {
		return err
	}
	for _, p := range append(res.Picks, res.Unknown...) {
		for _, d := range p.Drops {
			if strings.HasPrefix(d.Boss, "Eval Shade") {
				return fmt.Errorf("%s credits %q to %s, which has no creature", p.Name, d.Item, d.Boss)
			}
		}
		wantGhost := p.MapID%100 == 1
		if got := len(p.UnlinkedBosses); wantGhost != (got == 1) || got > 1 {
			return fmt.Errorf("%s lists %d unlinked bosses %v, want %v", p.Name, got, p.UnlinkedBosses, wantGhost)
		}
	}
	return nil
}

// A dungeon lists the character's open quests the dataset ties to it, and never a turned-in or failed one.
func dungeonOpenQuestsOnly(e *env) error {
	res, err := dungeonPlan(e)
	if err != nil {
		return err
	}
	b := level(e)
	for _, p := range append(res.Picks, res.Unknown...) {
		want := map[int]bool{}
		for _, q := range e.c.Quests {
			if k, ok := questDungeon[q.QuestID-b*100]; ok && int(p.MapID) == b*100+k && !yes(q.Complete) && !yes(q.Failed) {
				want[q.QuestID] = true
			}
		}
		if len(p.Quests) != len(want) {
			return fmt.Errorf("%s lists %d quests %v, want %d open ones %v", p.Name, len(p.Quests), p.Quests, len(want), want)
		}
		for _, q := range p.Quests {
			if !want[q.ID] {
				return fmt.Errorf("%s lists quest %q (%d), which is turned in, failed or not linked", p.Name, q.Title, q.ID)
			}
		}
	}
	return nil
}

// ---- inventory_review ----

func review(e *env) inventory.Result { return inventory.Review(e.c, e.ds) }

// every is each stack of the answer with the group it sits in.
func every(r inventory.Result) []inventory.Entry {
	var out []inventory.Entry
	for _, g := range inventory.Order {
		out = append(out, r.Groups[g]...)
	}
	return out
}

// Items of poor quality (grey, by the dataset) are in the vendor group, and nothing else is there.
func inventoryGreyIsVendor(e *env) error {
	for _, s := range every(review(e)) {
		r := row(e, "items", s.ItemID)
		grey := r != nil && num(r, "quality") == 0
		if grey && s.Group != inventory.Vendor {
			return fmt.Errorf("grey item %q is under %s (%s)", s.Name, s.Group, s.Reason)
		}
		if !grey && s.Group == inventory.Vendor {
			return fmt.Errorf("%q is under vendor but is not grey: %s", s.Name, s.Reason)
		}
	}
	return nil
}

// An item to equip is never armor of another type or an item locked to another class.
func inventoryNoWrongArmor(e *env) error {
	for _, s := range review(e).Groups[inventory.Equip] {
		r := row(e, "items", s.ItemID)
		if r == nil {
			return fmt.Errorf("equip %q is not in the dataset", s.Name)
		}
		if num(r, "class_id") == 4 && num(r, "slot") != 0 && num(r, "subclass_id") != 0 && num(r, "subclass_id") != armorOf[e.c.Class] {
			return fmt.Errorf("%s armor %q is to equip for a %s", armorName[num(r, "subclass_id")], s.Name, e.c.Class)
		}
		if mask := num(r, "allowable_class_mask"); mask > 0 && mask&classBit[e.c.Class] == 0 {
			return fmt.Errorf("%q, locked to class mask %d, is to equip for a %s", s.Name, mask, e.c.Class)
		}
	}
	return nil
}

// An item the dataset does not hold is sorted as unknown, with no price made up.
func inventoryUnknownNotGuessed(e *env) error {
	n := 0
	for _, s := range every(review(e)) {
		if row(e, "items", s.ItemID) != nil {
			continue
		}
		n++
		if s.Group != inventory.Unknown || s.SellPrice != nil {
			return fmt.Errorf("%q is not in the dataset but sits under %s with price %v", s.Name, s.Group, s.SellPrice)
		}
	}
	if n == 0 {
		return fmt.Errorf("the capture holds no item outside the dataset: the property has nothing to check")
	}
	return nil
}

// Every captured stack appears once, and the vendor value, the unpriced count and the slots freed add up.
func inventoryTotals(e *env) error {
	r := review(e)
	got := map[string]int{}
	for _, s := range every(r) {
		got[fmt.Sprintf("%s/%d", s.Where, s.ItemID)] += s.Count
	}
	want := map[string]int{}
	for where, l := range map[string][]character.ItemStack{"bags": e.c.Bags, "bank": e.c.Bank} {
		for _, s := range l {
			want[fmt.Sprintf("%s/%d", where, s.ItemID)] += s.Count
		}
	}
	if len(got) != len(want) {
		return fmt.Errorf("the capture holds %d stacks, the answer sorts %d", len(want), len(got))
	}
	for k, n := range want {
		if got[k] != n {
			return fmt.Errorf("stack %s holds %d, the answer says %d", k, n, got[k])
		}
	}
	var value int64
	unpriced, freed := 0, 0
	for _, s := range every(r) {
		if s.Where == "bags" && (s.Group == inventory.Vendor || s.Group == inventory.Bank) {
			freed++
		}
		if s.Group != inventory.Vendor {
			continue
		}
		if p, ok := row(e, "items", s.ItemID)["sell_price"].(int64); ok {
			value += p * int64(s.Count)
		} else {
			unpriced++
		}
	}
	if r.VendorValue != value || r.VendorUnpriced != unpriced || r.SlotsFreed != freed {
		return fmt.Errorf("vendor value %d, unpriced %d, slots freed %d; the dataset and capture give %d, %d, %d", r.VendorValue, r.VendorUnpriced, r.SlotsFreed, value, unpriced, freed)
	}
	return nil
}

// A reagent of a recipe in reach is kept; a reagent only of a recipe far out of reach is banked.
func inventoryMaterials(e *env) error {
	keep, far := map[string]bool{}, map[string]bool{}
	for _, p := range e.c.Professions {
		if d := profDefOf(p.Name); d != nil {
			keep[d.primary] = true
			if d.secondary != "" {
				far[d.secondary] = true
			}
		}
	}
	r := review(e)
	group := map[string]inventory.Group{}
	for _, s := range every(r) {
		group[s.Name] = s.Group
	}
	for name, g := range group {
		switch {
		case keep[name] && g != inventory.Keep:
			return fmt.Errorf("%q is a reagent in reach and sits under %s", name, g)
		case !keep[name] && far[name] && g != inventory.Bank:
			return fmt.Errorf("%q is a reagent of a far recipe and sits under %s", name, g)
		}
	}
	return nil
}

// The answer warns that the bank was not read, and only then.
func inventoryBankNote(e *env) error {
	if noted := has(strings.Join(review(e).Notes, " "), "bank not captured"); noted == e.c.BankCaptured {
		return fmt.Errorf("bank captured %v but the bank warning shows %v", e.c.BankCaptured, noted)
	}
	return nil
}

// ---- doctor ----

// setupWorld is a game folder, a dataset cache and a home under a temporary directory: the
// character's capture, every addon, a verified dataset for the client's build, and a client config.
type setupWorld struct {
	root, game, cache, sv string
	opt                   doctor.Options
}

func put(path, data string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		panic(err)
	}
}

func cacheBuild(cache, build string) {
	sum := sha256.Sum256([]byte("rows"))
	put(filepath.Join(cache, build, "t.bin"), "rows")
	put(filepath.Join(cache, build, "manifest.json"),
		fmt.Sprintf(`{"dataset_version":"v1","game_build":%q,"tables":[{"path":"t.bin","sha256":%q}]}`, build, hex.EncodeToString(sum[:])))
}

func healthyWorld(e *env) setupWorld {
	root, err := os.MkdirTemp("", "eval-doctor")
	if err != nil {
		panic(err)
	}
	w := setupWorld{root: root, game: filepath.Join(root, "game"), cache: filepath.Join(root, "cache")}
	w.sv = filepath.Join(w.game, "WTF", "Account", "ACCT", "SavedVariables", "ForeverScribe.lua")
	src, err := os.ReadFile(filepath.Join(fixtures, "captures", e.file+".lua"))
	if err != nil {
		panic(err)
	}
	put(w.sv, string(src))
	for _, a := range doctor.Addons {
		put(filepath.Join(w.game, "Interface", "AddOns", a, a+".toc"), "## Interface: 16001\n## Version: 1.2.3\n")
	}
	cacheBuild(w.cache, clientVersion+"."+clientBuild)
	put(filepath.Join(root, "home", ".gemini", "settings.json"), `{"mcpServers":{"wowc-advisor":{}}}`)
	w.opt = doctor.Options{SavedVariables: w.sv, Cache: w.cache, Now: e.now,
		Env: setup.Env{GOOS: "linux", Home: filepath.Join(root, "home"), Cwd: filepath.Join(root, "home", "proj")}}
	return w
}

// diagnose builds a healthy world, lets mutate break it, and runs the doctor.
func diagnose(e *env, mutate func(w *setupWorld)) doctor.Report {
	w := healthyWorld(e)
	defer os.RemoveAll(w.root)
	mutate(&w)
	return doctor.Run(w.opt)
}

func checkNamed(r doctor.Report, name string) (doctor.Check, error) {
	for _, c := range r.Checks {
		if c.Name == name {
			return c, nil
		}
	}
	return doctor.Check{}, fmt.Errorf("the report has no check %q", name)
}

// expect asks for a check to have a status, and to carry a fix exactly when it failed.
func expect(r doctor.Report, name, status string) error {
	c, err := checkNamed(r, name)
	if err != nil {
		return err
	}
	if c.Status != status || (status == doctor.Fail) != (c.Fix != "") {
		return fmt.Errorf("check %q is %s with fix %q (%s), want %s", name, c.Status, c.Fix, c.Detail, status)
	}
	return nil
}

func doctorMissingSlotAddon(e *env) error {
	r := diagnose(e, func(w *setupWorld) {
		for _, a := range []string{digest.SlotAddon, ask.SlotAddon} {
			os.RemoveAll(filepath.Join(w.game, "Interface", "AddOns", a))
		}
	})
	for name, status := range map[string]string{"Addon WoWCompanion": doctor.Pass, "Addon " + digest.SlotAddon: doctor.Fail, "Addon " + ask.SlotAddon: doctor.Fail} {
		if err := expect(r, name, status); err != nil {
			return err
		}
	}
	if r.OK {
		return fmt.Errorf("two addons are missing but the report is OK")
	}
	return nil
}

func doctorStaleCapture(e *env) error {
	if err := expect(diagnose(e, func(*setupWorld) {}), "Capture age", doctor.Pass); err != nil {
		return err
	}
	r := diagnose(e, func(w *setupWorld) { w.opt.Now = e.now.Add(3 * time.Hour) })
	if err := expect(r, "Capture age", doctor.Fail); err != nil {
		return err
	}
	if c, _ := checkNamed(r, "Capture age"); !strings.Contains(c.Fix, "/reload") {
		return fmt.Errorf("a stale capture's fix is %q, want a /reload", c.Fix)
	}
	return nil
}

func doctorMissingDataset(e *env) error {
	r := diagnose(e, func(w *setupWorld) { os.RemoveAll(w.cache) })
	for _, name := range []string{"Dataset cache", "Dataset build"} {
		if err := expect(r, name, doctor.Fail); err != nil {
			return err
		}
	}
	if c, _ := checkNamed(r, "Dataset cache"); !strings.Contains(c.Fix, "dataset update") {
		return fmt.Errorf("a missing dataset's fix is %q, want a dataset update", c.Fix)
	}
	return nil
}

func doctorBuildMismatch(e *env) error {
	r := diagnose(e, func(w *setupWorld) {
		os.RemoveAll(w.cache)
		cacheBuild(w.cache, "1.0.0.11111")
	})
	if err := expect(r, "Dataset cache", doctor.Pass); err != nil {
		return err
	}
	if err := expect(r, "Dataset build", doctor.Fail); err != nil {
		return err
	}
	if c, _ := checkNamed(r, "Dataset build"); !strings.Contains(c.Detail, clientBuild) {
		return fmt.Errorf("the mismatch does not name the client build %s: %q", clientBuild, c.Detail)
	}
	return nil
}

func doctorHealthy(e *env) error {
	r := diagnose(e, func(*setupWorld) {})
	for _, c := range r.Checks {
		if c.Status == doctor.Fail {
			return fmt.Errorf("a healthy setup fails check %q: %s", c.Name, c.Detail)
		}
	}
	if !r.OK || !strings.Contains(r.Text, "All checks passed") {
		return fmt.Errorf("a healthy setup reports OK %v and text %q", r.OK, r.Text)
	}
	return nil
}

func doctorFailuresCarryFix(e *env) error {
	r := diagnose(e, func(w *setupWorld) {
		os.RemoveAll(w.game)
		os.RemoveAll(w.cache)
	})
	if r.OK {
		return fmt.Errorf("a setup with no game folder and no dataset reports OK")
	}
	fails := 0
	for _, c := range r.Checks {
		if c.Status != doctor.Fail {
			continue
		}
		fails++
		if c.Fix == "" || !strings.Contains(r.Text, "fix: "+c.Fix) {
			return fmt.Errorf("failed check %q has fix %q and the text does not show it", c.Name, c.Fix)
		}
	}
	if fails < 4 {
		return fmt.Errorf("only %d checks failed in a setup with nothing installed", fails)
	}
	return nil
}
