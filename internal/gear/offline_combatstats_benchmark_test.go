package gear

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/stats"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline combat-stat benchmark: how the advisor treats hit, crit, defense, resistance and the per-point stat
// conversions, beside the dataset's item rows and the game's own rules, with no network. The dataset holds item stats
// (Defense Rating 12, Hit Rating 31, Crit Rating 32, the six resistances 51 to 56, with amounts) and the weapon skills a
// class learns; it holds no hit cap, no crit-immunity or defense-skill figure, no per-point stat table and no fight-to-
// resistance table, so the cases that need those check that the advisor states none. The item rows are real rows kept in
// testdata/items_snapshot, gating_snapshot, usability_snapshot and advice_snapshot. A difference whose cause is a missing
// dataset row is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	csItemSrc   = "dataset items.stats (stat type 12 Defense, 31 Hit, 32 Crit, 51 to 56 resistances, amount); Warcraft Wiki Hit, Defense, Resistance pages"
	csSkillSrc  = "dataset weapon_skills (class_id, skill_line_id, learnable)"
	csConvSrc   = "game class rules: 15 mana per Intellect, 10 health per Stamina, Intellect per 1% spell crit at level 60 (Warcraft Wiki Intellect, Stamina, Spell critical strike); the dataset holds no per-point table"
	csNoneSrc   = "dataset: no hit cap, defense skill, crit immunity, per-point or fight-resistance table"
	csGuideNote = "guides want it"
)

var csDirs = []string{"items_snapshot", "gating_snapshot", "usability_snapshot", "advice_snapshot"}

// csItem is the item row of an id from whichever snapshot holds it.
func csItem(t *testing.T, id int64) Item {
	t.Helper()
	for _, d := range csDirs {
		if row, err := store.NewDir("../../testdata/"+d).Get("items", id); err == nil {
			return itemOf(row)
		}
	}
	t.Fatalf("item %d is in no snapshot", id)
	return Item{}
}

func csProfile(class, spec, role string) character.Character {
	lvl := 60
	return character.Character{Name: "Benchtest", Class: class, Level: &lvl, Profile: &character.Profile{Role: role, Focus: "leveling", Spec: spec}}
}

// csWeights are the advisor's item weights by stat type id for a class, spec and role.
func csWeights(t *testing.T, class, spec, role string) map[int64]float64 {
	t.Helper()
	p, ok := stats.For(csProfile(class, spec, role), "")
	if !ok {
		t.Fatalf("no profile for %s %s", class, spec)
	}
	return idWeights(p.ItemWeights())
}

// ---- hit cap and weapon skills ----

var csHitCases = []struct {
	class, spec, role string
	stat              int64
	word, why         string
}{
	{"WARRIOR", "Fury", "damage", 31, "Hit Rating", "a melee damage dealer misses without it"},
	{"ROGUE", "Combat", "damage", 31, "Hit Rating", "a dual wielder misses a lot without it"},
	{"HUNTER", "Marksmanship", "damage", 31, "Hit Rating", "a ranged attacker misses without it"},
	{"MAGE", "Fire", "damage", 31, "Hit Rating", "a spell can miss a boss"},
}

// TestOfflineBenchmarkHitWeight: the stat types a damage dealer's gear carries for hit count for the build; the dataset gives
// no cap, so the weight is a plain "wanted", never a figure.
func TestOfflineBenchmarkHitWeight(t *testing.T) {
	var out []benchRow
	for _, c := range csHitCases {
		w := csWeights(t, c.class, c.spec, c.role)[c.stat]
		res := "match"
		if w <= 0 {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s: weight of %s (stat %d)", strings.ToLower(c.class), c.spec, c.word, c.stat),
			fmt.Sprintf("%.2f", w), "above 0: " + c.why + " (" + csGuideNote + ")", csItemSrc, res})
	}
	finish(t, "combat-hit", "Offline combat stats: hit weights", out)
}

