package stats

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// leadOf is the primary stat a class, spec and role should lead with, stated apart from the table.
func leadOf(p Profile) string {
	switch {
	case p.Role == RoleTank:
		return Stamina
	case p.Role == RoleHealer, p.Class == "MAGE", p.Class == "WARLOCK", p.Arch == archCaster:
		return Intellect
	case p.Class == "HUNTER", p.Class == "ROGUE", p.Arch == archFeral:
		return Agility
	}
	return Strength
}

func TestPrimaryStatAlwaysLeads(t *testing.T) {
	for _, p := range table {
		for _, pref := range []string{"", PrefBalanced, PrefDamage, PrefSurvival} {
			pr := Prof{Profile: p, Weights: Adjust(p.Weights, pref)}
			order := pr.Order()
			if len(order) == 0 || order[0] != leadOf(p) || pr.Lead() != leadOf(p) {
				t.Errorf("%s %s %s pref %q: order %v lead %s", p.Class, p.Spec, p.Role, pref, order, pr.Lead())
			}
			for s, v := range pr.Weights {
				if v > 1 || v < 0 {
					t.Errorf("%s %s %q: %s weight %v", p.Class, p.Spec, pref, s, v)
				}
			}
		}
	}
}

func TestSurvivabilityRaisesStaminaNextToThePrimary(t *testing.T) {
	p, _ := Lookup("WARLOCK", "Affliction", RoleDamage)
	bal, surv := Adjust(p.Weights, PrefBalanced), Adjust(p.Weights, PrefSurvival)
	if !(surv[Stamina] > bal[Stamina]) || !(surv[Intellect] >= surv[Stamina]) {
		t.Errorf("balanced %v survival %v", bal, surv)
	}
}

// itemRows are dataset rows named with the suffixes, for the verification.
func itemRows(names ...string) []store.Row {
	var out []store.Row
	for _, n := range names {
		out = append(out, store.Row{"name": n, "stats_status": "unknown"})
	}
	return out
}

type rowsDS struct {
	fakeDS
	rows []store.Row
}

func (r rowsDS) Filter(string, ...store.Range) ([]store.Row, error) { return r.rows, nil }

func shadow(pref string) character.Character {
	lvl := 24
	i, s, sp := 90.0, 42.0, 76.0
	return character.Character{Name: "Pw", Key: "R:Pw", Class: "PRIEST", Level: &lvl,
		Profile: &character.Profile{Role: "damage", Focus: "leveling", Spec: "Shadow", StatPref: pref},
		Totals:  &character.Totals{Int: &i, Sta: &s, Spi: &sp}}
}

func TestShadowSurvivabilityGivesEagleThenOwl(t *testing.T) {
	p, _ := For(shadow(PrefSurvival), "")
	rows := itemRows("Cap of the Eagle", "Cap of the Owl", "Cap of Intellect", "Cap of the Whale")
	var got []string
	for _, s := range p.VerifiedTop(rows, 2) {
		got = append(got, s.Name)
	}
	if strings.Join(got, "|") != "of the Eagle|of the Owl" {
		t.Errorf("suffixes = %v", got)
	}
	// A suffix the dataset does not hold is never named.
	for _, s := range p.VerifiedTop(itemRows("Cap of the Owl"), 2) {
		if s.Key == "eagle" {
			t.Error("unverified suffix named")
		}
	}
}

func TestChipTextShadowSurvivability(t *testing.T) {
	ds := rowsDS{fakeDS{}, itemRows("Cap of the Eagle", "Cap of the Owl")}
	r := Check(shadow(PrefSurvival), ds, "")
	if len(r.Insights) < 2 {
		t.Fatalf("insights = %q", r.Insights)
	}
	if want := "Shadow: Intellect first (your mana and spell crit), then Stamina for safer pulls."; r.Insights[0] != want {
		t.Errorf("chip = %q", r.Insights[0])
	}
	if want := "Look for 'of the Eagle' (Stamina + Intellect), or 'of the Owl'."; r.Insights[1] != want {
		t.Errorf("suffix line = %q", r.Insights[1])
	}
	if len(r.Insights) > MaxInsights || strings.Join(r.Suffixes, "|") != "of the Eagle|of the Owl" || r.Priority[0] != Intellect {
		t.Errorf("%+v", r)
	}
}

