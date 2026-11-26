package professions

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// RouteMaterial is one material of a route step: the count for the whole step.
type RouteMaterial struct {
	ItemID int64 `json:"item_id"`
	Count  int   `json:"count"`
}

// RouteStep is one row of the dataset's profession_routes table: the efficient path of the
// wow-professions.com Forever guides, as facts.
type RouteStep struct {
	ID          int64
	Step        int
	From, To    int
	SpellID     int64 // 0 when the row did not resolve in the dataset
	Recipe      string
	Crafts      int
	Materials   []RouteMaterial
	LearnedFrom string
	Alternative int64 // the id or step number this row is an alternative of; 0 for a main step
	Notes       string
}

// Route is the main steps of a profession in order, with each step's alternatives.
type Route struct {
	Profession string
	Steps      []RouteStep
	Alts       map[int][]RouteStep // by main step number
}

func asNum(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int32:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}

// parseMaterials reads a materials cell: JSON ([{"item_id":1,"count":2}] or {"1":2}) or "1:2,3:4".
func parseMaterials(v any) []RouteMaterial {
	s := strings.TrimSpace(asText(v))
	if s == "" {
		return nil
	}
	var out []RouteMaterial
	var list []map[string]any
	if json.Unmarshal([]byte(s), &list) == nil {
		for _, m := range list {
			id, n := asNum(m["item_id"]), asNum(m["count"])
			if id == 0 {
				id = asNum(m["item"])
			}
			if id > 0 && n > 0 {
				out = append(out, RouteMaterial{id, int(n)})
			}
		}
		return out
	}
	for _, part := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' }) {
		a, b, ok := strings.Cut(strings.TrimSpace(part), ":")
		id, e1 := strconv.ParseInt(strings.TrimSpace(a), 10, 64)
		n, e2 := strconv.Atoi(strings.TrimSpace(b))
		if ok && e1 == nil && e2 == nil && id > 0 && n > 0 {
			out = append(out, RouteMaterial{id, n})
		}
	}
	return out
}

// LoadRoute reads a profession's route from the dataset; ok is false when the dataset has no
// profession_routes table or no usable step for the profession (an older dataset).
func LoadRoute(ds store.DataStore, profession string) (Route, bool) {
	r := Route{Profession: profession, Alts: map[int][]RouteStep{}}
	if ds == nil {
		return r, false
	}
	rows, err := ds.Filter("profession_routes", store.Range{Column: "profession", Min: profession, Max: profession})
	if err != nil {
		return r, false
	}
	var alts []RouteStep
	for _, row := range rows {
		s := RouteStep{ID: asNum(row["id"]), Step: int(asNum(row["step"])), From: int(asNum(row["skill_from"])), To: int(asNum(row["skill_to"])),
			SpellID: asNum(row["recipe_spell_id"]), Recipe: asText(row["recipe_name"]), Crafts: int(asNum(row["crafts"])),
			Materials: parseMaterials(row["materials"]), LearnedFrom: asText(row["learned_from"]),
			Alternative: asNum(row["alternative_of"]), Notes: asText(row["notes"])}
		if s.To <= s.From || s.Recipe == "" {
			continue
		}
		if s.Alternative > 0 {
			alts = append(alts, s)
		} else {
			r.Steps = append(r.Steps, s)
		}
	}
	if len(r.Steps) == 0 {
		return r, false
	}
	sort.SliceStable(r.Steps, func(i, j int) bool { return r.Steps[i].From < r.Steps[j].From })
	for _, a := range alts {
		for _, m := range r.Steps {
			if m.ID == a.Alternative || m.ID == 0 && m.Step == int(a.Alternative) {
				r.Alts[m.Step] = append(r.Alts[m.Step], a)
				break
			}
		}
	}
	return r, true
}

// Covering is the index of the main step covering the skill, -1 when the route ends below it.
func (r Route) Covering(skill int) int {
	for i, s := range r.Steps {
		if skill >= s.From && skill < s.To {
			return i
		}
	}
	return -1
}

