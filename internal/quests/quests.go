// Package quests places a quest in its quest line, from the dataset's quest_lines table.
// The rules are in docs/chain.md.
package quests

import (
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Step is one quest of a line.
type Step struct {
	Position int    `json:"position" jsonschema:"1-based place in the line"`
	QuestID  int64  `json:"quest_id"`
	Name     string `json:"name,omitempty" jsonschema:"empty when the dataset has no quest name"`
}

// Chain is where a quest sits in its quest line. Found is false when the dataset has no
// quest_lines table or no line holds the quest; the other fields are then empty.
type Chain struct {
	Found    bool   `json:"found"`
	LineID   int64  `json:"line_id,omitempty"`
	Line     string `json:"line,omitempty"`
	QuestID  int64  `json:"quest_id"`
	Position int    `json:"position,omitempty" jsonschema:"1-based place of the quest in the line"`
	Steps    []Step `json:"steps"`
	// NextQuestID is the quest after this one, 0 on the last step.
	NextQuestID int64 `json:"next_quest_id,omitempty"`
	// Source is "dataset" or, for a chain built from the player's QuestieDB, "questie".
	Source string `json:"source,omitempty"`
	// Quest, Before and After are set only for a Questie chain: the quest itself, the quests it
	// follows and the quests it leads to.
	Quest  *Facts  `json:"quest,omitempty"`
	Before []Facts `json:"before,omitempty"`
	After  []Facts `json:"after,omitempty"`
}

// Total is the number of steps in the line.
func (c Chain) Total() int { return len(c.Steps) }

// Unlocks is true when another step follows this quest.
func (c Chain) Unlocks() bool { return c.Found && c.NextQuestID != 0 }

func i64(v any) int64 { n, _ := v.(int64); return n }

func str(v any) string { s, _ := v.(string); return s }

// Of returns the chain the quest sits in; when it is in several lines, the one with the lowest id.
// Steps are ordered by the table's position column when it has one, else by file order.
func Of(ds store.DataStore, questID int64) Chain {
	c := Chain{QuestID: questID, Steps: []Step{}}
	if ds == nil || questID <= 0 {
		return c
	}
	hits, err := ds.Filter("quest_lines", store.Range{Column: "quest_id", Min: questID, Max: questID})
	if err != nil || len(hits) == 0 {
		return c
	}
	line := i64(hits[0]["line_id"])
	for _, h := range hits[1:] {
		if l := i64(h["line_id"]); l < line {
			line = l
		}
	}
	rows, err := ds.Filter("quest_lines", store.Range{Column: "line_id", Min: line, Max: line})
	if err != nil || len(rows) == 0 {
		return c
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, aok := rows[i]["position"].(int64)
		b, bok := rows[j]["position"].(int64)
		return aok && bok && a < b
	})
	c.Found, c.LineID, c.Line = true, line, str(rows[0]["name"])
	for i, r := range rows {
		id := i64(r["quest_id"])
		c.Steps = append(c.Steps, Step{Position: i + 1, QuestID: id, Name: questName(ds, id)})
		if id == questID && c.Position == 0 {
			c.Position = i + 1
		}
	}
	if c.Position > 0 && c.Position < len(c.Steps) {
		c.NextQuestID = c.Steps[c.Position].QuestID
	}
	return c
}

// questName is the quest's name in the client tables, else the title players reported in quests_observed, else "".
func questName(ds store.DataStore, id int64) string {
	if row, err := ds.Get("quests", id); err == nil && str(row["name"]) != "" {
		return str(row["name"])
	}
	if f := observed(ds, id); f != nil {
		return f.Name
	}
	return ""
}

// Resolve turns a quest id or name into a quest id and name. The name index lists no quests, so a
// name is matched by scanning the quests table: exactly before partially; otherwise it returns the
// candidates and id 0.
func Resolve(ds store.DataStore, text string) (id int64, name string, candidates []string) {
	text = strings.TrimSpace(text)
	if n, err := strconv.ParseInt(text, 10, 64); err == nil && n > 0 {
		return n, questName(ds, n), nil
	}
	id, name, candidates = names.Resolve(ds, "quests", text)
	if id != 0 || len(candidates) > 0 {
		return id, name, candidates
	}
	return resolveObserved(ds, text)
}

// resolveObserved matches a quest name against the titles players reported in quests_observed (Everloot),
// for a dataset whose quests table carries no names: a title equal ignoring case, accents and apostrophes, else a title that
// holds the text. One quest is an answer; several are candidates, "Title (id)", for the player to pick by id.
func resolveObserved(ds store.DataStore, text string) (id int64, name string, candidates []string) {
	want := names.Normalize(text) // as the name index does: no case, accents or apostrophes
	if ds == nil || want == "" {
		return 0, "", nil
	}
	rows, err := ds.Filter("quests_observed", store.Range{Column: "quest_id", Min: int64(1)})
	if err != nil {
		return 0, "", nil
	}
	type hit struct {
		id    int64
		title string
	}
	var exact, partial []hit
	for _, r := range rows {
		title := str(r["title"])
		low := names.Normalize(title)
		switch {
		case title == "":
		case low == want:
			exact = append(exact, hit{i64(r["quest_id"]), title})
		case strings.Contains(low, want):
			partial = append(partial, hit{i64(r["quest_id"]), title})
		}
	}
	hits := exact
	if len(hits) == 0 {
		hits = partial
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].id < hits[j].id })
	if len(hits) == 1 {
		return hits[0].id, hits[0].title, nil
	}
	for _, h := range hits {
		if len(candidates) == 8 {
			break
		}
		candidates = append(candidates, h.title+" ("+strconv.FormatInt(h.id, 10)+")")
	}
	return 0, "", candidates
}

// Render is the plain-text form. name is the asked-for quest's name, which may be empty.
func Render(c Chain, name string) string {
	label := "quest " + strconv.FormatInt(c.QuestID, 10)
	if name != "" {
		label = name + " (" + label + ")"
	}
	if !c.Found {
		return "No quest line or follow-up quest is listed for " + label + " in the game data you have.\n"
	}
	if c.Source == SourceQuestie || c.Source == SourceEverloot {
		return renderQuestie(c, name)
	}
	var b strings.Builder
	b.WriteString(label + " is step " + strconv.Itoa(c.Position) + " of " + strconv.Itoa(c.Total()) + " in " + c.Line + " (line " + strconv.FormatInt(c.LineID, 10) + ")")
	if c.NextQuestID != 0 {
		b.WriteString("; next is quest " + strconv.FormatInt(c.NextQuestID, 10))
	} else {
		b.WriteString("; it is the last step")
	}
	b.WriteString(".\n")
	for _, s := range c.Steps {
		mark := "  "
		if s.Position == c.Position {
			mark = "> "
		}
		n := "quest " + strconv.FormatInt(s.QuestID, 10)
		if s.Name != "" {
			n = s.Name + " (" + n + ")"
		}
		b.WriteString(mark + strconv.Itoa(s.Position) + ". " + n + "\n")
	}
	return b.String()
}
