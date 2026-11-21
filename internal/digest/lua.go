package digest

import (
	"fmt"
	"strings"
)

// quote renders s as a Lua string literal: backslash, quote and control bytes are escaped, so the
// result holds no raw newline and nothing that could end the literal early.
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\\' || c == '"':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\%03d`, c)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// Lua renders the digest as the one data-only `WoWCompanionDigest = {...}` assignment.
func (d Digest) Lua() string {
	var b strings.Builder
	b.WriteString("-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.\n")
	b.WriteString("WoWCompanionDigest = {\n")
	fmt.Fprintf(&b, "  schema = %d,\n  generated = %d,\n  build = %s,\n  character = %s,\n  sections = {\n", d.Schema, d.Generated, quote(d.Build), quote(d.Character))
	for _, s := range d.Sections {
		fmt.Fprintf(&b, "    { id = %s, lines = {\n", quote(s.ID))
		for _, l := range s.Lines {
			fmt.Fprintf(&b, "      { text = %s", quote(l.Text))
			if l.Item != 0 {
				fmt.Fprintf(&b, ", item = %d", l.Item)
			}
			if l.Quest != 0 {
				fmt.Fprintf(&b, ", quest = %d", l.Quest)
			}
			b.WriteString(" },\n")
		}
		b.WriteString("    } },\n")
	}
	b.WriteString("  },\n}\n")
	return b.String()
}
