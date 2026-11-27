package ask

import (
	"log"
	"regexp"
	"strings"

	"github.com/WoW-Companion/advisor/internal/inventory"
)

// bagQuestion reports whether the question is about what to do with items in the bags or bank.
var bagQuestion = regexp.MustCompile(`(?i)\b(bags?|bank|inventory|sell|toss|vendor|destroy|junk|trash|keep|mail|clutter|slots?)\b`)

const maxProtectedFacts = 25

// protectedFacts lists the items in the bags and bank that a quest in the log needs, for bag and
// sell questions; "" when the question is no bag question or nothing is needed.
func protectedFacts(cc loaded, question string) string {
	if !bagQuestion.MatchString(question) {
		return ""
	}
	ps := inventory.ProtectedItems(cc.c, cc.ds, cc.qx)
	if len(ps) == 0 {
		return ""
	}
	var lines []string
	for i, p := range ps {
		if i == maxProtectedFacts {
			break
		}
		lines = append(lines, "- "+p.Item+": "+p.Reason())
	}
	return "Quest items the character must KEEP (a quest in the log needs them, even a ready turn-in). Never put these under sell, toss, destroy or send to an alt, and say why they stay:\n" + strings.Join(lines, "\n")
}

var (
	sectionWords = `(?:sell|toss|vendor|destroy|delete|trash|junk|get rid|throw|drop|mail|send|alts?|dump)`
	sectionHead  = regexp.MustCompile(`(?i)^\W*` + sectionWords + `\b`)
	// bulletRe is a list line; anything else that is short and bold, headed or ends in a colon starts a section.
	bulletRe  = regexp.MustCompile(`^\s*(?:[-*•]|\d+[.)])\s+`)
	headingRe = regexp.MustCompile(`^\s*(?:#{1,6}\s|\*\*|__)`)
)

func isHeading(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" || bulletRe.MatchString(line) {
		return false
	}
	return headingRe.MatchString(line) || strings.HasSuffix(t, ":") || len([]rune(t)) <= 40 && !strings.ContainsAny(t, ".!?")
}

// GuardBagAnswer checks a bag answer against the items the quest log needs. A line under a sell, toss,
// destroy or mail heading that names such an item is dropped, as is a "Toss: a, b, c" line's entry for
// it, and "Kept: <item> (needed for <quest>)" is added once per item. It returns the text and the
// corrections made; the text is unchanged when there are none.
func GuardBagAnswer(text string, ps []inventory.Protected) (string, []string) {
	if len(ps) == 0 || strings.TrimSpace(text) == "" {
		return text, nil
	}
	lower := strings.ToLower(text)
	var hit []inventory.Protected
	for _, p := range ps {
		if p.Item != "" && strings.Contains(lower, strings.ToLower(p.Item)) {
			hit = append(hit, p)
		}
	}
	if len(hit) == 0 {
		return text, nil
	}
	mentions := func(s string) []inventory.Protected {
		var out []inventory.Protected
		ls := strings.ToLower(s)
		for _, p := range hit {
			if strings.Contains(ls, strings.ToLower(p.Item)) {
				out = append(out, p)
			}
		}
		return out
	}
	var out []string
	var fixes []string
	kept := map[string]bool{}
	var keptOrder []inventory.Protected
	note := func(p inventory.Protected) {
		if !kept[p.Item] {
			kept[p.Item] = true
			keptOrder = append(keptOrder, p)
		}
	}
	inSection := false
	for _, line := range strings.Split(text, "\n") {
		if isHeading(line) {
			inSection = sectionHead.MatchString(strings.TrimLeft(strings.TrimSpace(line), "#*_ "))
			// "Toss: a, b, c" on one line: take the protected entries out of the list
			if head, rest, ok := strings.Cut(line, ":"); ok && inSection && strings.TrimSpace(rest) != "" {
				if ms := mentions(rest); len(ms) > 0 {
					parts := regexp.MustCompile(`\s*[,;]\s*`).Split(strings.TrimSpace(rest), -1)
					var keep []string
					for _, part := range parts {
						if len(mentions(part)) == 0 {
							keep = append(keep, part)
						}
					}
					for _, p := range ms {
						note(p)
						fixes = append(fixes, p.Item+" removed from: "+strings.TrimSpace(head))
					}
					if len(keep) > 0 {
						line = head + ": " + strings.Join(keep, ", ")
					} else {
						continue
					}
				}
			}
			out = append(out, line)
			continue
		}
		if inSection && strings.TrimSpace(line) != "" {
			if ms := mentions(line); len(ms) > 0 {
				for _, p := range ms {
					note(p)
					fixes = append(fixes, p.Item+" removed from a sell or toss list")
				}
				continue
			}
		}
		out = append(out, line)
	}
	if len(keptOrder) == 0 {
		return text, nil
	}
	res := strings.TrimRight(strings.Join(out, "\n"), "\n")
	for _, p := range keptOrder {
		res += "\n\nKept: " + p.Item + " (" + p.Reason() + ")"
	}
	return res, fixes
}

// GuardAnswer applies GuardBagAnswer for the question's character and logs each correction. It is a
// no-op when the capture cannot be read or the answer names no item the log needs.
func GuardAnswer(cfg Config, character, text string) string {
	if strings.TrimSpace(text) == "" {
		return text
	}
	cc, ok := load(cfg, character)
	if !ok {
		return text
	}
	out, fixes := GuardBagAnswer(text, inventory.ProtectedItems(cc.c, cc.ds, cc.qx))
	for _, f := range fixes {
		log.Printf("bag answer corrected: %s", f)
	}
	return out
}
