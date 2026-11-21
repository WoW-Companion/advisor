package digest

import (
	"regexp"
	"strings"
)

// SlotCheck is the addon repo's tests/slotcheck.lua (WoW-Companion/addon) ported to Go, rule for
// rule: after comments are dropped and every string replaced by 0, the file must be one
// `WoWCompanionDigest = {...}` assignment with no parentheses (calls), no bare identifiers as
// values, no call or index on a name, and no `..`, `:` or `#` operators.
func SlotCheck(src string) error {
	code, err := stripLua(src)
	if err != nil {
		return err
	}
	if strings.ContainsAny(code, "()") {
		return errString("call or parenthesis")
	}
	m := regexp.MustCompile(`^\s*WoWCompanionDigest\s*=\s*(\{[\s\S]*\})\s*;?\s*$`).FindStringSubmatch(code)
	if m == nil || !balanced(m[1]) {
		return errString("not a single WoWCompanionDigest = {...} assignment")
	}
	body := m[1]
	for _, w := range regexp.MustCompile(`([A-Za-z_]\w*)\s*(=?=?)`).FindAllStringSubmatch(body, -1) {
		if w[1] != "true" && w[1] != "false" && w[1] != "nil" && w[2] != "=" {
			return errString("identifier '" + w[1] + "' is not a key")
		}
	}
	if w := regexp.MustCompile(`([A-Za-z_]\w*)\s*[{\[]`).FindStringSubmatch(body); w != nil {
		return errString("call or index on '" + w[1] + "'")
	}
	if strings.Contains(body, "..") || strings.ContainsAny(body, ":#") {
		return errString("operator")
	}
	return nil
}

type errString string

func (e errString) Error() string { return "slot check: " + string(e) }

// balanced reports whether the braces of s close exactly at its end (Lua's %b{}).
func balanced(s string) bool {
	depth := 0
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 && i != len(s)-1 {
				return false
			}
		}
	}
	return depth == 0
}

// stripLua drops comments and replaces every string with 0 (the check's strip()).
func stripLua(src string) (string, error) {
	var out strings.Builder
	for i := 0; i < len(src); {
		c := src[i]
		switch {
		case strings.HasPrefix(src[i:], "--"):
			if eq := longOpen(src, i+2); eq >= 0 {
				i = longEnd(src, i+2, eq)
			} else if nl := strings.IndexByte(src[i:], '\n'); nl >= 0 {
				i += nl
			} else {
				i = len(src)
			}
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(src) && src[j] != c {
				if src[j] == '\\' {
					j++
				}
				if j < len(src) && src[j] == '\n' {
					return "", errString("unterminated string")
				}
				j++
			}
			if j >= len(src) {
				return "", errString("unterminated string")
			}
			out.WriteByte('0')
			i = j + 1
		case c == '[' && longOpen(src, i) >= 0:
			out.WriteByte('0')
			i = longEnd(src, i, longOpen(src, i))
		default:
			out.WriteByte(c)
			i++
		}
	}
	return out.String(), nil
}

// longOpen is the number of = in a long bracket opening at pos, or -1.
func longOpen(s string, pos int) int {
	m := regexp.MustCompile(`^\[(=*)\[`).FindStringSubmatch(s[pos:])
	if m == nil {
		return -1
	}
	return len(m[1])
}

func longEnd(s string, pos, eq int) int {
	if e := strings.Index(s[pos:], "]"+strings.Repeat("=", eq)+"]"); e >= 0 {
		return pos + e + eq + 2
	}
	return len(s)
}
