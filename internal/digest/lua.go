package digest

import (
	"fmt"
	"strings"
)

// Quote renders s as a Lua string literal: backslash, quote and control bytes are escaped, so the
// result holds no raw newline and nothing that could end the literal early.
func Quote(s string) string {
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

// writeFields renders the optional fields of a line that are set (schema 1.3), after the older ones.
func (l Line) writeFields(b *strings.Builder) {
	str := func(k, v string) {
		if v != "" {
			fmt.Fprintf(b, ", %s = %s", k, Quote(v))
		}
	}
	num := func(k string, v int) {
		if v != 0 {
			fmt.Fprintf(b, ", %s = %d", k, v)
		}
	}
	str("kind", l.Kind)
	str("title", l.Title)
	num("level", l.Level)
	if l.Class {
		b.WriteString(", class = true")
	}
	if l.Elite {
		b.WriteString(", elite = true")
	}
	str("npc", l.NPC)
	str("zone", l.Zone)
	if len(l.Objectives) > 0 {
		b.WriteString(", objectives = { ")
		for i, o := range l.Objectives {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(b, "{ text = %s", Quote(o.Text))
			if o.HasCount {
				fmt.Fprintf(b, ", have = %d, need = %d", o.Have, o.Need)
			}
			b.WriteString(" }")
		}
		b.WriteString(" }")
	}
	num("points", l.Points)
	str("slot", l.Slot)
	num("ilvl", l.ILvl)
	num("atLevel", l.AtLevel)
}

// header is the comment line that opens the file.
const header = "-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.\n"

// Lua renders the digest as a one-character entry body: the `{ ... }` table, without its key.
func (d Digest) Lua() string {
	var b strings.Builder
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  schema = %s,\n  generated = %d,\n  build = %s,\n  character = %s,\n  sections = {\n", Quote(d.Schema), d.Generated, Quote(d.Build), Quote(d.Character))
	for _, s := range d.Sections {
		fmt.Fprintf(&b, "    { id = %s, lines = {\n", Quote(s.ID))
		for _, l := range s.Lines {
			fmt.Fprintf(&b, "      { text = %s", Quote(l.Text))
			if l.Item != 0 {
				fmt.Fprintf(&b, ", item = %d", l.Item)
			}
			if l.Quest != 0 {
				fmt.Fprintf(&b, ", quest = %d", l.Quest)
			}
			l.writeFields(&b)
			b.WriteString(" },\n")
		}
		b.WriteString("    } },\n")
	}
	b.WriteString("  },\n")
	if len(d.Dungeons) > 0 {
		b.WriteString("  dungeons = {\n")
		for _, l := range d.Dungeons {
			fmt.Fprintf(&b, "    { name = %s, line = %s", Quote(l.Name), Quote(l.Line))
			if l.Item != 0 {
				fmt.Fprintf(&b, ", item_id = %d", l.Item)
			}
			b.WriteString(" },\n")
		}
		b.WriteString("  },\n")
	}
	if len(d.Talents) > 0 {
		b.WriteString("  talents = {\n")
		for _, l := range d.Talents {
			fmt.Fprintf(&b, "    { line = %s", Quote(l.Line))
			if l.Spell != 0 {
				fmt.Fprintf(&b, ", spell_id = %d", l.Spell)
			}
			b.WriteString(" },\n")
		}
		b.WriteString("  },\n")
	}
	b.WriteString("}")
	return b.String()
}

// Envelope is the whole file, schema 2.0: one entry per captured character, keyed by the name as
// captured.
type Envelope struct {
	Generated  int64
	Characters []Digest // in file order; each Character is its key
}

// Lua renders the envelope as the one data-only `WoWCompanionDigest = {...}` assignment.
func (e Envelope) Lua() string {
	var b strings.Builder
	b.WriteString(header)
	b.WriteString("WoWCompanionDigest = {\n")
	fmt.Fprintf(&b, "  schema = %s,\n  generated = %d,\n  characters = {\n", Quote(EnvelopeSchema), e.Generated)
	for _, d := range e.Characters {
		entry := strings.ReplaceAll(strings.TrimSuffix(d.Lua(), "\n"), "\n", "\n    ")
		fmt.Fprintf(&b, "    [%s] = %s,\n", Quote(d.Character), entry)
	}
	b.WriteString("  },\n}\n")
	return b.String()
}
