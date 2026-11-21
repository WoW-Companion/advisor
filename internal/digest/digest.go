// Package digest builds the in-game digest the WoWCompanion_Digest slot addon shows (schema 1, see
// docs/digest.md). It adds no advice logic of its own: quests come from advice.WhatsNext, gear from
// gear.Find, and the rest is read from the capture.
package digest

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/brief"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
)

// Schema is the digest format version this package writes.
const Schema = 1

// Section ids the panel knows.
const (
	NextQuests    = "next_quests"
	Upgrades      = "upgrades"
	KeepMaterials = "keep_materials"
	Notes         = "notes"
)

// Caps: the most lines a section holds, and the longest line, in characters. The panel is narrow.
const (
	MaxQuests    = 8
	MaxUpgrades  = 8
	MaxMaterials = 10
	MaxNotes     = 6
	MaxText      = 60
)

// Line is one digest line; Item and Quest are zero when absent.
type Line struct {
	Text  string
	Item  int
	Quest int
}

// Section is an ordered group of lines.
type Section struct {
	ID    string
	Lines []Line
}

// Digest is the whole file.
type Digest struct {
	Schema    int
	Generated int64
	Build     string
	Character string
	Sections  []Section
}

// Options is what Build needs besides the capture and the dataset.
type Options struct {
	Now          time.Time
	Client       character.Client // the game client that wrote the capture
	DatasetBuild string           // the dataset's game build; empty when none
}

// clip shortens s to MaxText characters.
func clip(s string) string {
	if utf8.RuneCountInString(s) <= MaxText {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:MaxText-3]), " ") + "..."
}

func name(c character.Character) string {
	if c.Name != "" {
		return c.Name
	}
	return c.Key
}

func buildOf(cl character.Client) string {
	if cl.Version != "" && cl.Build != "" {
		return cl.Version + "." + cl.Build
	}
	return cl.Build
}

// Build computes the digest for c. ds may be nil (no dataset); sections with nothing to say are
// left out.
func Build(c character.Character, ds store.DataStore, opt Options) Digest {
	d := Digest{Schema: Schema, Generated: opt.Now.Unix(), Build: buildOf(opt.Client), Character: name(c)}
	steps := advice.WhatsNext(c, ds)
	notes := stepNotes(c, ds, steps)

	var quests []Line
	for _, s := range steps {
		if s.Kind != advice.KindTurnIn && s.Kind != advice.KindQuest || strings.HasPrefix(s.Title, advice.AbandonPrefix) {
			continue
		}
		text := s.Title
		if s.Zone != "" {
			text += " (" + s.Zone + ")"
		}
		quests = append(quests, Line{Text: clip(text), Quest: s.QuestID})
	}
	ups, note := upgradeLines(c, ds)
	notes = append(notes, note...)
	notes = append(notes, captureNotes(c, ds, opt)...)

	for _, s := range []Section{
		{NextQuests, capLines(quests, MaxQuests)},
		{Upgrades, capLines(ups, MaxUpgrades)},
		{KeepMaterials, capLines(materials(c, ds), MaxMaterials)},
		{Notes, capLines(notes, MaxNotes)},
	} {
		if len(s.Lines) > 0 {
			d.Sections = append(d.Sections, s)
		}
	}
	return d
}

func capLines(l []Line, n int) []Line {
	if len(l) > n {
		return l[:n]
	}
	return l
}

// stepNotes turns the steps that are not next_quests lines into notes: unspent talents, failed quests.
func stepNotes(c character.Character, ds store.DataStore, steps []advice.Step) []Line {
	var out []Line
	failed := 0
	for _, s := range steps {
		switch {
		case s.Kind == advice.KindTalent:
			out = append(out, Line{Text: clip(talentNote(c, ds, s.Title))})
		case s.Kind == advice.KindQuest && strings.HasPrefix(s.Title, advice.AbandonPrefix):
			failed++
		}
	}
	if failed > 0 {
		out = append(out, Line{Text: fmt.Sprintf("%d failed %s: abandon", failed, map[bool]string{true: "quest", false: "quests"}[failed == 1])})
	}
	return out
}

// talentNote is the unspent-points title, naming the build's next node when the dataset has one.
func talentNote(c character.Character, ds store.DataStore, title string) string {
	if ds == nil {
		return title
	}
	if p := talents.Plan(c, ds, talents.Options{Limit: 1}); len(p.Next) > 0 {
		return title + ": " + p.Next[0].Name
	}
	return title
}

// captureNotes are the gaps in the inputs the player can fix.
func captureNotes(c character.Character, ds store.DataStore, opt Options) []Line {
	var out []Line
	if !c.BankCaptured {
		out = append(out, Line{Text: "Bank not captured: open your bank once"})
	}
	switch {
	case ds == nil:
		out = append(out, Line{Text: "No dataset: zones and gear are limited"})
	case opt.DatasetBuild != "" && opt.Client.Build != "" && !dataset.Matches(opt.DatasetBuild, opt.Client.Version, opt.Client.Build):
		out = append(out, Line{Text: clip(fmt.Sprintf("Dataset is build %s, game is %s", opt.DatasetBuild, opt.Client.Build))})
	}
	return out
}

// upgradeLines is one line per slot, the top candidate and its first source; the second result is
// a note when the search could not run.
func upgradeLines(c character.Character, ds store.DataStore) (lines, notes []Line) {
	if ds == nil {
		return nil, nil // captureNotes says so
	}
	group := ""
	if names, _ := brief.TopGroups(c.Talents); len(names) == 1 {
		group = names[0]
	}
	res, err := gear.Find(c, ds, group, gear.Options{Limit: 1})
	if err != nil {
		return nil, []Line{{Text: clip("No gear upgrades: " + err.Error())}}
	}
	slots := make([]string, 0, len(res.Slots))
	for s := range res.Slots {
		slots = append(slots, s)
	}
	rank := map[string]int{}
	for i, s := range brief.Slots {
		rank[s] = i + 1
	}
	sort.Slice(slots, func(i, j int) bool {
		a, b := rank[slots[i]], rank[slots[j]]
		if a == 0 || b == 0 {
			if a != b {
				return a != 0
			}
			return slots[i] < slots[j]
		}
		return a < b
	})
	for _, slot := range slots {
		cd := res.Slots[slot][0]
		item := cd.Item.Name
		if item == "" {
			item = fmt.Sprintf("item %d", cd.Item.ID)
		}
		text := fmt.Sprintf("%s: %s (ilvl %d)", slot, item, cd.Item.ItemLevel)
		if len(cd.Sources) > 0 {
			s := cd.Sources[0]
			src := s.Name
			if src == "" {
				src = fmt.Sprintf("%s %d", s.Kind, s.ID)
			}
			text += " from " + src
		}
		lines = append(lines, Line{Text: clip(text), Item: int(cd.Item.ID)})
	}
	return lines, nil
}
