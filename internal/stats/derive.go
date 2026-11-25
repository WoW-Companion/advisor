package stats

import (
	"math"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// The client this addon runs on has no per-point stat APIs, so "what a point is worth for you" is derived
// from class rules and then cross-checked against the character's own captured mana and spell crit. The
// numbers are shown only when both checks agree; otherwise the advice keeps the stat priority and the
// general words, never a wrong number.
//
// Rule table (Classic-era rules; the per-class figures are curated approximations of the community-
// documented tables, close enough for the cross-check and not a game-data source):
//
//	rule                        value                                   source
//	mana per Intellect          15, for every point above the first 20  Classic Era class rules (wowpedia "Intellect")
//	health per Stamina          10, for every point above the first 20  Classic Era class rules (wowpedia "Stamina")
//	base mana at level L        manaPerLevel[class] * L                 approximation of the per-class base mana tables
//	spell crit per Intellect    1 / (intPerCritAt60[class] * L / 60)    wowpedia "Spell critical strike" Classic ratios,
//	                                                                    scaled down linearly by level
//
// Talents and gear crit move the captured numbers off the prediction, hence the tolerances below.
const (
	manaPerInt      = 15.0
	healthPerSta    = 10.0
	firstFree       = 20.0 // the first 20 points of Intellect/Stamina give 1 mana/health each, not the full rate
	manaTolerance   = 0.05 // predicted manaMax within 5% of the captured one
	critTolerance   = 0.05 // predicted spell crit within 5% (or critSlackPoints) of the captured one
	critSlackPoints = 1.0  // talents add crit the table cannot know
)

type casterRule struct {
	manaPerLevel   float64 // base mana per character level
	intPerCritAt60 float64 // Intellect needed for 1% spell crit at level 60
}

var casterRules = map[string]casterRule{
	"PRIEST":  {12.5, 59.5},
	"MAGE":    {12.5, 59.5},
	"WARLOCK": {12.5, 60.6},
	"DRUID":   {12.5, 60},
	"SHAMAN":  {12.0, 59.2},
	"PALADIN": {11.0, 54},
}

// Agreement is the cross-check of the class rules against a character's capture.
type Agreement struct {
	PredictedMana, PredictedCrit float64
	ManaOK, CritOK               bool
}

// OK reports whether the rules agree with the capture on both mana and spell crit.
func (c Agreement) OK() bool { return c.ManaOK && c.CritOK }

func intPerCrit(r casterRule, level float64) float64 { return r.intPerCritAt60 * level / 60 }

func above20(v float64) float64 { return math.Max(v-firstFree, 0) }

// CrossCheck predicts manaMax and spell crit from the class rules and compares them with the capture.
// ok is false when the class has no rule or the capture lacks Intellect, level, mana or spell crit.
func CrossCheck(class string, t *character.Totals) (Agreement, bool) {
	r, found := casterRules[strings.ToUpper(class)]
	if !found || t == nil || t.Int == nil || t.Level == nil || t.ManaMax == nil || t.SpellCrit == nil || *t.Level < 1 {
		return Agreement{}, false
	}
	lvl := float64(*t.Level)
	c := Agreement{PredictedMana: r.manaPerLevel*lvl + firstFree + manaPerInt*above20(*t.Int)}
	// Intellect's first 20 points give 1 mana each, so the 20 sits in the prediction once.
	c.PredictedCrit = *t.Int / intPerCrit(r, lvl)
	c.ManaOK = *t.ManaMax > 0 && math.Abs(c.PredictedMana-*t.ManaMax) <= manaTolerance**t.ManaMax
	d := math.Abs(c.PredictedCrit - *t.SpellCrit)
	c.CritOK = *t.SpellCrit > 0 && (d <= critTolerance**t.SpellCrit || d <= critSlackPoints)
	return c, true
}

// derivedEffects are the per-point phrases derived from the class rules, only when the cross-check passes.
func derivedEffects(class string, t *character.Totals) map[string]string {
	c, ok := CrossCheck(class, t)
	if !ok || !c.OK() {
		return nil
	}
	r := casterRules[strings.ToUpper(class)]
	crit := 1 / intPerCrit(r, float64(*t.Level))
	return map[string]string{
		Intellect: "+" + trim(manaPerInt) + " mana, +" + trim(crit) + "% spell crit a point",
		Stamina:   "+" + trim(healthPerSta) + " health a point",
	}
}