// TestOfflineBenchmarkWeaponSkillRows: the weapon skills the advisor offers a class are the dataset's learnable rows.
func TestOfflineBenchmarkWeaponSkillRows(t *testing.T) {
	ds := store.NewDir("../../testdata/usability_snapshot")
	rows, err := ds.Filter("weapon_skills", store.Range{Column: "class_id", Min: int64(0), Max: int64(99)})
	if err != nil {
		t.Fatal(err)
	}
	var out []benchRow
	for _, c := range []struct {
		class string
		skill int64
		name  string
	}{{"HUNTER", 229, "Polearms"}, {"PRIEST", 43, "One-Handed Swords"}, {"MAGE", 136, "Staves"}, {"ROGUE", 46, "Guns"}, {"PALADIN", 45, "Bows"}, {"SHAMAN", 473, "Fist Weapons"}} {
		var want bool
		for _, r := range rows {
			if i64(r["class_id"]) == classIDs[c.class] && i64(r["skill_line_id"]) == c.skill {
				want, _ = r["learnable"].(bool)
			}
		}
		lvl := 60
		got := LoadWeaponSkills(character.Character{Class: c.class, Level: &lvl}, ds).Learnable[c.skill]
		res := "match"
		if got != want {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s: can learn %s", strings.ToLower(c.class), c.name), strconv.FormatBool(got), strconv.FormatBool(want), csSkillSrc, res})
	}
	finish(t, "combat-weaponskill", "Offline combat stats: weapon skills", out)
}

// ---- tank defense and crit immunity ----

// TestOfflineBenchmarkTankDefenseWeight: a tank's gear counts Defense Rating; the dataset has stat 12 on tank shields and belts.
func TestOfflineBenchmarkTankDefenseWeight(t *testing.T) {
	var out []benchRow
	for _, c := range []struct{ class, spec, role string }{{"WARRIOR", "Protection", "tank"}, {"PALADIN", "Protection", "tank"}, {"DRUID", "Feral Combat", "tank"}} {
		w := csWeights(t, c.class, c.spec, c.role)[12]
		res := "match"
		if w <= 0 {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s tank: weight of Defense Rating (stat 12)", strings.ToLower(c.class), c.spec), fmt.Sprintf("%.2f", w),
			"above 0: defense is the tank's crit-immunity stat", csItemSrc, res})
	}
	finish(t, "combat-defense-weight", "Offline combat stats: tank defense weights", out)
}

// TestOfflineBenchmarkTankDefenseItems: a shield or belt that carries Defense Rating is worth more to a tank than its item level alone,
// and of two belts of one item level the one with the tank's stats ranks ahead.
func TestOfflineBenchmarkTankDefenseItems(t *testing.T) {
	var out []benchRow
	for _, c := range []struct {
		id          int64
		class, spec string
	}{
		{9607, "PALADIN", "Protection"},   // Bastion of Stormwind: Defense 3 only
		{250514, "WARRIOR", "Protection"}, // Guard's Chain Belt: Defense 3, Strength 4, Stamina 4
		{250514, "DRUID", "Feral Combat"}, // the same belt weighed for a bear
	} {
		it := csItem(t, c.id)
		base := float64(it.ItemLevel) * 0.5
		got := score(it, csWeights(t, c.class, c.spec, "tank"), true)
		res := "match"
		if got <= base {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s %s): score", it.Name, strings.ToLower(c.class), c.spec), fmt.Sprintf("%.1f", got),
			fmt.Sprintf("above the item-level floor %.1f: Defense Rating counts for a tank", base), csItemSrc, res})
	}
	w := csWeights(t, "WARRIOR", "Protection", "tank")
	guard, protector := score(csItem(t, 250514), w, true), score(csItem(t, 250515), w, true)
	res := "match"
	if guard <= protector {
		res = "differ"
	}
	out = append(out, benchRow{"protection warrior: Guard's Chain Belt against Protector's Chain Belt", fmt.Sprintf("%.1f against %.1f", guard, protector),
		"Guard's first: Defense 3 and Strength 4 against Defense 2 and Spell Power 5", csItemSrc, res})
	finish(t, "combat-defense-items", "Offline combat stats: Defense Rating on gear", out)
}

