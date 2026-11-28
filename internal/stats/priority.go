package stats

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// Order is the profile's priority: the stats that matter (weight at least fineWeight, the five primary
// stats and, for tanks, armor), highest weight first. The primary stat always leads (see Adjust); a tie
// keeps the table's order of the primary stats.
func (p Prof) Order() []string {
	var out []string
	for _, s := range append(append([]string{}, Primary...), Armor) {
		if p.Weights[s] >= fineWeight {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return p.Weights[out[i]] > p.Weights[out[j]] })
	return out
}

// Lead is the primary stat.
func (p Prof) Lead() string { return p.top() }

// gives: what each stat does for an archetype, in a few plain words, for the lead sentence.
var gives = map[string]map[string]string{
	archStr:    {Strength: "your attack power", Agility: "crits and rage", Stamina: "safer pulls", Armor: "fewer hits hurt"},
	archAgi:    {Agility: "your attack power and crits", Strength: "attack power", Stamina: "safer pulls", Armor: "fewer hits hurt"},
	archRanged: {Agility: "your shots and crits", Stamina: "safer pulls", Intellect: "more mana", Armor: "fewer hits hurt"},
	archFeral:  {Agility: "your attack power and crits", Strength: "attack power", Stamina: "safer pulls", Armor: "fewer hits hurt"},
	archCaster: {Intellect: "your mana and spell crit", Stamina: "safer pulls", Spirit: "faster mana back", Armor: "fewer hits hurt"},
	archHealer: {Intellect: "your mana pool", Spirit: "faster mana back", Stamina: "safer pulls", Armor: "fewer hits hurt"},
	archTank:   {Stamina: "your health", Strength: "block and threat", Agility: "dodge and armor", Armor: "fewer hits land", Intellect: "a little mana"},
}

func (p Profile) gives(stat string) string {
	if o, ok := classGives[classStat{p.Class, p.Arch, stat}]; ok {
		return o
	}
	return gives[p.Arch][stat]
}

// classStat keys a wording that differs from the archetype's for one class: a paladin or shaman has no rage to feed with crits,
// and a druid cannot block.
type classStat struct{ class, arch, stat string }

var classGives = map[classStat]string{
	{"PALADIN", archStr, Agility}: "crits",
	{"SHAMAN", archStr, Agility}:  "crits",
	{"DRUID", archTank, Strength}: "attack power and threat",
}

// classReasons are the longer reasons for the same classes.
var classReasons = map[classStat]string{
	{"PALADIN", archStr, Agility}: "more critical hits",
	{"SHAMAN", archStr, Agility}:  "more critical hits",
	{"DRUID", archTank, Strength}: "more attack power for holding enemies",
}

// Effects are the per-point numbers for a character, one phrase per stat ("+0.06% spell crit a
// point"): the client's own when the capture has them, else derived from class rules and shown only when
// they agree with the captured mana and spell crit (see CrossCheck). Empty when there are none to trust.
func Effects(class string, t *character.Totals) map[string]string {
	if t == nil {
		return nil
	}
	if t.Per == nil {
		return derivedEffects(class, t)
	}
	pp := t.Per
	out := map[string]string{}
	add := func(stat string, parts ...string) {
		var keep []string
		for _, s := range parts {
			if s != "" {
				keep = append(keep, s)
			}
		}
		if len(keep) > 0 {
			out[stat] = strings.Join(keep, ", ") + " a point"
		}
	}
	add(Intellect, plus(pp.SpellCritFromInt, "%s%% spell crit"))
	add(Agility, plus(pp.CritFromAgi, "%s%% crit"))
	add(Spirit, plus(pp.ManaRegenFromSpi, "%s mana a second"), plus(pp.HealthRegenFromSpi, "%s health a second"))
	return out
}

func plus(v *float64, format string) string {
	if v == nil || *v <= 0 {
		return ""
	}
	return "+" + fmt.Sprintf(format, trim(*v))
}

// trim is a number with at most two decimals and no trailing zeros: 15, 0.06, 1.5.
func trim(f float64) string {
	s := strconv.FormatFloat(f, 'f', 2, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

// lead is the first insight: the priority in one sentence, the primary stat first, "Shadow:
// Intellect first (your mana and spell crit), then Stamina for safer pulls." When the client's
// numbers are known for the lead stat they replace the general words.
func (p Prof) leadLine(eff map[string]string) string {
	order := p.Order()
	if len(order) == 0 {
		return ""
	}
	first := order[0]
	why := p.gives(first)
	if e := eff[first]; e != "" {
		why = e
	}
	base := fmt.Sprintf("%s: %s first", p.Label(), statWord(first))
	full := base
	if why != "" {
		full += " (" + why + ")"
	}
	short := base
	if g := p.gives(first); g != "" {
		short += " (" + g + ")"
	}
	if len(order) > 1 {
		second := order[1]
		then := ", then " + statWord(second)
		if g := p.gives(second); g != "" {
			then += " for " + g
		}
		return fit(full+then+".", short+then+".", base+then+".", base+".")
	}
	return fit(full+".", short+".", base+".")
}

func statWord(s string) string {
	if s == Armor {
		return "armor"
	}
	return Name(s)
}

// suffixLine names the top suffixes: "Look for 'of the Eagle' (Stamina + Intellect), or 'of the Owl'."
func suffixLine(top []Suffix) string {
	switch len(top) {
	case 0:
		return ""
	case 1:
		return fit(fmt.Sprintf("Look for '%s' (%s).", top[0].Name, top[0].Words), fmt.Sprintf("Look for '%s'.", top[0].Name))
	}
	return fit(
		fmt.Sprintf("Look for '%s' (%s), or '%s'.", top[0].Name, top[0].Words, top[1].Name),
		fmt.Sprintf("Look for '%s' or '%s'.", top[0].Name, top[1].Name))
}
