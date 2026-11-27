package quests

import (
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// SourceQuestie marks a chain or fact read from the player's own QuestieDB.
const SourceQuestie = "questie"

// SourceEverloot marks a fact read from the dataset's quests_observed table: what players saw in
// game and reported to Everloot (CC BY-SA 4.0).
const SourceEverloot = "everloot"

// Credit is the attribution line shown wherever Everloot facts appear.
const Credit = "Everloot (CC BY-SA)"

// SourceDataset marks a chain read from the dataset's quest lines.
const SourceDataset = "dataset"

// Facts is what the player's QuestieDB says about one quest.
type Facts struct {
	QuestID    int64  `json:"quest_id"`
	Name       string `json:"name,omitempty"`
	Level      int    `json:"level,omitempty"`
	MinLevel   int    `json:"min_level,omitempty"`
	Giver      string `json:"giver,omitempty" jsonschema:"who or what starts it, with the zone"`
	TurnIn     string `json:"turn_in,omitempty" jsonschema:"who or what takes it in, with the zone"`
	Objective  string `json:"objective,omitempty"`
	Classes    string `json:"classes,omitempty" jsonschema:"classes it is limited to; empty when open to all"`
	Races      string `json:"races,omitempty" jsonschema:"races it is limited to; empty when open to all"`
	Category   string `json:"category,omitempty"`
	Faction    string `json:"faction,omitempty"`
	Tag        string `json:"tag,omitempty" jsonschema:"group or dungeon when players tagged the quest as one that needs a group; empty otherwise"`
	Source     string `json:"source" jsonschema:"questie or everloot (seen by players on Everloot)"`
	Credit     string `json:"credit,omitempty" jsonschema:"set to the Everloot credit when any fact comes from Everloot"`
	Followers  []int  `json:"-"`
	Precursors []int  `json:"-"`
}

var classNames = []string{"Warrior", "Paladin", "Hunter", "Rogue", "Priest", "Death Knight", "Shaman", "Mage", "Warlock", "Monk", "Druid", "Demon Hunter", "Evoker"}
var raceNames = []string{"Human", "Orc", "Dwarf", "Night Elf", "Undead", "Tauren", "Gnome", "Troll", "Goblin", "Blood Elf", "Draenei"}

func maskNames(m uint64, names []string) string {
	var out []string
	for i, n := range names {
		if m&(1<<uint(i)) != 0 {
			out = append(out, n)
		}
	}
	return strings.Join(out, ", ")
}

// QuestieFacts describes quest id from the index, or nil when Questie does not know it.
func QuestieFacts(ix *questie.Index, id int64) *Facts {
	if ix == nil {
		return nil
	}
	q := ix.Quests[int(id)]
	if q == nil {
		return nil
	}
	f := &Facts{QuestID: id, Name: q.Name, Level: q.Level, MinLevel: q.MinLevel, Giver: ix.Describe(q.Starters),
		TurnIn: ix.Describe(q.Finishers), Objective: q.Objective, Source: SourceQuestie}
	f.Classes, f.Races = maskNames(q.Classes, classNames), maskNames(q.Races, raceNames)
	return f
}

func link(ix *questie.Index, id int) Facts {
	if f := QuestieFacts(ix, int64(id)); f != nil {
		return *f
	}
	return Facts{QuestID: int64(id), Source: SourceQuestie}
}

// beforeOf is the quests that come before q in the player's QuestieDB, and whether all of them must be finished (a group prerequisite) or any one is enough (single prerequisites, when there are several).
func beforeOf(ix *questie.Index, q *questie.Quest) ([]Facts, bool) {
	var before []Facts
	seen := map[int]bool{}
	for _, id := range append(append([]int{}, q.PreSingle...), q.PreGroup...) {
		if !seen[id] {
			seen[id] = true
			before = append(before, link(ix, id))
		}
	}
	return before, len(q.PreGroup) > 1
}

// OfWith is Of with a fallback: when the dataset has no quest line for the quest, the chain is built
// from the player's QuestieDB (ix may be nil). The dataset wins whenever it has a line.
func OfWith(ds store.DataStore, ix *questie.Index, questID int64) Chain {
	c := Of(ds, questID)
	q := (*questie.Quest)(nil)
	if ix != nil {
		q = ix.Quests[int(questID)]
	}
	if c.Found {
		c.Source = SourceDataset
		if f := observed(ds, questID); f != nil && f.GroupWords() != "" {
			c.Quest = f
		}
		if q != nil {
			// the line gives the order and wins over the player's Questie; the line holds no giver, turn-in or minimum level, which the Questie still gives
			if f := FactsOf(ds, ix, questID); f != nil && (f.Giver != "" || f.TurnIn != "" || f.MinLevel > 0 || f.GroupWords() != "") {
				c.Quest = f
			}
		}
		return c
	}
	if q == nil {
		f := FactsOf(ds, nil, questID)
		if f == nil || f.Source != SourceEverloot {
			return c
		}
		c.Found, c.Source, c.Quest = true, SourceEverloot, f
		return c
	}
	c.Found, c.Source = true, SourceQuestie
	c.Quest = FactsOf(ds, ix, questID)
	if c.Quest != nil && c.Quest.Source == SourceEverloot {
		c.Source = SourceEverloot
	}
	c.Before, c.BeforeAll = beforeOf(ix, q)
	seen := map[int]bool{}
	var after []int
	add := func(id int) {
		if id != 0 && id != int(questID) && !seen[-id] {
			seen[-id] = true
			after = append(after, id)
		}
	}
	add(q.Next)
	var leads []int
	for id, o := range ix.Quests {
		for _, p := range append(append([]int{}, o.PreSingle...), o.PreGroup...) {
			if p == int(questID) {
				leads = append(leads, id)
				break
			}
		}
	}
	sort.Ints(leads)
	for _, id := range leads {
		add(id)
	}
	for _, id := range after {
		c.After = append(c.After, link(ix, id))
	}
	c.NextQuestID = int64(q.Next)
	return c
}

// ResolveWith is Resolve that also matches names the dataset lacks against QuestieDB.
func ResolveWith(ds store.DataStore, ix *questie.Index, text string) (id int64, name string, candidates []string) {
	id, name, candidates = Resolve(ds, text)
	if id != 0 || len(candidates) > 0 || ix == nil {
		if id != 0 && name == "" && ix != nil {
			if q := ix.Quests[int(id)]; q != nil {
				name = q.Name
			}
		}
		return
	}
	ids := ix.FindByName(text)
	switch {
	case len(ids) == 1:
		return int64(ids[0]), ix.Quests[ids[0]].Name, nil
	case len(ids) > 1:
		for _, i := range ids {
			if len(candidates) == 8 {
				break
			}
			candidates = append(candidates, ix.Quests[i].Name+" ("+strconv.Itoa(i)+")")
		}
	}
	return 0, "", candidates
}

func (f Facts) label() string {
	n := "quest " + strconv.FormatInt(f.QuestID, 10)
	if f.Name != "" {
		n = f.Name + " (" + n + ")"
	}
	if f.Level > 0 {
		n += ", level " + strconv.Itoa(f.Level)
	}
	return n
}

// GroupWords says a quest players tagged as one for a group or a dungeon; empty for any other.
func (f Facts) GroupWords() string {
	switch f.Tag {
	case "group":
		return "a group quest"
	case "dungeon":
		return "a dungeon quest"
	}
	return ""
}

// classHeadings are the class names players' reports use as a quest's heading (quests_observed.category): a class quest is filed
// under its class, not under a zone.
var classHeadings = map[string]string{"warrior": "Warrior", "paladin": "Paladin", "hunter": "Hunter", "rogue": "Rogue", "priest": "Priest",
	"shaman": "Shaman", "mage": "Mage", "warlock": "Warlock", "druid": "Druid"}

// ClassHeading is the class a quest's heading names ("Warlock"), "" when the heading is a zone or there is none: such a quest is a class
// quest and its heading is no place.
func (f Facts) ClassHeading() string {
	return classHeadings[strings.ToLower(strings.TrimSpace(f.Category))]
}

func (f Facts) detail() string {
	var b strings.Builder
	if f.Giver != "" {
		b.WriteString("; starts with " + f.Giver)
	}
	if f.TurnIn != "" {
		b.WriteString("; turn in to " + f.TurnIn)
	}
	if f.Objective != "" {
		b.WriteString("; objective: " + f.Objective)
	}
	return b.String()
}

func renderQuestie(c Chain, name string) string {
	var b strings.Builder
	if f := c.Quest; f != nil {
		if f.Source == SourceEverloot {
			b.WriteString("Seen by players on Everloot (CC BY-SA): " + f.label())
			if f.Faction != "" {
				b.WriteString(", " + f.Faction)
			}
			if f.Category != "" {
				b.WriteString(", " + f.Category)
			}
			if w := f.GroupWords(); w != "" {
				b.WriteString(", " + w)
			}
		} else {
			b.WriteString("From your Questie database: " + f.label())
		}
		if f.MinLevel > 0 {
			b.WriteString(", needs level " + strconv.Itoa(f.MinLevel))
		}
		if f.Classes != "" {
			b.WriteString(", " + f.Classes + " only")
		}
		if f.Races != "" {
			b.WriteString(", " + f.Races + " only")
		}
		b.WriteString(f.detail() + ".\n")
		var unnamed []string
		if f.Giver == "" {
			unnamed = append(unnamed, "who gives it")
		}
		if f.TurnIn == "" {
			unnamed = append(unnamed, "who takes it in")
		}
		if len(unnamed) > 0 {
			b.WriteString("The game data you have doesn't name " + strings.Join(unnamed, " or ") + ".\n")
		}
	}
	if c.Source == SourceEverloot && len(c.Before) == 0 && len(c.After) == 0 {
		b.WriteString("No follow-up quest is listed for it in the game data you have.\n")
		return b.String()
	}
	writeBefore(&b, c)
	if len(c.After) == 0 {
		b.WriteString("No follow-up quest is listed for it in the game data you have.\n")
		return b.String()
	}
	b.WriteString("Leads to:\n")
	for _, s := range c.After {
		b.WriteString("  - " + s.label() + s.detail() + "\n")
	}
	return b.String()
}

// writeBefore lists the quests that must be finished first, when the chain holds any.
func writeBefore(b *strings.Builder, c Chain) {
	if len(c.Before) == 0 {
		return
	}
	switch {
	case c.BeforeAll:
		b.WriteString("Comes after (finish all of these first):\n")
	case len(c.Before) > 1:
		b.WriteString("Comes after (finish any one of these first):\n")
	default:
		b.WriteString("Comes after:\n")
	}
	for _, s := range c.Before {
		b.WriteString("  - " + s.label() + s.detail() + "\n")
	}
}
