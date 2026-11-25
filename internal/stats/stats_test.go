package stats

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// fakeDS answers items by id.
type fakeDS map[int64]store.Row

func (f fakeDS) Get(_ string, id int64) (store.Row, error) {
	if r, ok := f[id]; ok {
		return r, nil
	}
	return nil, errors.New("not found")
}
func (fakeDS) Filter(string, ...store.Range) ([]store.Row, error)  { return nil, nil }
func (fakeDS) SearchName(string, string, int) ([]store.Row, error) { return nil, nil }

func row(id int64, stats ...int64) store.Row {
	var l []any
	for _, s := range stats {
		l = append(l, s)
	}
	return store.Row{"id": id, "stats_status": "known", "stat_types": l}
}

// gear is worn gear for a level 10 warrior: strength, agility and stamina on a hammer and a cloak, two
// items with Spirit.
func gear() (character.Character, store.DataStore) {
	ids := map[string]int{"mainhand": 1, "back": 2, "head": 3, "chest": 4}
	names := map[string]string{"mainhand": "Sequoia Hammer of the Wolf", "back": "Plain Cloak", "head": "Owl Cap", "chest": "Owl Vest"}
	eq := map[string]character.EquippedItem{}
	for slot, id := range ids {
		id := id
		eq[slot] = character.EquippedItem{Slot: slot, ItemID: &id, Name: names[slot]}
	}
	lvl := 10
	c := character.Character{Name: "Tw", Key: "R:Tw", Class: "WARRIOR", Level: &lvl, Equipment: eq}
	ds := fakeDS{1: row(1, 3, 6), 2: row(2, 4, 3), 3: row(3, 5, 6, 7), 4: row(4, 4, 6, 7)}
	return c, ds
}

func style(c character.Character, role, spec string) character.Character {
	c.Profile = &character.Profile{Role: role, Focus: "leveling", Spec: spec}
	return c
}

var plain = regexp.MustCompile(`%|\d{3,}|\d+\s*:\s*\d+|\bratio\b`)

func TestEveryClassHasAProfileAndFallback(t *testing.T) {
	for _, class := range []string{"WARRIOR", "PALADIN", "HUNTER", "ROGUE", "PRIEST", "SHAMAN", "MAGE", "WARLOCK", "DRUID"} {
		p, ok := ForClass(class)
		if !ok || len(p.Weights) == 0 || p.Reason(Prof{Profile: p, Weights: p.Weights}.top()) == "" {
			t.Errorf("%s: no fallback profile", class)
		}
	}
	if len(Classes()) != 9 {
		t.Errorf("classes = %v", Classes())
	}
	if _, ok := ForClass("MONK"); ok {
		t.Error("unknown class has a profile")
	}
	for _, p := range table {
		for stat := range p.Weights {
			if p.Reason(stat) == "" {
				t.Errorf("%s %s: no reason for %s", p.Class, p.Spec, stat)
			}
		}
	}
}

func TestFuryAndProtectionDiffer(t *testing.T) {
	c, ds := gear()
	fury := Check(style(c, "damage", "Fury"), ds, "")
	prot := Check(style(c, "tank", "Protection"), ds, "")
	if len(fury.Insights) == 0 || len(prot.Insights) == 0 || fury.Insights[0] == prot.Insights[0] {
		t.Fatalf("same top insight: %q / %q", fury.Insights, prot.Insights)
	}
	if !strings.Contains(fury.Insights[0], "Strength") || !strings.Contains(prot.Insights[0], "Stamina") {
		t.Errorf("tops: %q / %q", fury.Insights[0], prot.Insights[0])
	}
}

func TestSpiritIsWastedOnAWarrior(t *testing.T) {
	c, ds := gear()
	lvl := 30 // past the low-level note, so the suffix line and the swap both fit
	c.Level = &lvl
	r := Check(style(c, "damage", "Fury"), ds, "")
	var found bool
	for _, in := range r.Insights {
		found = found || strings.Contains(in, "Spirit") && strings.Contains(in, "swap")
	}
	if !found || len(r.Swap) == 0 {
		t.Errorf("insights %q swap %v", r.Insights, r.Swap)
	}
	// A priest keeps it.
	c.Class = "PRIEST"
	for _, in := range Check(style(c, "healer", "Holy"), ds, "").Insights {
		if strings.Contains(in, "Spirit") && strings.Contains(in, "swap") {
			t.Errorf("priest told Spirit is wasted: %q", in)
		}
	}
}

