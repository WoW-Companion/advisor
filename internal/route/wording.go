package route

import (
	"fmt"
	"regexp"
	"strings"
)

func init() {
	for _, w := range strings.Fields("kill slay collect gather speak talk use explore escort defeat reach visit discover find deliver protect clear loot obtain listen investigate search return bring recover retrieve acquire free rescue destroy burn place plant capture tame interrogate inspect examine survey light activate open ride fly travel go meet accompany follow help assist heal cleanse purify summon defend guard mine fish cook craft learn train equip wear complete") {
		actionVerbs[w] = true
	}
}

var (
	countAfter  = regexp.MustCompile(`^(.*?)\s*:?\s*(\d+)\s*/\s*(\d+)$`)
	countBefore = regexp.MustCompile(`^(\d+)\s*/\s*(\d+)\s+(.*)$`)
	actionVerbs = map[string]bool{}
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
	l := strings.ToLower(text)
	if f := strings.Fields(l); len(f) > 0 && actionVerbs[strings.Trim(f[0], ".,:;!")] {
		verb = ""
	}
	if verb != "" {
		text = verb + " " + text
	}
	return text + count
}
