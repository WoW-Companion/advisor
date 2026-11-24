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
	return res
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