func TestPreferenceShiftsWeights(t *testing.T) {
	p, _ := Lookup("WARRIOR", "Fury", "damage")
	dmg, surv := Adjust(p.Weights, PrefDamage), Adjust(p.Weights, PrefSurvival)
	if !(dmg[Stamina] < p.Weights[Stamina] && p.Weights[Stamina] < surv[Stamina]) {
		t.Errorf("stamina %v %v %v", dmg[Stamina], p.Weights[Stamina], surv[Stamina])
	}
	if !(dmg[Strength] >= surv[Strength]) || Adjust(p.Weights, "")[Stamina] != p.Weights[Stamina] {
		t.Error("strength or balanced")
	}
	for _, v := range surv {
		if v > 1 {
			t.Error("weight above 1")
		}
	}
}

func TestGearOnlyTotalsAreUsedAndSaid(t *testing.T) {
	c, ds := gear()
	if r := Check(style(c, "damage", "Fury"), ds, ""); r.Source != SourceGear {
		t.Errorf("source = %q", r.Source)
	}
	one := 40.0
	c.Totals = &character.Totals{Str: &one, Agi: &one, Sta: &one}
	if r := Check(style(c, "damage", "Fury"), ds, ""); r.Source != SourceCaptured {
		t.Errorf("source = %q", r.Source)
	}
	c.Totals, c.Equipment = nil, nil
	r := Check(style(c, "damage", "Fury"), ds, "")
	if r.Source != SourceNone || len(r.Insights) == 0 {
		t.Errorf("no stats: %+v", r)
	}
}

func TestInsightsArePlain(t *testing.T) {
	c, ds := gear()
	for _, role := range [][2]string{{"damage", "Fury"}, {"damage", "Arms"}, {"tank", "Protection"}, {"", ""}} {
		r := Check(style(c, role[0], role[1]), ds, "")
		if len(r.Insights) < 1 || len(r.Insights) > MaxInsights {
			t.Errorf("%v: %d insights", role, len(r.Insights))
		}
		for _, in := range r.Insights {
			if plain.MatchString(in) || len([]rune(in)) > MaxInsight {
				t.Errorf("%v: %q", role, in)
			}
		}
		// A level 10 character is told the numbers are small.
		if !strings.Contains(strings.Join(r.Insights, " "), "small") {
			t.Errorf("%v: no low-level note in %q", role, r.Insights)
		}
	}
}

func TestNoPlayStyleReturnsChoices(t *testing.T) {
	c, ds := gear()
	r := Check(c, ds, "")
	if r.Needs != NeedsPlayStyle || r.Hook != "Choose a play style for a stat check" || len(r.Insights) != 0 {
		t.Errorf("%+v", r)
	}
}

func TestTotalsParse(t *testing.T) {
	src := `WoWCompanionDB = { characters = { ["Realm:Tw"] = { stats = { seenAt = 5, level = 10, armor = 140, stats = { str = { total = 31 }, sta = { total = 22.5 } } } }, ["Other:Zz"] = "junk" } }`
	tt := character.ParseTotals(src, "Realm:Tw", "Tw")
	if tt == nil || *tt.Str != 31 || *tt.Sta != 22.5 || tt.Agi != nil || !tt.Any() {
		t.Errorf("%+v", tt)
	}
	if character.ParseTotals(src, "Other:Zz", "Zz") != nil || character.ParseTotals("not lua {{", "a", "a") != nil || character.ParseTotals(src, "No:One", "One") != nil {
		t.Error("tolerant parse")
	}
}

// TestReportWarriors prints the level 10 warrior's check under each warrior profile, for the report
// (go test -run TestReportWarriors -v).
func TestReportWarriors(t *testing.T) {
	c, ds := gear()
	for _, role := range [][2]string{{"damage", "Arms"}, {"damage", "Fury"}, {"tank", "Protection"}} {
		p, _ := For(style(c, role[0], role[1]), "")
		r := Check(style(c, role[0], role[1]), ds, "")
		t.Logf("%s weights=%v source=%s lean=%v totals=%v swap=%v", p.Title(), p.Weights, r.Source, r.Lean, r.Totals, r.Swap)
		for _, in := range r.Insights {
			t.Logf("   - %s", in)
		}
	}
}
