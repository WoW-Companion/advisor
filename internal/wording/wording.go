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
