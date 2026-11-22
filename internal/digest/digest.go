// Package digest builds the in-game digest the WoWCompanion_Digest slot addon shows (schema 2.0, one 1.2 entry per character, see
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
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
)

// Schema is the version of one character's entry; EnvelopeSchema is the file's.
const (
	Schema         = "1.2"
	EnvelopeSchema = "2.0"
)

// MaxCharacters is the most characters one file holds.
const MaxCharacters = 30

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
	MaxDungeons  = 3
	MaxTalents   = 3
	MaxText      = 60
)

// Line is one digest line; Item and Quest are zero when absent.
type Line struct {
	Text  string
	Item  int
	Quest int
}

// DungeonLine is one dungeons entry: the dungeon, its level fit and top upgrade; Item is zero when
// absent.
type DungeonLine struct {
	Name string
	Line string
	Item int
}

// TalentLine is one talents entry: the next point to spend; Spell is zero when the dataset has none.
type TalentLine struct {
	Line  string
	Spell int
}

// Section is an ordered group of lines.
type Section struct {
	ID    string
	Lines []Line
}

// Digest is the whole file.
type Digest struct {
	Schema    string
	Generated int64
	Build     string
	Character string
	Sections  []Section
	Dungeons  []DungeonLine // optional, at most MaxDungeons
	Talents   []TalentLine  // optional, at most MaxTalents
}

// Options is what Build needs besides the capture and the dataset.
type Options struct {
	Now          time.Time
	Client       character.Client // the game client that wrote the capture
	DatasetBuild string           // the dataset's game build; empty when none
	Questie      *questie.Index   // the player's own QuestieDB; nil when they have none
}

// NoDataText is what NEXT says when no game dataset has downloaded.
const NoDataText = "Game data not downloaded yet; see Setup"

// clip shortens s to MaxText characters.
func clip(s string) string {
	if utf8.RuneCountInString(s) <= MaxText {
		return s
	}
	r := []rune(s)
	return strings.TrimRight(string(r[:MaxText-3]), " ") + "..."
}

// clipWords is clip that never cuts a word: it ends at the last space that leaves room for "...".
func clipWords(s string) string {
	if utf8.RuneCountInString(s) <= MaxText {
		return s
	}
	head := string([]rune(s)[:MaxText-3])
	if i := strings.LastIndex(head, " "); i > 0 {
		// A word ends where the next rune is a space.
		if r := []rune(s); r[MaxText-3] == ' ' {
			return strings.TrimRight(head, " ") + "..."
		}
		return strings.TrimRight(head[:i], " ") + "..."
	}
	return clip(s)
}

// oneObjective keeps a quest's progress readable on one Next line: the first objective plus "+N more".
func oneObjective(p string) string {
	head, rest, found := strings.Cut(p, "; ")
	if !found {
		return p
	}
	return fmt.Sprintf("%s; +%d more", head, strings.Count(rest, "; ")+1)
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
	steps := advice.WhatsNextWith(c, ds, opt.Questie)
	notes := stepNotes(c, ds, steps)

	var quests, milestones []Line
	for _, s := range steps {
		if s.Kind == advice.KindTalent { // milestones lead the NEXT section
			l := Line{Text: clip(talentNote(c, ds, s.Title))}
			quests, milestones = append(quests, l), append(milestones, l)
			continue
		}
		if advice.IsClassQuest(s) {
			l := Line{Text: clip(s.Title), Quest: s.QuestID}
			quests, milestones = append(quests, l), append(milestones, l)
			continue
		}
		if s.Kind != advice.KindTurnIn && s.Kind != advice.KindQuest || strings.HasPrefix(s.Title, advice.AbandonPrefix) {
			continue
		}
		text := s.Title
		if s.Progress != "" {
			text = oneObjective(s.Progress)
		}
		if s.Zone != "" {
			text += " (" + s.Zone + ")"
		}
		quests = append(quests, Line{Text: clip(text), Quest: s.QuestID})
	}
	if ds == nil {
		quests = append(milestones, Line{Text: NoDataText})
	}
	ups, note := upgradeLines(c, ds)
	notes = append(notes, note...)
	if g := c.GiverLines(opt.Now, 1); len(g) > 0 {
		notes = append(notes, Line{Text: clip(strings.Replace(g[0], "Quest giver you've met nearby: ", "Giver nearby: ", 1))})
	}
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
	d.Dungeons = dungeonLines(c, ds)
	d.Talents = talentLines(c, ds)
	return d
}