// realStats is the addon's documented shape (capture-format.md), synthetic values.
const realStats = `WoWCompanionDB = { characters = { ["Realm:Pw"] = { name = "Pw", stats = {
  seenAt = 1700000000, level = 20,
  stats = { int = { base = 50, total = 90, pos = 40, neg = 0 }, sta = { base = 40, total = 55, pos = 15, neg = 0 },
    agi = { base = 20, total = 22, pos = 2, neg = 0 }, str = { base = 10, total = 10, pos = 0, neg = 0 },
    spi = { base = 45, total = 60, pos = 15, neg = 0 } },
  healthMax = 700, manaMax = 900, armor = 120, meleeCrit = 3.5, rangedCrit = 3.1, dodge = 4.2, parry = 5, block = 0,
  spellCrit = { max = 4.8, schools = { [2] = 4.8 } }, manaRegen = { base = 6.5, casting = 2.1 },
  attackPower = 40, rangedAttackPower = 30, spellBonus = { damage = 25, healing = 10 },
  perPoint = { spellCritFromInt = 0.06, critFromAgi = 0.05, manaRegenFromSpi = 0.04, healthRegenFromSpi = 0.5 } } },
  ["Realm:No"] = { stats = { stats = { int = { total = 5 } }, perPoint = {} } } } }`

func realCharacter(pref string) character.Character {
	c := shadow(pref)
	c.Totals = character.ParseTotals(realStats, "Realm:Pw", "Pw")
	return c
}

func TestClientNumbersExplainTheAdvice(t *testing.T) {
	c := realCharacter(PrefSurvival)
	r := Check(c, nil, "")
	if want := "Shadow: Intellect first (+0.06% spell crit a point), then Stamina for safer pulls."; r.Insights[0] != want {
		t.Errorf("chip = %q", r.Insights[0])
	}
	eff := Effects(c.Class, c.Totals)
	if eff[Agility] != "+0.05% crit a point" || eff[Spirit] != "+0.04 mana a second, +0.5 health a second a point" || eff[Intellect] != "+0.06% spell crit a point" {
		t.Errorf("effects = %v", eff)
	}
	// Without the numbers the general words stay.
	c.Totals.Per = nil
	if r := Check(c, nil, ""); !strings.Contains(r.Insights[0], "your mana and spell crit") {
		t.Errorf("chip = %q", r.Insights[0])
	}
}

func TestStatsParseRealShape(t *testing.T) {
	tt := character.ParseTotals(realStats, "Realm:Pw", "Pw")
	if tt == nil || tt.Per == nil || *tt.Per.SpellCritFromInt != 0.06 || *tt.Per.CritFromAgi != 0.05 ||
		*tt.Per.ManaRegenFromSpi != 0.04 || *tt.Per.HealthRegenFromSpi != 0.5 {
		t.Fatalf("%+v", tt)
	}
	if *tt.Int != 90 || tt.Base["int"] != 50 || tt.Pos["int"] != 40 || tt.Neg["int"] != 0 || *tt.Spi != 60 || !tt.Any() {
		t.Errorf("five stats: %+v", tt)
	}
	if *tt.HealthMax != 700 || *tt.ManaMax != 900 || *tt.Crit != 3.5 || *tt.RangedCrit != 3.1 || *tt.SpellCrit != 4.8 ||
		*tt.ManaRegenBase != 6.5 || *tt.ManaRegenCasting != 2.1 || *tt.AttackPower != 40 || *tt.SpellHealing != 10 ||
		*tt.At != 1700000000 || *tt.Level != 20 {
		t.Errorf("rest: %+v", tt)
	}
	if no := character.ParseTotals(realStats, "Realm:No", "No"); no == nil || no.Per != nil || *no.Int != 5 {
		t.Errorf("empty perPoint: %+v", no)
	}
	if character.ParseTotals(realStats, "Other:Zz", "Zz") != nil {
		t.Error("unknown character")
	}
}