var csDigits = regexp.MustCompile(`[0-9%]`)

// TestOfflineBenchmarkTankNoImmunityFigure: the dataset has no defense skill or crit-immunity figure, so the stat check names none.
func TestOfflineBenchmarkTankNoImmunityFigure(t *testing.T) {
	lvl := 60
	c := csProfile("WARRIOR", "Protection", "tank")
	c.Level = &lvl
	st := 400.0
	c.Totals = &character.Totals{Sta: &st}
	r := stats.Check(c, store.NewDir("../../testdata/advice_snapshot"), "Protection")
	text := strings.Join(r.Insights, " ")
	res := "match"
	if csDigits.MatchString(text) || strings.Contains(strings.ToLower(text), "immun") {
		res = "differ"
	}
	finish(t, "combat-immunity", "Offline combat stats: crit immunity", []benchRow{{"protection warrior: stat check names a crit-immunity or defense figure", text,
		"no figure and no immunity claim: " + csNoneSrc, csNoneSrc, res}})
}

// ---- resistance and fight-specific gear ----

// withoutResist is the item with its resistance stats removed.
func withoutResist(it Item) Item {
	out := it
	out.Stats, out.StatTypes = map[int64]int{}, nil
	for id, n := range it.Stats {
		if id < 51 || id > 56 {
			out.Stats[id] = n
		}
	}
	for _, id := range it.StatTypes {
		if id < 51 || id > 56 {
			out.StatTypes = append(out.StatTypes, id)
		}
	}
	return out
}

// TestOfflineBenchmarkResistanceNeutral: a resistance on a piece of gear neither adds to nor takes from its general value: it is for
// a particular fight (fire for Ragnaros and Onyxia, nature for the Ahn'Qiraj bosses), and the dataset has no fight table to say which.
func TestOfflineBenchmarkResistanceNeutral(t *testing.T) {
	var out []benchRow
	for _, c := range []struct {
		id                int64
		class, spec, role string
	}{
		{16983, "WARRIOR", "Protection", "tank"}, // Molten Helm: Stamina 16, Fire Resistance 29, Dodge 12
		{15053, "WARRIOR", "Fury", "damage"},     // Volcanic Breastplate: Fire Resistance 20, Stamina 11
		{15071, "MAGE", "Fire", "damage"},        // Frostsaber Boots: Stamina 9, Fire and Shadow Resistance 12
		{12406, "PALADIN", "Protection", "tank"}, // Thorium Belt: Stamina 12, five resistances 6
		{16982, "ROGUE", "Combat", "damage"},     // Corehound Boots: Agility 13, Stamina 10, Fire Resistance 24
	} {
		it := csItem(t, c.id)
		w := csWeights(t, c.class, c.spec, c.role)
		with, without := score(it, w, true), score(withoutResist(it), w, true)
		res := "match"
		if math.Abs(with-without) > 0.001 {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s (%s %s): score with and without its resistance", it.Name, strings.ToLower(c.class), c.spec),
			fmt.Sprintf("%.2f with, %.2f without", with, without), "equal: a resistance is for one fight, not a general stat", csItemSrc, res})
	}
	// resistance-only gear carries nothing a build weighs: it falls to the floor, half its item level
	it := csItem(t, 12631) // Fiery Plate Gauntlets: Fire Resistance 10 only
	got := score(it, csWeights(t, "WARRIOR", "Protection", "tank"), true)
	res := "match"
	if math.Abs(got-float64(it.ItemLevel)*0.5) > 0.001 {
		res = "differ"
	}
	out = append(out, benchRow{"Fiery Plate Gauntlets (protection warrior): resistance-only gear", fmt.Sprintf("%.1f", got), fmt.Sprintf("%.1f: half its item level, no stat the build weighs", float64(it.ItemLevel)*0.5), csItemSrc, res})
	finish(t, "combat-resist", "Offline combat stats: resistance gear", out)
}

