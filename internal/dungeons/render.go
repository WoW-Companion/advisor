package dungeons

import (
	"fmt"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
)

// maxDrops is how many drops a rendered pick lists.
const maxDrops = 5

func (d Drop) String() string {
	name := d.Item
	if name == "" {
		name = fmt.Sprintf("item %d", d.ItemID)
	}
	gain := fmt.Sprintf("+%.1f", d.Delta)
	if d.Empty {
		gain = "empty slot"
	}
	chance := "drop chance unknown"
	if d.Chance != nil {
		chance = fmt.Sprintf("drops %.1f%% of kills", *d.Chance)
	}
	return fmt.Sprintf("%s (%s, %s) from %s, %s", name, d.Slot, gain, d.Boss, chance)
}

func (p DungeonPick) describe(b *strings.Builder, n int) {
	lvl := "level unknown"
	if p.MinLevel != nil && p.MaxLevel != nil {
		lvl = fmt.Sprintf("level %d-%d", *p.MinLevel, *p.MaxLevel)
	}
	size := ""
	if p.MaxPlayers != nil {
		size = fmt.Sprintf(", %d players", *p.MaxPlayers)
	}
	prefix := "  - "
	if n > 0 {
		prefix = fmt.Sprintf("  %d. ", n)
	}
	fmt.Fprintf(b, "%s%s (%s%s): %d upgrade(s)\n", prefix, p.Name, lvl, size, p.Upgrades)
	for i, d := range p.Drops {
		if i == maxDrops {
			fmt.Fprintf(b, "       and %d more\n", len(p.Drops)-maxDrops)
			break
		}
		b.WriteString("       " + d.String() + "\n")
	}
	if len(p.UnlinkedBosses) > 0 {
		b.WriteString("       boss drops not linked in this dataset: " + strings.Join(p.UnlinkedBosses, ", ") + "\n")
	}
	for _, q := range p.Quests {
		fmt.Fprintf(b, "       open quest: %s (%d)\n", q.Title, q.ID)
	}
}

// Render is the plain-text plan, showing at most limit ranked picks (all when limit <= 0).
func Render(c character.Character, r Result, limit int) string {
	name := c.Name
	if name == "" {
		name = c.Key
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Dungeon plan for %s", name)
	if c.Level != nil {
		fmt.Fprintf(&b, " (level %d %s)", *c.Level, strings.ToLower(c.Class))
	}
	b.WriteString(":\n")
	if r.ItemLevelOnly {
		fmt.Fprintf(&b, "Upgrades ranked by item level only: %s.\n", r.Reason)
	} else {
		fmt.Fprintf(&b, "Upgrades weighed for %s; advisor defaults, not authoritative.\n", r.Spec)
	}
	if len(r.Picks) == 0 {
		fmt.Fprintf(&b, "No dungeon in the dataset has a level range within %d levels of the character.\n", LevelWindow)
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
		b.WriteString("Level unknown (unranked; the dataset holds no level range):\n")
		for _, p := range r.Unknown {
			p.describe(&b, 0)
		}
	}
	b.WriteString("Bring: gear for the slots above; bosses with unknown drops are not guessed. Group size is the dataset's maximum players.\n")
	return b.String()
}
