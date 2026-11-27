package professions

import (
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Advice is one suggested pair of primary professions with a one-line reason.
type Advice struct {
	A, B   string
	Reason string
}

// classPairs are the pairs that suit a class's armor and play, by lower-case class name.
var classPairs = map[string][]string{
	"mage": {"Tailoring"}, "priest": {"Tailoring"}, "warlock": {"Tailoring"},
	"rogue": {"Skinning", "Leatherworking"}, "druid": {"Skinning", "Leatherworking"}, "hunter": {"Skinning", "Leatherworking"},
	"warrior": {"Mining", "Blacksmithing"}, "paladin": {"Mining", "Blacksmithing"}, "shaman": {"Skinning", "Leatherworking", "Mining"},
}

// Pairings suggests profession pairs from the curated notes, ordered by what the character already
// has, their class and how they play; any pair is fine and the answer says so.
func Pairings(c character.Character, ds store.DataStore) []Advice {
	n := LoadNotes(ds)
	type scoredPair struct {
		Advice
		score int
	}
	var out []scoredPair
	for _, p := range n.Pairings {
		s := 0
		why := p.Text
		for _, h := range c.Professions {
			if strings.EqualFold(h.Name, p.A) || strings.EqualFold(h.Name, p.B) {
				s += 4
				why += " You already have " + h.Name + "."
				break
			}
		}
		for _, w := range classPairs[strings.ToLower(c.Class)] {
			if strings.EqualFold(w, p.A) || strings.EqualFold(w, p.B) {
				s += 2
				break
			}
		}
		if c.Profile != nil {
			gather := strings.EqualFold(p.A, "Mining") || strings.EqualFold(p.A, "Skinning") || strings.EqualFold(p.A, "Herbalism")
			switch c.Profile.Focus {
			case "leveling":
				if gather {
					s++
					why += " Gathering as you level costs nothing extra."
				}
			case "dungeons", "pvp":
				if strings.EqualFold(p.B, "Alchemy") || strings.EqualFold(p.A, "Tailoring") {
					s++
					why += " Potions and enchants help in group fights."
				}
			}
		}
		out = append(out, scoredPair{Advice{p.A, p.B, why}, s})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].score > out[j].score })
	res := make([]Advice, len(out))
	for i, o := range out {
		res[i] = o.Advice
	}
	if held := heldPrimaries(c, n); len(held) >= maxPrimary {
		res = fullSlots(res, held)
	}
	return res
}

// maxPrimary is how many primary professions a character holds (the notes' slots rule); Cooking, Fishing and First Aid
// do not count.
const maxPrimary = 2

// heldPrimaries are the primary professions the character has, in the capture's order.
func heldPrimaries(c character.Character, n Notes) []string {
	var out []string
	for _, h := range c.Professions {
		l := strings.ToLower(h.Name)
		if h.Name == "" || n.Secondary[l] || l == "cooking" || l == "fishing" || l == "first aid" {
			continue
		}
		out = append(out, h.Name)
	}
	return out
}

// lossNote is said when a change would drop a profession: the dataset holds no skill lost, recipes forgotten or cost, so none is stated.
const lossNote = " The game data does not say what dropping a profession costs (skill or recipes); check in the game before you unlearn one."

// fullSlots rewrites the advice for a character whose two primary slots are taken: what the pair they hold is, first, and
// every other pair as a change that means dropping one of them. A pair the notes do not list is said to have no note, not
// praised or faulted.
func fullSlots(res []Advice, held []string) []Advice {
	a, b := held[0], held[1]
	same := func(x Advice) bool {
		return strings.EqualFold(x.A, a) && strings.EqualFold(x.B, b) || strings.EqualFold(x.A, b) && strings.EqualFold(x.B, a)
	}
	first := Advice{A: a, B: b, Reason: "Both of your primary slots are taken by " + a + " and " + b + ". The game data has no pairing note for this pair." + lossNote}
	var out []Advice
	for _, x := range res {
		if same(x) {
			reason := strings.ReplaceAll(strings.ReplaceAll(x.Reason, " You already have "+a+".", ""), " You already have "+b+".", "")
			first = Advice{A: x.A, B: x.B, Reason: reason + " You already have both." + lossNote}
			continue
		}
		x.Reason += switchText(x, a, b)
		out = append(out, x)
	}
	return append([]Advice{first}, out...)
}

// switchText says what taking a pair costs a character whose slots hold a and b: a pair that shares one of them keeps it and
// drops only the other; a pair that shares none drops both. Skill already earned in a dropped profession is not stated: the
// dataset holds nothing on it.
func switchText(x Advice, a, b string) string {
	const lead = " Both your primary slots are taken"
	for _, kept := range [][2]string{{a, b}, {b, a}} {
		if strings.EqualFold(x.A, kept[0]) || strings.EqualFold(x.B, kept[0]) {
			add := x.A
			if strings.EqualFold(x.A, kept[0]) {
				add = x.B
			}
			return lead + ": this keeps " + kept[0] + " and means dropping " + kept[1] + " for " + add + "."
		}
	}
	return lead + ", so this means dropping both " + a + " and " + b + "."
}

// RenderPairings is the plain text of pairing advice.
func RenderPairings(a []Advice) string {
	if len(a) == 0 {
		return "Any two primary professions are fine: pick the ones you enjoy, and keep Cooking, Fishing and First Aid on the side, since they cost no slot.\n"
	}
	var b strings.Builder
	b.WriteString("Common pairings, best fit first (any two are fine; these just save effort):\n")
	for i, x := range a {
		b.WriteString(itoa(i+1) + ". " + x.A + " + " + x.B + ": " + x.Reason + "\n")
	}
	b.WriteString("Cooking, Fishing and First Aid cost no slot, so take them all.\n")
	return b.String()
}