// craftsLeft scales a step's crafts to the skill the player already has within it.
func (s RouteStep) craftsLeft(skill int) int {
	if skill <= s.From || s.To <= s.From {
		return s.Crafts
	}
	return (s.Crafts*(s.To-skill) + (s.To - s.From) - 1) / (s.To - s.From)
}

// Stock is a count of an item, with where it is.
type Stock struct{ Bags, Bank int }

func (s Stock) Total() int { return s.Bags + s.Bank }

// where reads "60 in bags, 32 in the bank".
func (s Stock) where() string {
	var p []string
	if s.Bags > 0 {
		p = append(p, fmt.Sprintf("%d in bags", s.Bags))
	}
	if s.Bank > 0 {
		p = append(p, fmt.Sprintf("%d in the bank", s.Bank))
	}
	return strings.Join(p, ", ")
}

// toolNames are the words of each profession's tool item name.
var toolWords = map[string][]string{
	"enchanting":    {"runed", "rod"},
	"mining":        {"mining pick"},
	"skinning":      {"skinning knife"},
	"blacksmithing": {"blacksmith hammer"},
}

// HasTool reports whether the character carries or wears the profession's tool; needed is false
// when the profession has none this code knows.
func HasTool(c character.Character, profession string) (has, needed bool) {
	words, ok := toolWords[strings.ToLower(profession)]
	if !ok {
		return false, false
	}
	match := func(name string) bool {
		n := strings.ToLower(name)
		for _, w := range words {
			if !strings.Contains(n, w) {
				return false
			}
		}
		return true
	}
	for _, s := range c.Bags {
		if match(s.Name) {
			return true, true
		}
	}
	for _, s := range c.Bank {
		if match(s.Name) {
			return true, true
		}
	}
	for _, e := range c.Equipment {
		if match(e.Name) {
			return true, true
		}
	}
	return false, true
}

func stockOf(c character.Character) map[int64]Stock {
	bags, bank := held(c)
	out := map[int64]Stock{}
	for id, n := range bags {
		s := out[id]
		s.Bags = n
		out[id] = s
	}
	for id, n := range bank {
		s := out[id]
		s.Bank = n
		out[id] = s
	}
	return out
}

func (s RouteStep) materialsText(ds store.DataStore, stock map[int64]Stock, skill int) string {
	crafts := s.craftsLeft(skill)
	var parts []string
	for _, m := range s.Materials {
		per := float64(m.Count) / float64(max(s.Crafts, 1))
		need := int(per*float64(crafts) + 0.999)
		name := itemName(ds, m.ItemID)
		if name == "" {
			name = fmt.Sprintf("item %d", m.ItemID)
		}
		st := stock[m.ItemID]
		t := fmt.Sprintf("%d %s", need, name)
		switch {
		case st.Total() == 0:
			t += " (you have none)"
		case st.Total() >= need:
			t += fmt.Sprintf(" (you have %d: %s, enough)", st.Total(), st.where())
		default:
			t += fmt.Sprintf(" (you have %d: %s; %d short)", st.Total(), st.where(), need-st.Total())
		}
		parts = append(parts, t)
	}
	return strings.Join(parts, "; ")
}

