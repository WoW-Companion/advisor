package digest

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/store"
)

// keep_materials comes from the dataset's recipes (professions.KeepList). Only when the dataset has
// no recipes for the character's professions does it fall back to a name match: a bag item is kept when
// a whole word (or word run) of its name is one of its profession's fragments and no exclusion
// phrase appears in the name. Fragments are lower case. A profession missing here keeps nothing.
var professionFragments = map[string][]string{
	"alchemy":        {"herb", "vial", "peacebloom", "silverleaf", "earthroot", "mageroyal", "briarthorn", "bruiseweed", "stranglekelp", "kingsblood", "liferoot", "fadeleaf", "goldthorn", "grave moss", "sungrass"},
	"blacksmithing":  {"bar", "stone", "ore"},
	"cooking":        {"meat", "egg", "wing", "fin", "flesh", "steak", "spice"},
	"enchanting":     {"dust", "essence", "shard"},
	"engineering":    {"bar", "stone", "parts", "powder", "tube"},
	"first aid":      {"cloth"},
	"herbalism":      {"herb", "peacebloom", "silverleaf", "earthroot", "mageroyal", "briarthorn", "bruiseweed", "stranglekelp", "kingsblood", "liferoot", "fadeleaf", "goldthorn", "grave moss", "sungrass"},
	"jewelcrafting":  {"gem", "bar", "stone"},
	"leatherworking": {"leather", "hide", "scale", "thread"},
	"mining":         {"ore", "bar"},
	"skinning":       {"leather", "hide", "scale", "pelt"},
	"tailoring":      {"cloth", "thread", "bolt"},
}

// notMaterials names things that share a word with a reagent but are not one: quest currencies and
// class reagents. An item whose name contains any of these is never kept.
var notMaterials = []string{"blood shard", "soul shard", "mark of honor", "hearthstone", "soulstone", "firestone", "spellstone"}

// words is the name in lower case with every run of non-letters as one space, padded for whole-word search.
func words(s string) string {
	f := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool { return !unicode.IsLetter(r) })
	return " " + strings.Join(f, " ") + " "
}

// isMaterial reports whether an item name is a reagent for any of the professions.
func isMaterial(itemName string, professions []string) bool {
	w := words(itemName)
	for _, x := range notMaterials {
		if strings.Contains(w, words(x)) {
			return false
		}
	}
	for _, p := range professions {
		for _, frag := range professionFragments[strings.ToLower(p)] {
			if strings.Contains(w, words(frag)) {
				return true
			}
		}
	}
	return false
}

// materials are the bag and bank items the character's professions use, most numerous first: the
// reagents of recipes still worth making, or the name guess when the dataset lacks recipes.
func materials(c character.Character, ds store.DataStore, craftFor map[int64]string) []Line {
	keep, err := professions.KeepList(c, ds)
	if err != nil {
		return guessMaterials(c, craftFor)
	}
	var out []Line
	for _, k := range keep {
		l := Line{Text: clip(fmt.Sprintf("%s x%d", k.Name, k.Count)), Item: int(k.ItemID), Asks: materialAsks(k.Name), Ref: itemRef(int(k.ItemID))}
		forItem(&l, craftFor[k.ItemID])
		out = append(out, l)
	}
	return out
}

// forItem marks a material as needed for a craftable upgrade: "for <item>" on the text when it fits,
// and always as the why.
func forItem(l *Line, item string) {
	if item == "" {
		return
	}
	if t := l.Text + ", for " + item; utf8.RuneCountInString(t) <= MaxText {
		l.Text = t
	}
	l.Why = firstWhy("For " + item)
}

// guessMaterials is the name-fragment fallback, over the bags.
func guessMaterials(c character.Character, craftFor map[int64]string) []Line {
	var profs []string
	for _, p := range c.Professions {
		profs = append(profs, p.Name)
	}
	type stack struct {
		name  string
		id, n int
	}
	var kept []stack
	for _, it := range c.Bags {
		if it.Count > 0 && it.Name != "" && isMaterial(it.Name, profs) {
			kept = append(kept, stack{it.Name, it.ItemID, it.Count})
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].n != kept[j].n {
			return kept[i].n > kept[j].n
		}
		return kept[i].name < kept[j].name
	})
	var out []Line
	for _, k := range kept {
		l := Line{Text: clip(fmt.Sprintf("%s x%d", k.name, k.n)), Item: k.id, Asks: materialAsks(k.name), Ref: itemRef(k.id)}
		forItem(&l, craftFor[int64(k.id)])
		out = append(out, l)
	}
	return out
}
