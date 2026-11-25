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

func TestClientNumbersExplainTheAdvice(t *testing.T) {
	c := shadow(PrefSurvival)
	mana, crit, hp := 15.0, 0.06, 10.0
	c.Totals.Per = &character.PerPoint{IntMana: &mana, IntCrit: &crit, StaHealth: &hp}
	r := Check(c, nil, "")
	if want := "Shadow: Intellect first (+15 mana, +0.06% spell crit a point), then Stamina for safer pulls."; r.Insights[0] != want {
		t.Errorf("chip = %q", r.Insights[0])
	}
	if got := Effects(c.Totals)[Stamina]; got != "+10 health a point" {
		t.Errorf("stamina effect = %q", got)
	}
	// Without the numbers the general words stay.
	c.Totals.Per = nil
	if r := Check(c, nil, ""); !strings.Contains(r.Insights[0], "your mana and spell crit") {
		t.Errorf("chip = %q", r.Insights[0])
	}
}

func TestPerPointParse(t *testing.T) {
	src := `WoWCompanionDB = { stats = { ["R:Pw"] = { int = 90, per = { int_mana = 15, int_crit = 0.06 } }, ["R:No"] = { int = 5, per = {} } } }`
	tt := character.ParseTotals(src, "R:Pw", "Pw")
	if tt == nil || tt.Per == nil || *tt.Per.IntMana != 15 || tt.Per.AgiCrit != nil {
		t.Errorf("%+v", tt)
	}
	if no := character.ParseTotals(src, "R:No", "No"); no == nil || no.Per != nil {
		t.Errorf("empty per table: %+v", no)
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
