package ask

import (
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Subjects are the known entities whose names appear in the answer text, in the order they first
// appear, so the answer's lead subject comes first. A name inside a longer matched name is not a
// separate subject. Each entity counts once.
func Subjects(answer string, known []Entity) []Entity {
	hay := norm(answer)
	if hay == "" {
		return nil
	}
	hay = " " + hay + " "
	type hit struct {
		at, end int
		e       Entity
	}
	var hits []hit
	seen := map[string]bool{}
	for _, e := range known {
		n := norm(e.Name)
		if len(n) < 4 || seen[n] {
			continue
		}
		seen[n] = true
		if i := strings.Index(hay, " "+n+" "); i >= 0 {
			hits = append(hits, hit{i, i + len(n) + 1, e})
		}
	}
	sort.SliceStable(hits, func(a, b int) bool {
		if hits[a].at != hits[b].at {
			return hits[a].at < hits[b].at
		}
		return hits[a].end > hits[b].end // the longer name first
	})
	var out []Entity
	last := -1
	for _, h := range hits {
		if h.end <= last { // inside a longer name already taken
			continue
		}
		last = max(last, h.end)
		out = append(out, h.e)
	}
	return out
}

var possessive = regexp.MustCompile(`['’]s\b`)

// norm is a name or text as Subjects compares it: lower case, no accents or punctuation, no possessive
// ("Seena Skybreaker's hut" names Seena Skybreaker).
func norm(s string) string { return names.Normalize(possessive.ReplaceAllString(s, "")) }

// KnownEntities are the names the character's own data can put on a card: its quest log, gear, bags,
// NPCs, zone and the entries of its digest.
func KnownEntities(cfg Config, char string) []Entity {
	cc, ok := load(cfg, char)
	if !ok {
		return nil
	}
	c := cc.c
	var out []Entity
	for _, q := range c.Quests {
		out = append(out, Entity{Kind: "quest", ID: int64(q.QuestID), Name: q.Title})
		if q.Zone != "" {
			out = append(out, Entity{Kind: "zone", Name: q.Zone})
		}
	}
	for _, it := range c.Equipment {
		id := int64(0)
		if it.ItemID != nil {
			id = int64(*it.ItemID)
		}
		out = append(out, Entity{Kind: "item", ID: id, Name: it.Name})
	}
	for _, it := range c.Bags {
		out = append(out, Entity{Kind: "item", ID: int64(it.ItemID), Name: it.Name})
	}
	for _, n := range c.NPCs {
		out = append(out, Entity{Kind: "npc", ID: int64(n.ID), Name: n.Name})
	}
	if c.Location != nil && c.Location.Zone != "" {
		out = append(out, Entity{Kind: "zone", Name: c.Location.Zone})
	}
	if cc.ds != nil {
		for _, sec := range cc.digest().Sections {
			for _, l := range sec.Lines {
				if l.Title != "" {
					out = append(out, Entity{Kind: kindOfLine(l.Quest, l.Item), ID: int64(max(l.Quest, l.Item)), Name: l.Title})
				}
				if l.NPC != "" {
					out = append(out, Entity{Kind: "npc", Name: l.NPC})
				}
				if l.Zone != "" {
					out = append(out, Entity{Kind: "zone", Name: l.Zone})
				}
			}
		}
	}
	return out
}

func kindOfLine(quest, item int) string {
	switch {
	case quest > 0:
		return "quest"
	case item > 0:
		return "item"
	}
	return "quest"
}

var phraseWord = regexp.MustCompile(`[\p{L}'’-]+`)

// DatasetEntities are the dataset's items, recipes, NPCs and instances named in the answer: every
// run of up to six words that starts with a capital is looked up by exact name.
func DatasetEntities(ds store.DataStore, answer string) []Entity {
	if ds == nil {
		return nil
	}
	words := phraseWord.FindAllString(answer, -1)
	var out []Entity
	tried := map[string]bool{}
	const maxLookups = 80
	for i := 0; i < len(words) && len(tried) < maxLookups; i++ {
		if !capital(words[i]) {
			continue
		}
		for n := min(6, len(words)-i); n >= 1; n-- {
			if !capital(words[i+n-1]) || !nameShaped(words[i:i+n]) {
				continue
			}
			phrase := strings.Join(words[i:i+n], " ")
			key := names.Normalize(phrase)
			if len(key) < 5 || tried[key] || len(tried) >= maxLookups {
				continue
			}
			tried[key] = true
			for _, t := range []struct{ table, kind string }{{names.Items, "item"}, {names.Recipes, "recipe"}, {names.Npcs, "npc"}, {names.Instances, "zone"}} {
				hits, _, err := names.Find(ds, t.table, phrase, 3)
				if err != nil {
					continue
				}
				for _, h := range hits {
					if names.Normalize(h.Name) == key {
						out = append(out, Entity{Kind: t.kind, ID: h.ID, Name: h.Name})
					}
				}
			}
		}
	}
	return out
}

// AnswerEntities is what the answer is about: the touched, character and dataset entities named in
// the answer text, first mention first.
func AnswerEntities(cfg Config, char, answer string, touched []Entity) []Entity {
	known := append([]Entity(nil), touched...)
	known = append(known, KnownEntities(cfg, char)...)
	if cfg.Dataset != "" {
		known = append(known, DatasetEntities(store.NewDir(cfg.Dataset), answer)...)
	}
	return Subjects(answer, known)
}

func capital(w string) bool { return unicode.IsUpper([]rune(w)[0]) }

var connectors = map[string]bool{"of": true, "the": true, "and": true, "in": true, "for": true, "to": true, "a": true, "on": true}

// nameShaped reports whether the words could be one proper name: capitalised, with small connectors inside.
func nameShaped(ws []string) bool {
	for _, w := range ws {
		if !capital(w) && !connectors[w] {
			return false
		}
	}
	return true
}