// ---- stat conversion ----

var csConv = []struct {
	class      string
	intPerCrit float64 // Intellect per 1% spell crit at level 60
	baseMana   float64 // base mana at level 60
}{
	{"MAGE", 59.5, 750}, {"PRIEST", 59.5, 750}, {"WARLOCK", 60.6, 750}, {"DRUID", 60, 750}, {"SHAMAN", 59.2, 720}, {"PALADIN", 54, 660},
}

func trimNum(f float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(f, 'f', 2, 64), "0"), ".")
}

// TestOfflineBenchmarkStatConversion: the per-point words for Intellect and Stamina, shown when the capture agrees with the class rules.
func TestOfflineBenchmarkStatConversion(t *testing.T) {
	var out []benchRow
	for _, c := range csConv {
		lvl, in := 60, 200.0
		mana, crit, sta := c.baseMana+20+15*(in-20), in/c.intPerCrit, 150.0
		tot := &character.Totals{Level: &lvl, Int: &in, ManaMax: &mana, SpellCrit: &crit, Sta: &sta}
		eff := stats.Effects(c.class, tot)
		want := "+15 mana, +" + trimNum(1/c.intPerCrit) + "% spell crit a point"
		res := "match"
		if eff[stats.Intellect] != want || eff[stats.Stamina] != "+10 health a point" {
			res = "differ"
		}
		out = append(out, benchRow{fmt.Sprintf("%s level 60: Intellect and Stamina per point", strings.ToLower(c.class)),
			eff[stats.Intellect] + "; " + eff[stats.Stamina], want + "; +10 health a point", csConvSrc, res})
	}
	finish(t, "combat-conversion", "Offline combat stats: per-point stat conversion", out)
}

// ---- no answer ----

// TestOfflineBenchmarkCombatNoAnswer: what the dataset and the class rules lack is said by saying nothing: no per-point figure for a
// class without rules, none when the capture disagrees with the rules, and no hit cap, defense skill or resistance target in the
// gear answer.
func TestOfflineBenchmarkCombatNoAnswer(t *testing.T) {
	var out []benchRow
	lvl, in, agi := 60, 200.0, 150.0
	mana, crit := 9999.0, 3.0 // far from what a mage with 200 Intellect has
	for _, c := range []struct {
		name, class string
		tot         *character.Totals
	}{
		{"warrior: Strength and Agility per point (no class rule, no table)", "WARRIOR", &character.Totals{Level: &lvl, Int: &in, Agi: &agi}},
		{"hunter: Agility to crit and attack power per point (no class rule)", "HUNTER", &character.Totals{Level: &lvl, Int: &in, ManaMax: &mana, SpellCrit: &crit}},
		{"mage whose capture disagrees with the rules (mana 9999, crit 3%)", "MAGE", &character.Totals{Level: &lvl, Int: &in, ManaMax: &mana, SpellCrit: &crit}},
	} {
		eff := stats.Effects(c.class, c.tot)
		res := "match"
		if len(eff) != 0 {
			res = "differ"
		}
		out = append(out, benchRow{c.name, fmt.Sprintf("%d per-point phrases", len(eff)), "none: " + csNoneSrc, csNoneSrc, res})
	}
	ds := store.NewDir("../../testdata/advice_snapshot")
	r, err := Find(csProfile("WARRIOR", "Fury", "damage"), ds, "", Options{NoSources: true})
	if err != nil {
		t.Fatal(err)
	}
	text := r.Rationale + " " + r.Reason
	res := "match"
	if csDigits.MatchString(text) || strings.Contains(strings.ToLower(text), "cap") || strings.Contains(strings.ToLower(text), "resist") {
		res = "differ"
	}
	out = append(out, benchRow{"fury warrior: gear answer names a hit cap, a percentage or a resistance target", strings.TrimSpace(text), "none: " + csNoneSrc, csNoneSrc, res})
	finish(t, "combat-noanswer", "Offline combat stats: no answer in the dataset", out)
}
