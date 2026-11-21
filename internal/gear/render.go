package gear

import (
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
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
	req := "level unknown"
	if c.Item.RequiredLevel > 0 {
		req = fmt.Sprintf("level %d", c.Item.RequiredLevel)
	}
	return fmt.Sprintf("%s (ilvl %d, requires %s; %s, by %s): %s", name, c.Item.ItemLevel, req, gain, c.Basis, src)
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
	if r.ItemLevelOnly {
		fmt.Fprintf(&b, "Ranked by item level only (quality breaks ties): %s.\n", r.Reason)
	} else {
		fmt.Fprintf(&b, "Ranked by item level weighted by %s stat types; advisor defaults, not authoritative (%s). The dataset has stat types but not their sizes.\n", r.Spec, r.Rationale)
	}
	if len(r.Slots) == 0 {
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
	for _, slot := range append(order, extra...) {
		cands := r.Slots[slot]
		if len(cands) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n", slot)
		for i, cd := range cands {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, cd.Describe())
		}
	}
	return b.String()
}
