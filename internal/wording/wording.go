// Package wording is the one place that phrases counts, level fit and item sources for players, so
// digest lines, card details and prompts cannot drift apart.
package wording

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// Plural is one when n is 1, else many.
func Plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Count is "1 talent point" or "2 talent points".
func Count(n int, one, many string) string {
	return fmt.Sprintf("%d %s", n, Plural(n, one, many))
}

// Levels is "a level", "2 levels".
func Levels(n int) string {
	if n == 1 {
		return "a level"
	}
	return fmt.Sprintf("%d levels", n)
}

// Fit says how a level compares to the player's in words; "" when either is unknown (<= 0).
func Fit(level, mine int) string {
	if level <= 0 || mine <= 0 {
		return ""
	}
	switch d := level - mine; {
	case d == 0:
		return "Right for your level"
	case d > 0:
		return Upper(Levels(d) + " above you")
	default:
		return Upper(Levels(-d) + " below you")
	}
}

// Source phrases where an item comes from, by the kind the dataset records and the source's name.
// Without a name it is "": a bare kind or an id is never shown.
func Source(kind, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	k := strings.ToLower(kind)
	switch {
	case strings.Contains(k, "quest"):
		return "a reward from " + name
	case strings.Contains(k, "craft") || strings.Contains(k, "recipe") || strings.Contains(k, "spell"):
		return "crafted by " + name
	case strings.Contains(k, "vendor"):
		return "sold by " + name
	case k == "object":
		return "found in " + name
	}
	return "a drop from " + name
}

// CutWords shortens s to at most n characters at a word boundary, with no ellipsis.
func CutWords(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	head := string(r[:n])
	if r[n] != ' ' {
		if i := strings.LastIndex(head, " "); i > 0 {
			head = head[:i]
		}
	}
	return strings.TrimRight(head, " ,;:-")
}

// Slot is an equipment slot in words: "finger1" is "ring", "mainhand" is "main hand".
func Slot(slot string) string {
	s := strings.ToLower(strings.TrimRight(slot, "0123456789"))
	switch s {
	case "finger":
		return "ring"
	case "mainhand":
		return "main hand"
	case "offhand":
		return "off hand"
	case "wrist":
		return "wrist"
	}
	return s
}

// Upper capitalizes the first letter of s.
func Upper(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// SlotName is a capture slot as a player says it, capitalized: "back" is "Cloak", "wrist" is
// "Wrists". invType is the item's inventory type when known (13 one-hand, 17 two-hand, 14 shield), else 0.
func SlotName(slot string, invType int) string {
	s := strings.ToLower(strings.TrimRight(slot, "0123456789"))
	switch s {
	case "back":
		return "Cloak"
	case "wrist":
		return "Wrists"
	case "shoulder":
		return "Shoulders"
	case "finger":
		return "Ring"
	case "mainhand", "offhand":
		// The label is the item's inventory type, so it never disagrees with the item.
		switch invType {
		case 13:
			return "One-hand"
		case 17:
			return "Two-hand"
		case 21:
			return "Main hand"
		case 14:
			return "Shield"
		case 22:
			return "Off hand"
		case 23:
			return "Held in off hand"
		}
		if s == "mainhand" {
			return "Main hand"
		}
		return "Off hand"
	case "ranged":
		return "Ranged weapon"
	}
	return Upper(s)
}

var actionVerbs = map[string]bool{}

func init() {
	for _, w := range strings.Fields("kill slay collect gather speak talk use explore escort defeat reach visit discover find deliver protect clear loot obtain listen investigate search return bring recover retrieve acquire free rescue destroy burn place plant capture tame interrogate inspect examine survey light activate open ride fly walk zeppelin boat travel go meet accompany follow help assist heal cleanse purify summon defend guard mine fish cook craft learn train equip wear complete finish pick turn continue") {
		actionVerbs[w] = true
	}
}

// StartsWithAction reports whether s opens with an action verb ("Kill Wolf", "Turn in Quest").
func StartsWithAction(s string) bool {
	f := strings.Fields(strings.ToLower(s))
	return len(f) > 0 && actionVerbs[strings.Trim(f[0], ".,:;!")]
}

// pastToAction maps the past-tense ending the game puts on an objective to the verb it means.
var pastToAction = []struct{ suffix, verb string }{
	{" placed", "Place"}, {" slain", "Kill"}, {" killed", "Kill"}, {" collected", "Collect"},
	{" used", "Use"}, {" destroyed", "Destroy"}, {" burned", "Burn"}, {" planted", "Plant"},
	{" rescued", "Rescue"}, {" freed", "Free"}, {" delivered", "Deliver"}, {" activated", "Activate"},
	{" discovered", "Discover"}, {" investigated", "Investigate"}, {" cleansed", "Cleanse"},
}

// Action makes an objective's text read as an instruction: text that already opens with a verb is
// kept, "Western vial placed" becomes "Place Western vial", and anything else gets verb ("Complete"
// when verb is empty) in front.
func Action(verb, text string) string {
	text = strings.TrimSpace(text)
	if StartsWithAction(text) {
		return text
	}
	l := strings.ToLower(text)
	for _, p := range pastToAction {
		if strings.HasSuffix(l, p.suffix) {
			return p.verb + " " + strings.TrimSpace(text[:len(text)-len(p.suffix)])
		}
	}
	if verb == "" {
		verb = "Complete"
	}
	return verb + " " + text
}

// Trip says where a quest with nothing to do is handed in, as a trip: "Find Vincent inside Shadowfang
// Keep (Silverpine Forest)" for a finisher inside a dungeon, "Deliver to Sputtervalve in Ratchet (The
// Barrens)" otherwise. place is the finisher's place in words; zone is the dungeon's zone.
func Trip(finisher, place, instance, zone string) string {
	if instance == "" {
		return "Deliver to " + finisher + " in " + place
	}
	s := "Find " + finisher + " inside " + instance
	if zone != "" && zone != instance {
		s += " (" + zone + ")"
	}
	return s
}

// A puts the right indefinite article before a name: "an Enchanting trainer", "a Tailoring trainer". It goes
// by the first letter (a vowel takes "an"), which is right for every profession and crafter name.
func A(word string) string {
	if word != "" && strings.ContainsRune("AEIOUaeiou", rune(word[0])) {
		return "an " + word
	}
	return "a " + word
}
