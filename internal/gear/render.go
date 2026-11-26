package gear

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/vendors"
	"github.com/WoW-Companion/advisor/internal/wording"
)

func (s Source) String() string {
	var b strings.Builder
	name := s.Name
	if name == "" {
		name = fmt.Sprintf("%s %d", s.Kind, s.ID)
	} else if s.Kind != "creature" {
		name += " (" + s.Kind + ")"
	}
	if s.Everloot && s.Name != "" {
		name += " (name from Everloot)"
	}
	b.WriteString(name)
	switch {
	case s.MinLevel != nil && s.MaxLevel != nil && *s.MinLevel != *s.MaxLevel:
		fmt.Fprintf(&b, ", level %d-%d", *s.MinLevel, *s.MaxLevel)
	case s.MinLevel != nil:
		fmt.Fprintf(&b, ", level %d", *s.MinLevel)
	case s.MaxLevel != nil:
		fmt.Fprintf(&b, ", level %d", *s.MaxLevel)
	}
	b.WriteString(", zone unknown")
	if s.Chance != nil {
		fmt.Fprintf(&b, ", drops %.1f%% of kills", *s.Chance)
	} else {
		b.WriteString(", drop chance unknown")
	}
	return b.String()
}

// LevelText is how soon the character can equip the candidate: "requires level N" when they can
// now, "at level N" when they must reach it first, and "(estimated)" when N is a guess from item
// level because the dataset holds no required level.
func (c Candidate) LevelText() string {
	t := fmt.Sprintf("requires level %d", c.Item.ReqLevel)
	if c.Soon {
		t = fmt.Sprintf("at level %d", c.Item.ReqLevel)
	}
	if c.Item.Estimated {
		t += " (estimated)"
	}
	return t
}

// Describe is the one-line text of a candidate.
func (c Candidate) Describe() string {
	name := c.Item.Name
	if name == "" {
		name = fmt.Sprintf("item %d", c.Item.ID)
	}
	gain := fmt.Sprintf("+%.1f", c.Delta)
	if c.Empty {
		gain = "empty slot"
	}
	src := "source unknown"
	if len(c.Sources) > 0 {
		parts := make([]string, len(c.Sources))
		for i, s := range c.Sources {
			parts[i] = s.String()
		}
		src = "from " + strings.Join(parts, "; ")
	}
	if c.Craft != nil {
		if l := c.Craft.Line(); l != "" {
			src += "; " + l
		}
	}
	if c.Vendor != nil {
		src += "; " + c.Vendor.Line() + " (seen by players on Everloot)"
	}
	basis := "required level"
	if c.Basis == BasisStats {
		basis = BasisStats
	}
	return fmt.Sprintf("%s (%s; %s, by %s): %s", name, c.LevelText(), gain, basis, src)
}

// Render is the plain-text answer. Slots are listed in the capture's slot order.
func Render(c character.Character, r Result) string {
	name := c.Name
	if name == "" {
		name = c.Key
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Gear upgrades for %s", name)
	if c.Level != nil {
		fmt.Fprintf(&b, " (level %d %s)", *c.Level, strings.ToLower(c.Class))
	}
	b.WriteString(":\n")
	if len(r.Bags) > 0 {
		b.WriteString("Equip these now (already in your bags, free):\n")
		for _, u := range r.Bags {
			fmt.Fprintf(&b, "- %s\n", u.Line())
		}
	}
	if r.ItemLevelOnly {
		fmt.Fprintf(&b, "Ranked by the level needed to wear it only (quality breaks ties): %s.\n", r.Reason)
	} else {
		fmt.Fprintf(&b, "Ranked by the gain over what is worn, weighted by %s stat types and listed by the level needed to wear it; advisor defaults, not authoritative (%s).\n", r.Spec, r.Rationale)
	}
	if len(r.Slots) == 0 {
		if len(r.Bags) > 0 {
			return b.String()
		}
		b.WriteString("No upgrades found for the items the dataset holds.\n")
		return b.String()
	}
	order := append([]string{}, brief.Slots...)
	var extra []string
	known := map[string]bool{}
	for _, s := range order {
		known[s] = true
	}
	for s := range r.Slots {
		if !known[s] {
			extra = append(extra, s)
		}
	}
	sort.Strings(extra)
	credit := false
	var reach []Candidate      // crafts the character's skill cannot make, listed apart
	soon := map[int][]string{} // the level an upgrade needs -> its lines
	for _, slot := range append(order, extra...) {
		cands := r.Slots[slot]
		if len(cands) == 0 {
			continue
		}
		var now []Candidate
		for _, cd := range cands {
			if cd.outOfReach() {
				reach = append(reach, cd)
				continue
			}
			if cd.Soon {
				soon[cd.Item.ReqLevel] = append(soon[cd.Item.ReqLevel], fmt.Sprintf("  %s: %s\n", slot, cd.Describe()))
				credit = credit || cd.Vendor != nil
			} else {
				now = append(now, cd)
			}
		}
		if len(now) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n", slot)
		for i, cd := range now {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, cd.Describe())
			credit = credit || cd.Vendor != nil
		}
	}
	// Upgrades the character's level can't use yet are never ranked as within reach: they follow the list.
	var levels []int
	for l := range soon {
		levels = append(levels, l)
	}
	sort.Ints(levels)
	for _, l := range levels {
		fmt.Fprintf(&b, "Soon (level %d):\n%s", l, strings.Join(soon[l], ""))
	}
	if len(reach) > 0 || hasCraft(r) {
		b.WriteString("Call an item craftable only when its line says you can make it now.\n")
	}
	for _, l := range beyondReach(reach) {
		b.WriteString(l + "\n")
	}
	if credit {
		b.WriteString("Sources: " + vendors.Credit + ".\n")
	}
	return b.String()
}

// outOfReach is a crafted candidate the character cannot make and that comes from nowhere else: it is
// never listed as craftable.
func (c Candidate) outOfReach() bool {
	return c.Craft != nil && c.Craft.Status != CraftNow && len(c.Sources) == 0 && c.Vendor == nil
}

// hasCraft is true when a ranked candidate is made by a profession.
func hasCraft(r Result) bool {
	for _, cs := range r.Slots {
		for _, cd := range cs {
			if cd.Craft != nil {
				return true
			}
		}
	}
	return false
}

// LikelyWording is "likely better (item level 40 vs 36)": the claim without a stat basis, with the
// numbers it rests on.
func (u BagUpgrade) LikelyWording() string {
	return fmt.Sprintf("likely better (item level %d vs %d)", u.Item.ItemLevel, u.ReplacesILvl)
}

// Line is the upgrade in a sentence: "Feet: Copper Chain Boots, in your bags; equip it now (replaces
// Hiking Boots; likely better)".
func (u BagUpgrade) Line() string {
	s := wording.SlotName(u.Slot, u.Item.Slot) + ": " + u.Name + ", in your bags; equip it now"
	if u.Need != nil {
		s = wording.SlotName(u.Slot, u.Item.Slot) + ": " + u.Name + ", in your bags; " + u.Need.Line()
	}
	var more []string
	if u.Replaces != "" {
		more = append(more, "replaces "+u.Replaces)
	}
	if u.Likely && !u.Empty {
		more = append(more, u.LikelyWording())
	} else if !u.Empty {
		more = append(more, "better")
	}
	if len(more) > 0 {
		s += " (" + strings.Join(more, "; ") + ")"
	}
	return s
}
