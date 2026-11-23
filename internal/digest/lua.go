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
	str("wear", l.Wear)
	str("gain", l.Gain)
	str("replaces", l.Replaces)
	str("how", l.How)
	str("pathKind", l.PathKind)
	if len(l.Spells) > 0 {
		b.WriteString(", spells = { ")
		for i, sp := range l.Spells {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(b, "{ id = %d, name = %s", sp.ID, Quote(sp.Name))
			if sp.Rank > 0 {
				fmt.Fprintf(b, ", rank = %d", sp.Rank)
			}
			fmt.Fprintf(b, ", new = %t }", sp.New)
		}
		b.WriteString(" }")
	}
	if l.HasXY {
		fmt.Fprintf(b, ", x = %.1f, y = %.1f", l.X, l.Y)
	}
	writeContext(b, l.Details, l.Prompt)
	writeAsks(b, l.Asks, l.Ref)
	writeWhy(b, l.Why)
}

// writeWhy renders the schema 1.6 field of an entry: why, left out when empty.
func writeWhy(b *strings.Builder, why string) {
	if why != "" {
		fmt.Fprintf(b, ", why = %s", Quote(why))
	}
}

// writeContext renders the schema 1.4 fields of an entry: details (a list of strings) and prompt.
func writeContext(b *strings.Builder, details []string, prompt string) {
	if len(details) > 0 {
		b.WriteString(", details = { ")
		for i, d := range details {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(Quote(d))
		}
		b.WriteString(" }")
	}
	if prompt != "" {
		fmt.Fprintf(b, ", prompt = %s", Quote(prompt))
	}
}

// writeAsks renders the schema 1.5 fields of an entry: asks (a list of strings) and ref.
func writeAsks(b *strings.Builder, asks []string, ref string) {
	if len(asks) > 0 {
		b.WriteString(", asks = { ")
		for i, a := range asks {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(Quote(a))
		}
		b.WriteString(" }")
	}
	if ref != "" {
		fmt.Fprintf(b, ", ref = %s", Quote(ref))
	}
}

// header is the comment line that opens the file.
const header = "-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.\n"

// Lua renders the digest as a one-character entry body: the `{ ... }` table, without its key.
func (d Digest) Lua() string {
	var b strings.Builder
	b.WriteString("{\n")
	fmt.Fprintf(&b, "  schema = %s,\n  generated = %d,\n  build = %s,\n  character = %s,\n  sections = {\n", Quote(d.Schema), d.Generated, Quote(d.Build), Quote(d.Character))
	for _, s := range d.Sections {
		fmt.Fprintf(&b, "    { id = %s, ", Quote(s.ID))
		if s.Summary != "" {
			fmt.Fprintf(&b, "summary = %s, ", Quote(s.Summary))
		}
		b.WriteString("lines = {\n")
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
			writeContext(&b, l.Details, l.Prompt)
			writeAsks(&b, l.Asks, l.Ref)
			writeWhy(&b, l.Why)
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
			writeContext(&b, l.Details, l.Prompt)
			writeAsks(&b, l.Asks, l.Ref)
			writeWhy(&b, l.Why)
			b.WriteString(" },\n")
		}
		b.WriteString("  },\n")
	}
	if d.Stats != nil {
		fmt.Fprintf(&b, "  stats = { profile = %s, insights = { ", Quote(d.Stats.Profile))
		for i, in := range d.Stats.Insights {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(Quote(in))
		}
		b.WriteString(" }")
		writeAsks(&b, d.Stats.Asks, "")
		b.WriteString(" },\n")
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
