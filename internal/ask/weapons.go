package ask

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
)

// NoWeaponFacts stands in for the weapon-skill facts when the dataset has no weapon-skill tables.
const NoWeaponFacts = "Weapon skills: this game data has no weapon-skill facts. For any question about which weapons the class can use or where to train a weapon skill, say you don't have those facts and the player should update the game data. Never answer it from memory: WoW Forever differs from Classic."

// weaponWords make a question one about weapon skills, the weapons a class can use, or where to train.
var weaponWords = regexp.MustCompile(`(?i)\b(weapons?|wield|dual wield|staffs?|staves|axes?|swords?|maces?|daggers?|polearms?|bows?|guns?|crossbows?|wands?|fist|thrown|weapon masters?|weapon skills?|(?:one|two|1|2)[- ]?(?:hand(?:er|ers|ed)?|h))\b`)

// dualWieldWords make a weapon question one about dual wielding.
var dualWieldWords = regexp.MustCompile(`(?i)\bdual[- ]?wield`)

// WeaponQuestion reports whether a question is about which weapons a class can use or where to train a
// weapon skill.
func WeaponQuestion(text string) bool { return weaponWords.MatchString(text) }

// weaponRule is added to the prompt of a weapon question: the facts are the only source.
const weaponRule = " Weapon and armour rules come from the weapon-skill facts above, never from your general knowledge: WoW Forever differs from Classic " +
	"(for example priests can use Staves but must buy the skill from a Weapon Master, rogues learn One-Handed Axes, druids Polearms). " +
	"Answer which weapons the class can use, whether the character has the skill, and where to train it only from those facts, naming the masters and cost they list. " +
	"If the facts say there are none, say plainly that you don't have weapon-skill facts (this overrides the wording rule about what is missing). "

// WeaponFacts is the weapon-skill section of the facts: the character's captured weapon skills, the
// class's learnable skills, and for each learnable skill the character lacks the masters of their
// faction with cost and level. Masters are listed only for a weapon question, to spare the budget.
func WeaponFacts(c character.Character, ds store.DataStore, text string) string {
	f := gear.LoadWeaponSkills(c, ds)
	if ds == nil || !f.Have {
		if ds == nil || !WeaponQuestion(text) {
			return ""
		}
		return NoWeaponFacts
	}
	var have []string
	for _, s := range c.Skills {
		if strings.EqualFold(s.Group, "Weapon Skills") {
			t := s.Name
			if s.Rank != nil && s.Max != nil {
				t += fmt.Sprintf(" %d/%d", *s.Rank, *s.Max)
			}
			have = append(have, t)
		}
	}
	var learn []string
	for id := range f.Learnable {
		learn = append(learn, f.Names[id])
	}
	sort.Strings(learn)
	var b strings.Builder
	b.WriteString("Weapon skills (from the game data for WoW Forever; use these, never your memory of Classic):\n")
	switch {
	case !c.SkillsCaptured:
		b.WriteString("- The character's weapon skills were not captured, so what they have learned is unknown.\n")
	case len(have) == 0:
		b.WriteString("- The character has learned no weapon skills.\n")
	default:
		b.WriteString("- The character has learned: " + strings.Join(have, ", ") + ".\n")
	}
	fmt.Fprintf(&b, "- The class can learn: %s.\n", strings.Join(learn, ", "))
	if dualWieldWords.MatchString(text) {
		b.WriteString(dualWieldFact(c, ds))
	}
	lacking := f.Lacking(c)
	if len(lacking) == 0 {
		return strings.TrimRight(b.String(), "\n")
	}
	if !WeaponQuestion(text) {
		var names []string
		for _, id := range lacking {
			names = append(names, f.Names[id])
		}
		b.WriteString("- Learnable but not learned yet: " + strings.Join(names, ", ") + " (the weapon masters are in the facts when the player asks).\n")
		return strings.TrimRight(b.String(), "\n")
	}
	b.WriteString("- Learnable, not learned yet; train at a Weapon Master (never the class trainer):\n")
	for _, id := range lacking {
		ms := f.MastersFor(id)
		if len(ms) == 0 {
			if f.Default[id] {
				fmt.Fprintf(&b, "  - %s: the class starts knowing it and no Weapon Master teaches it (no master in the data), so it is no skill to train: look for it in the skill list\n", f.Names[id])
				continue
			}
			fmt.Fprintf(&b, "  - %s: no master in the data\n", f.Names[id])
			continue
		}
		// One group per city and price, so "Hanashi, Sayoc in Orgrimmar, Valley of Honor" is one clause.
		type key struct {
			where string
			cost  int64
			lvl   int
		}
		var order []key
		names := map[key][]string{}
		for _, t := range ms {
			k := key{t.Master.Where(), t.Cost, t.MinLevel}
			if _, ok := names[k]; !ok {
				order = append(order, k)
			}
			names[k] = append(names[k], t.Master.Name)
		}
		var parts []string
		for _, k := range order {
			s := strings.Join(names[k], " or ") + " in " + k.where + " for " + gear.CostText(k.cost)
			if k.lvl > 1 {
				s += fmt.Sprintf(" (level %d)", k.lvl)
			}
			parts = append(parts, s)
		}
		fmt.Fprintf(&b, "  - %s: %s\n", f.Names[id], strings.Join(parts, "; "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// dualWieldFact is the line for a question about dual wielding: Dual Wield is a class spell, not a weapon skill, so the Weapon Masters
// do not teach it; the level is the class's own, and the dataset holds no price for it.
func dualWieldFact(c character.Character, ds store.DataStore) string {
	level, ok := gear.DualWieldLevel(c.Class, ds)
	if !ok {
		return "- Dual Wield: this class cannot dual wield; name no level for it.\n"
	}
	return fmt.Sprintf("- Dual Wield is a class trainer spell, not a weapon skill, so a Weapon Master does not teach it: this class learns it at level %d. The game data holds no price for it, so name none.\n", level)
}
