package route

import (
	"cmp"
	"fmt"
	"github.com/WoW-Companion/advisor/internal/questie"
	"regexp"
	"strings"

	"github.com/WoW-Companion/advisor/internal/wording"
)

var (
	countAfter  = regexp.MustCompile(`^(.*?)\s*:?\s*(\d+)\s*/\s*(\d+)$`)
	countBefore = regexp.MustCompile(`^(\d+)\s*/\s*(\d+)\s+(.*)$`)
)

// objectiveLabel is a quest objective as a plain action: "Kill Wolf (0/8)" or "Collect Linen (3/8)".
// text is the game's own objective line ("Wolf slain: 0/8", "Linen: 3/8"); verb is the action the
// quest data implies ("Kill" for a creature, "Collect" for an item, "" when unknown). The quest's
// name is never part of it.
func objectiveLabel(verb, text string) string {
	text = strings.TrimSpace(text)
	count := ""
	if m := countAfter.FindStringSubmatch(text); m != nil {
		text, count = strings.TrimSpace(m[1]), fmt.Sprintf(" (%s/%s)", m[2], m[3])
	} else if m := countBefore.FindStringSubmatch(text); m != nil {
		text, count = strings.TrimSpace(m[3]), fmt.Sprintf(" (%s/%s)", m[1], m[2])
	}
	text = strings.TrimSpace(strings.TrimSuffix(text, " slain"))
	if !wording.StartsWithAction(text) && verb != "" {
		text = verb + " " + text
	}
	return wording.Action(verb, text) + count
}

// tripLabel is a trip's step: where to go, then what to hand in. A dungeon is marked "(dungeon)".
func tripLabel(t questie.Trip, quests string) string {
	if t.Instance == "" {
		return "Turn in " + quests + " to " + t.Finisher + " (" + cmp.Or(t.Sub, t.Zone) + ")"
	}
	return wording.Trip(t.Finisher, t.Place(), t.Instance, t.Zone) + " (dungeon) to turn in " + quests
}