// Select picks the characters a file holds: the most recently captured first, at most MaxCharacters,
// one per name (the first wins). The rest come back as skipped.
func Select(chars []character.Character) (kept, skipped []character.Character) {
	sorted := append([]character.Character(nil), chars...)
	sort.SliceStable(sorted, func(i, j int) bool {
		a, b := sorted[i].CapturedAt, sorted[j].CapturedAt
		if a == nil || b == nil {
			return a != nil && b == nil
		}
		return *a > *b
	})
	seen := map[string]bool{}
	for _, c := range sorted {
		if n := name(c); len(kept) < MaxCharacters && !seen[n] {
			seen[n] = true
			kept = append(kept, c)
		} else {
			skipped = append(skipped, c)
		}
	}
	return kept, skipped
}

// dungeonLines is the top MaxDungeons dungeons of dungeons.Plan that have an upgrade to show.
func dungeonLines(c character.Character, ds store.DataStore) []DungeonLine {
	res, err := dungeons.Plan(c, ds)
	if err != nil {
		return nil
	}
	var out []DungeonLine
	for _, p := range res.Picks {
		if len(out) == MaxDungeons {
			break
		}
		if len(p.Drops) == 0 {
			continue
		}
		d := p.Drops[0]
		item := d.Item
		if item == "" {
			item = fmt.Sprintf("item %d", d.ItemID)
		}
		out = append(out, DungeonLine{Name: clip(p.Name), Line: clip(fitText(p) + ": " + item), Item: int(d.ItemID)})
	}
	return out
}

// fitText is the dungeon's level range and how the character sits against it.
func fitText(p dungeons.DungeonPick) string {
	s := fmt.Sprintf("lvl %d-%d", *p.MinLevel, *p.MaxLevel)
	if p.Fit > 0 {
		s += fmt.Sprintf(" (%d off)", p.Fit)
	}
	return s
}

// talentLines is the next MaxTalents points of talents.Plan, with the node's spell id when the
// dataset has one.
func talentLines(c character.Character, ds store.DataStore) []TalentLine {
	if ds == nil {
		return nil
	}
	var out []TalentLine
	for _, p := range talents.Plan(c, ds, talents.Options{Limit: MaxTalents}).Next {
		out = append(out, TalentLine{Line: clip(fmt.Sprintf("%s rank %d", p.Name, p.Rank)), Spell: spellID(ds, p.NodeID)})
	}
	return out
}

// spellID is the spell of a talent node, 0 when the dataset has none.
func spellID(ds store.DataStore, node int64) int {
	rows, err := ds.Filter("talent_nodes", store.Range{Column: "node_id", Min: node, Max: node})
	if err != nil || len(rows) == 0 {
		return 0
	}
	n, _ := rows[0]["spell_id"].(int64)
	return int(n)
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
		// The NEXT section says so (NoDataText); no second line here.
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
		text := fmt.Sprintf("%s: %s (ilvl %d", slot, item, cd.Item.ItemLevel)
		if cd.Soon {
			text += fmt.Sprintf(", at level %d", cd.Item.ReqLevel)
		}
		text += ")"
		if len(cd.Sources) > 0 {
			s := cd.Sources[0]
			src := s.Name
			if src == "" {
				src = fmt.Sprintf("%s %d", s.Kind, s.ID)
			}
			// The source goes on only whole: when it would push the line past the cap it is dropped.
			if with := text + " from " + src; utf8.RuneCountInString(with) <= MaxText {
				text = with
			}
		}
		lines = append(lines, Line{Text: clipWords(text), Item: int(cd.Item.ID)})
	}
	return lines, nil
}
