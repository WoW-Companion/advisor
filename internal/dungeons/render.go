package dungeons

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// maxDrops is how many drops a rendered pick lists.
const maxDrops = 3

func (d Drop) String() string {
	name := d.Item
	if name == "" {
		name = fmt.Sprintf("item %d", d.ItemID)
	}
	gain := fmt.Sprintf("+%.1f", d.Delta)
	if d.Empty {
		gain = "fills an empty slot"
	}
	s := fmt.Sprintf("%s (%s, %s) from %s", name, d.Slot, gain, d.Boss)
	if d.Chance != nil {
		s += fmt.Sprintf(", drops %.0f%% of kills", *d.Chance)
	}
	return s
}

// levels words a pick's range; a curated range is "about".
func (p DungeonPick) levels() string {
	if p.MinLevel == nil || p.MaxLevel == nil {
		return ""
	}
	if p.Curated {
		return fmt.Sprintf("about levels %d\u2013%d", *p.MinLevel, *p.MaxLevel)
	}
	return fmt.Sprintf("levels %d\u2013%d", *p.MinLevel, *p.MaxLevel)
}

func (p DungeonPick) where() string {
	if p.EntranceZone == "" {
		return ""
	}
	return ", in " + p.EntranceZone
}

func (p DungeonPick) describe(b *strings.Builder, n int) {
	prefix := "  - "
	if n > 0 {
		prefix = fmt.Sprintf("  %d. ", n)
	}
	var meta []string
	if l := p.levels(); l != "" {
		meta = append(meta, l)
	}
	if p.EntranceZone != "" {
		meta = append(meta, "entrance in "+p.EntranceZone)
	}
	if p.MaxPlayers != nil {
		meta = append(meta, fmt.Sprintf("%d players", *p.MaxPlayers))
	}
	fmt.Fprintf(b, "%s%s", prefix, p.Name)
	if len(meta) > 0 {
		fmt.Fprintf(b, " (%s)", strings.Join(meta, "; "))
	}
	b.WriteString("\n")
	if len(p.Pickups) > 0 {
		b.WriteString("       Quests to grab first:\n")
		for _, q := range p.Pickups {
			line := "         " + q.Title
			if q.Giver != "" {
				line += " (from " + q.Giver + ")"
			}
			b.WriteString(line + "\n")
		}
	}
	for _, q := range p.Quests {
		fmt.Fprintf(b, "       In your log: %s\n", q.Title)
	}
	if len(p.Drops) > 0 {
		b.WriteString("       Drops for you:\n")
	}
	for i, d := range p.Drops {
		if i == maxDrops {
			fmt.Fprintf(b, "         and %d more\n", len(p.Drops)-maxDrops)
			break
		}
		b.WriteString("         " + d.String() + "\n")
	}
	if len(p.UnlinkedBosses) > 0 {
		b.WriteString("       Drops not known yet: " + strings.Join(p.UnlinkedBosses, ", ") + "\n")
	}
}

// Render is the plain-text plan, showing at most limit ranked picks (all when limit <= 0). It
// leads with the answer and never narrates where the data came from.
func Render(c character.Character, r Result, limit int) string {
	name := c.Name
	if name == "" {
		name = c.Key
	}
	var b strings.Builder
	if f := r.First; f != nil {
		fmt.Fprintf(&b, "Too low for dungeons yet. The first one is %s (%s)%s: %d more level(s) to go.\n", f.Name, f.levels(), f.where(), r.LevelsToGo)
		if r.Meanwhile != "" {
			fmt.Fprintf(&b, "Meanwhile: %s.\n", r.Meanwhile)
		}
		if len(f.Pickups) > 0 {
			b.WriteString("Quests to grab there:\n")
			for _, q := range f.Pickups {
				line := "  " + q.Title
				if q.Giver != "" {
					line += " (from " + q.Giver + ")"
				}
				b.WriteString(line + "\n")
			}
		}
		return b.String()
	}
	if len(r.Picks) == 0 {
		b.WriteString("No dungeon suits this level right now.\n")
	} else {
		fmt.Fprintf(&b, "Dungeons for %s", name)
		if c.Level != nil {
			fmt.Fprintf(&b, " (level %d %s)", *c.Level, strings.ToLower(c.Class))
		}
		b.WriteString(":\n")
		if r.ItemLevelOnly {
			b.WriteString("Drops are ranked by item level.\n")
		}
	}
	picks := r.Picks
	if limit > 0 && len(picks) > limit {
		picks = picks[:limit]
	}
	for i, p := range picks {
		p.describe(&b, i+1)
	}
	if len(r.Picks) > len(picks) {
		fmt.Fprintf(&b, "%d more dungeon(s) fit.\n", len(r.Picks)-len(picks))
	}
	if len(r.Unknown) > 0 {
		var ns []string
		for _, p := range r.Unknown {
			ns = append(ns, p.Name)
		}
		b.WriteString("Level not known yet: " + strings.Join(ns, ", ") + "\n")
	}
	return b.String()
}
