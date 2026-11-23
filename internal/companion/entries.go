package companion

import "github.com/WoW-Companion/advisor/internal/digest"

// MaxEntries is the most plan entries a character carries for the overlay.
const MaxEntries = 14

// PlanEntry is one entry of the in-game panel as the overlay lists it: a one-line title and status in
// the player's words, and the questions to offer. Ref is sent back with a question so the advisor
// knows the subject; the overlay never shows it.
type PlanEntry struct {
	Group  string   `json:"group"` // next, upgrades, notes, dungeons or talents
	Kind   string   `json:"kind,omitempty"`
	Title  string   `json:"title"`
	Status string   `json:"status,omitempty"`
	Why    string   `json:"why,omitempty"` // one short reason, at most 70 characters
	Class  bool     `json:"class,omitempty"`
	Asks   []string `json:"asks,omitempty"`
	Ref    string   `json:"ref,omitempty"`
}

var groupOf = map[string]string{digest.NextQuests: "next", digest.Upgrades: "upgrades", digest.Notes: "notes"}

// entriesOf lists d's entries that have questions, in the panel's order: next quests, upgrades, notes,
// dungeons, talents.
func entriesOf(d digest.Digest) []PlanEntry {
	var out []PlanEntry
	seen := map[string]bool{}
	add := func(e PlanEntry) {
		if len(e.Asks) == 0 || (e.Ref != "" && seen[e.Ref]) {
			return
		}
		seen[e.Ref] = true
		out = append(out, e)
	}
	for _, sec := range d.Sections {
		g, ok := groupOf[sec.ID]
		if !ok {
			continue
		}
		for _, l := range sec.Lines {
			e := PlanEntry{Group: g, Kind: l.Kind, Title: l.Title, Status: statusOf(l), Why: l.Why, Class: l.Class, Asks: l.Asks, Ref: l.Ref}
			if e.Title == "" {
				e.Title, e.Status = l.Text, ""
			}
			add(e)
		}
	}
	for _, l := range d.Dungeons {
		add(PlanEntry{Group: "dungeons", Kind: "dungeon", Title: l.Name, Status: l.Line, Why: l.Why, Asks: l.Asks, Ref: l.Ref})
	}
	for _, l := range d.Talents {
		add(PlanEntry{Group: "talents", Kind: "talent", Title: l.Line, Why: l.Why, Asks: l.Asks, Ref: l.Ref})
	}
	return out[:min(len(out), MaxEntries)]
}

// statusOf is the short status after a quest title: what to do with it.
func statusOf(l digest.Line) string {
	switch l.Kind {
	case digest.KindPickup:
		return "pick up"
	case digest.KindTurnIn:
		return "turn in"
	case digest.KindProgress:
		return "in progress"
	}
	return ""
}
