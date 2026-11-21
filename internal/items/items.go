// Package items holds what the advisor knows about items independent of any table: which ones the
// client data lists but a player can never get.
//
// The items table has no column that marks an internal item (its columns are id, name, levels,
// quality, class, slot, class mask, bonding, stats and prices), and a missing loot source is true of
// many real items (vendor, quest and world items), so the only reliable signal is the name.
package items

import (
	"strings"
	"unicode"
)

// Message is what a tool says when it is asked about an unobtainable item.
const Message = "this item isn't obtainable in game (internal or placeholder)"

// Pattern is one row of the table below.
type Pattern struct {
	Label string
	// Prefix is matched at the start of the trimmed name; Contains anywhere in it. Exactly one is set.
	Prefix   string
	Contains string
	// ExactCase makes the match case-sensitive, for words real items also start with in another
	// case ("Old Blanchy's Blanket", "Testament of Light").
	ExactCase bool
	// WholeWord requires the match not to be followed by a letter or digit, so "DNT" does not
	// take "Dntfoo". Set for prefixes that end in a letter.
	WholeWord bool
	// Except lists lower-case name prefixes that stay obtainable although the pattern matches.
	Except []string
}

// Patterns is the one table of unobtainable-name rules. Order only decides which label a name
// is counted under.
var Patterns = []Pattern{
	{Label: "DNT", Prefix: "DNT", WholeWord: true},
	{Label: "Deprecated", Prefix: "Deprecated", WholeWord: true},
	{Label: "[PH]", Prefix: "[PH]"},
	{Label: "PH ", Prefix: "PH ", ExactCase: true},
	{Label: "OLD", Prefix: "OLD", ExactCase: true, WholeWord: true},
	{Label: "zz", Prefix: "zz", ExactCase: true},
	{Label: "ZZ", Prefix: "ZZ", ExactCase: true},
	{Label: "Test ", Prefix: "Test ", Except: []string{"test of "}},
	{Label: "TEST", Prefix: "TEST", ExactCase: true, WholeWord: true},
	{Label: "QA ", Prefix: "QA ", ExactCase: true},
	{Label: "Monster -", Prefix: "Monster -"},
	{Label: "Unused", Prefix: "Unused", WholeWord: true},
	{Label: "(DNT)", Contains: "(DNT)"},
	{Label: "[DNT]", Contains: "[DNT]"},
	{Label: "DEPRECATED", Contains: "DEPRECATED"},
	{Label: "PLACEHOLDER", Contains: "PLACEHOLDER"},
}

func (p Pattern) matches(name string) bool {
	if p.Contains != "" {
		if p.ExactCase {
			return strings.Contains(name, p.Contains)
		}
		return strings.Contains(strings.ToUpper(name), strings.ToUpper(p.Contains))
	}
	if len(name) < len(p.Prefix) {
		return false
	}
	head, rest := name[:len(p.Prefix)], name[len(p.Prefix):]
	if p.ExactCase && head != p.Prefix || !p.ExactCase && !strings.EqualFold(head, p.Prefix) {
		return false
	}
	if p.WholeWord && rest != "" {
		if r := []rune(rest)[0]; unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	low := strings.ToLower(name)
	for _, e := range p.Except {
		if strings.HasPrefix(low, e) {
			return false
		}
	}
	return true
}

// Match returns the label of the first pattern the item name matches, or "" for an obtainable one.
func Match(name string) string {
	name = strings.TrimSpace(name)
	for _, p := range Patterns {
		if p.matches(name) {
			return p.Label
		}
	}
	return ""
}

// Unobtainable reports whether an item with this name is an internal, deprecated, placeholder or
// test item that no player can get.
func Unobtainable(name string) bool { return Match(name) != "" }