func TestSuffixTableStats(t *testing.T) {
	want := map[string]string{"eagle": "Stamina + Intellect", "owl": "Intellect + Spirit", "whale": "Stamina + Spirit",
		"bear": "Strength + Stamina", "tiger": "Strength + Agility", "monkey": "Agility + Stamina", "falcon": "Agility + Intellect",
		"wolf": "Agility + Spirit", "boar": "Strength + Spirit", "gorilla": "Strength + Intellect"}
	for k, w := range want {
		if s, ok := SuffixByKey(k); !ok || s.Words != w || len(s.Stats) != 2 {
			t.Errorf("%s = %+v", k, s)
		}
	}
}

// capture096 is the real shape of a level 24 Priest capture from addon 0.6.38: no perPoint, and base equal to total.
const capture096 = `WoWCompanionDB = { characters = { ["Realm:Pri"] = { name = "Pri", stats = {
  seenAt = 1700000000, level = 24,
  stats = { int = { base = 119, total = 119, pos = 30, neg = 0 }, sta = { base = 60, total = 60, pos = 0, neg = 0 },
    spi = { base = 80, total = 80, pos = 0, neg = 0 } },
  healthMax = 573, manaMax = 1790, meleeCrit = 3, spellCrit = { max = 5.86, schools = { [2] = 5.86 } },
  manaRegen = { base = 6.5, casting = 2.1 } } } } }`

func priest(manaMax string) character.Character {
	c := shadow(PrefSurvival)
	c.Class = "PRIEST"
	c.Totals = character.ParseTotals(strings.Replace(capture096, "manaMax = 1790", "manaMax = "+manaMax, 1), "Realm:Pri", "Pri")
	return c
}

func TestDerivedPerPointPassesCrossCheckForPriest(t *testing.T) {
	c := priest("1790")
	if c.Totals.Per != nil {
		t.Fatal("fixture has no perPoint")
	}
	ck, ok := CrossCheck("PRIEST", c.Totals)
	if !ok || !ck.OK() || ck.PredictedMana < 1700 || ck.PredictedMana > 1900 || ck.PredictedCrit < 4.9 || ck.PredictedCrit > 5.1 {
		t.Errorf("cross-check = %+v ok=%v", ck, ok)
	}
	eff := Effects(c.Class, c.Totals)
	if eff[Intellect] != "+15 mana, +0.04% spell crit a point" || eff[Stamina] != "+10 health a point" {
		t.Errorf("effects = %v", eff)
	}
	if r := Check(c, nil, ""); !strings.Contains(r.Insights[0], "+15 mana") {
		t.Errorf("chip = %q", r.Insights[0])
	}
}

func TestMismatchHidesDerivedNumbers(t *testing.T) {
	c := priest("3200")
	if ck, ok := CrossCheck("PRIEST", c.Totals); !ok || ck.ManaOK {
		t.Errorf("mana should disagree: %+v", ck)
	}
	if eff := Effects(c.Class, c.Totals); eff != nil {
		t.Errorf("effects = %v", eff)
	}
	if r := Check(c, nil, ""); strings.Contains(r.Insights[0], "+15") || !strings.Contains(r.Insights[0], "your mana and spell crit") {
		t.Errorf("chip = %q", r.Insights[0])
	}
	if Effects("WARRIOR", priest("1790").Totals) != nil {
		t.Error("no rule for a warrior's Intellect")
	}
}

func TestPreAddon096BaseIsCorrected(t *testing.T) {
	tt := priest("1790").Totals
	if tt.Base["int"] != 89 || tt.Base["sta"] != 60 {
		t.Errorf("base = %v", tt.Base)
	}
}