// RouteFacts is the route advice for a skill-up question: the step covering the player's skill and
// the next two, with crafts scaled to the skill, materials counted over bags and bank, the tier
// trainer, and the tool only when it is missing. "" when the profession has no route.
func RouteFacts(c character.Character, ds store.DataStore, qx *questie.Index, profession string) string {
	sk, ok := skillOf(c, profession)
	if !ok || sk.Rank == nil {
		return ""
	}
	route, ok := LoadRoute(ds, profession)
	if !ok {
		return ""
	}
	have := *sk.Rank
	i := route.Covering(have)
	if i < 0 {
		return ""
	}
	notes := LoadNotes(ds)
	stock := stockOf(c)
	var b strings.Builder
	fmt.Fprintf(&b, "%s skill-up ROUTE (the efficient path from the dataset's profession_routes, the wow-professions.com guide). Recommend exactly these steps, never other recipes from general knowledge. Their skill is %d.\n", profession, have)
	end := min(i+3, len(route.Steps))
	for k := i; k < end; k++ {
		s := route.Steps[k]
		lead := "Now"
		if k > i {
			lead = "Then"
		}
		start := max(have, s.From)
		fmt.Fprintf(&b, "%s: skill %d to %d: %d x %s", lead, start, s.To, s.craftsLeft(have), s.Recipe)
		if s.SpellID == 0 {
			b.WriteString(" (recipe not resolved in the game data)")
		}
		if m := s.materialsText(ds, stock, have); m != "" {
			b.WriteString("; materials: " + m)
		}
		if s.LearnedFrom != "" && k == i && !c.KnownSpells[s.SpellID] {
			b.WriteString("; learned from: " + s.LearnedFrom)
		}
		if s.Notes != "" {
			b.WriteString("; note: " + s.Notes)
		}
		b.WriteString(".\n")
		for _, a := range route.Alts[s.Step] {
			fmt.Fprintf(&b, "  Alternative for that step: %d x %s", a.craftsLeft(have), a.Recipe)
			if m := a.materialsText(ds, stock, have); m != "" {
				b.WriteString("; materials: " + m)
			}
			b.WriteString(".\n")
		}
	}
	if tier, need := tierTraining(sk, route, i, end, notes, have); need {
		fmt.Fprintf(&b, "Before step %s you must train %s %s (character level %d, skill %d). ", route.Steps[i].Recipe, profession, tier.Name, tier.MinLevel, tier.SkillMin)
		b.WriteString(trainerLine(c, ds, qx, profession))
		b.WriteString("\n")
	} else if k := firstUnknown(c, route, i, end); k >= 0 && strings.Contains(strings.ToLower(route.Steps[k].LearnedFrom), "trainer") {
		b.WriteString("Learn " + route.Steps[k].Recipe + " at a " + profession + " trainer. " + trainerLine(c, ds, qx, profession) + "\n")
	}
	if has, needed := HasTool(c, profession); needed && !has {
		b.WriteString("They have no " + profession + " tool (rod, pick or knife) in bags, bank or worn: they need one; ")
		hint := "buy it on the Auction House or from a vendor."
		for _, s := range route.Steps {
			if strings.Contains(strings.ToLower(s.Notes), "rod") || strings.Contains(strings.ToLower(s.Notes), "tool") {
				hint = s.Notes + "."
				break
			}
		}
		b.WriteString(hint + "\n")
	}
	b.WriteString("Trainers for this profession are the " + profession + " trainers named here, never a class trainer or quest NPC.\n")
	return b.String()
}

func firstUnknown(c character.Character, r Route, i, end int) int {
	if len(c.KnownSpells) == 0 {
		return -1
	}
	for k := i; k < end; k++ {
		if r.Steps[k].SpellID != 0 && !c.KnownSpells[r.Steps[k].SpellID] {
			return k
		}
	}
	return -1
}

// tierTraining says whether a step in range needs a rank the character has not trained: the step
// reaches past their skill cap. It returns the tier that opens at the cap.
func tierTraining(sk character.Skill, r Route, i, end int, n Notes, have int) (Tier, bool) {
	if sk.Max == nil || len(n.Tiers) == 0 {
		return Tier{}, false
	}
	for k := i; k < end; k++ {
		if r.Steps[k].To > *sk.Max && *sk.Max < n.Tiers[len(n.Tiers)-1].SkillCap {
			return n.TierAt(*sk.Max), true
		}
	}
	return Tier{}, false
}

// trainerLine names the profession's own trainers for the faction, nearest first.
func trainerLine(c character.Character, ds store.DataStore, qx *questie.Index, profession string) string {
	ways := learn.ProfessionTrainers(c, ds, qx, profession)
	if len(ways) == 0 {
		return "The game data names no " + profession + " trainer for their faction; ask in a capital city."
	}
	var l []string
	for _, w := range ways {
		l = append(l, w.Line)
	}
	return profession + " trainers, nearest first: " + strings.Join(l, "; ") + "."
}
