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
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// RouteMaterial is one material of a route step: the count one craft uses (the dataset's profession_routes
// holds materials per craft).
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
	// Gather is true for a gathering step (Herbalism, Mining, Skinning, Fishing): no recipe, the zones in
	// Where, and the materials listed are what to gather (count 0).
	Gather bool
	Where  []string
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

// columnMaterials reads the dataset's materials column as the store returns it, a list of {item_id, name,
// count} structs flattened column by column: every item id, then every name, then every count. A count of
// 0 stays (a gathering step names what to gather, not how many).
func columnMaterials(l []any) []RouteMaterial {
	n := len(l) / 3
	if n == 0 || len(l) != 3*n {
		return nil
	}
	var out []RouteMaterial
	for i := 0; i < n; i++ {
		id, ok1 := l[i].(int64)
		c, ok2 := l[2*n+i].(int64)
		if ok1 && ok2 && id > 0 && c >= 0 {
			out = append(out, RouteMaterial{id, int(c)})
		}
	}
	return out
}

func asStrings(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, x := range l {
		if s, ok := x.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// parseMaterials reads a materials cell: the dataset's struct list (see columnMaterials), JSON
// ([{"item_id":1,"count":2}] or {"1":2}) or "1:2,3:4".
func parseMaterials(v any) []RouteMaterial {
	if l, ok := v.([]any); ok {
		return columnMaterials(l)
	}
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
			Alternative: asNum(row["alternative_of"]), Notes: asText(row["notes"]),
			Gather: asText(row["kind"]) == "gather", Where: asStrings(row["where"])}
		if s.To <= s.From || s.Recipe == "" && !s.Gather {
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
	"engineering":   {"arclight spanner"},
	"mining":        {"mining pick"},
	"skinning":      {"skinning knife"},
	"blacksmithing": {"blacksmith hammer"},
	"fishing":       {"fishing pole"},
}

// toolItems are the base tool item of each profession that has one, named as the dataset names it.
var toolItems = map[string]string{
	"enchanting":    "Runed Copper Rod",
	"engineering":   "Arclight Spanner",
	"mining":        "Mining Pick",
	"skinning":      "Skinning Knife",
	"blacksmithing": "Blacksmith Hammer",
	"fishing":       "Fishing Pole",
}

// toolHint says where to get the profession's tool: the item by name, and the vendor price when the dataset has
// vendor rows for it ("a Mining Pick, sold by vendors for 10c"); "" when the profession has no tool item the dataset names.
func toolHint(ds store.DataStore, profession string) string {
	name := toolItems[strings.ToLower(profession)]
	if name == "" || ds == nil {
		return ""
	}
	rows, err := ds.Filter("items", store.Range{Column: "name", Min: name, Max: name})
	if err != nil || len(rows) == 0 {
		return ""
	}
	id := asNum(rows[0]["id"])
	if len(rowsBy(ds, "vendor_sources", "item_id", id)) == 0 {
		return ""
	}
	t := "buy " + name + " from a vendor"
	if m := vendors.Money(asNum(rows[0]["buy_price"])); m != "" {
		t += " (" + m + ")"
	}
	return t + "."
}

// npcProp is true for a creature's held prop the dataset lists as an item ("Monster - Mining Pick", "Monster - Item, Fishing
// Pole"): it is no tool a player can carry or use, so it counts for no profession.
func npcProp(lowerName string) bool { return strings.HasPrefix(lowerName, "monster - ") }

// ToolOf names the profession whose tool an item is, by the item's name ("Mining Pick" is Mining's; a fishing pole is
// Fishing's); "" for any other item.
func ToolOf(name string) string {
	n := strings.ToLower(name)
	if npcProp(n) {
		return ""
	}
	for _, prof := range []string{"Enchanting", "Engineering", "Mining", "Skinning", "Blacksmithing", "Fishing"} {
		found := true
		for _, w := range toolWords[strings.ToLower(prof)] {
			found = found && strings.Contains(n, w)
		}
		if found {
			return prof
		}
	}
	return ""
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
		if npcProp(n) {
			return false
		}
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

// gatherText is a gathering step: "Now: skill 20 to 70: gather in Durotar, Mulgore; gather: Peacebloom, Silverleaf".
func (s RouteStep) gatherText(ds store.DataStore, lead string, from int) string {
	t := fmt.Sprintf("%s: skill %d to %d: gather", lead, from, s.To)
	if len(s.Where) > 0 {
		t += " in " + strings.Join(s.Where, ", ")
	}
	var what []string
	for _, m := range s.Materials {
		name := itemName(ds, m.ItemID)
		if name == "" {
			name = fmt.Sprintf("item %d", m.ItemID)
		}
		what = append(what, name)
	}
	if len(what) > 0 {
		t += "; gather: " + strings.Join(what, ", ")
	}
	return t
}

func (s RouteStep) materialsText(ds store.DataStore, stock map[int64]Stock, skill int) string {
	crafts := s.craftsLeft(skill)
	var parts []string
	for _, m := range s.Materials {
		need := m.Count * crafts
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
		if s.Gather {
			b.WriteString(s.gatherText(ds, lead, start))
		} else {
			fmt.Fprintf(&b, "%s: skill %d to %d: %d x %s", lead, start, s.To, s.craftsLeft(have), s.Recipe)
			if s.SpellID == 0 {
				b.WriteString(" (recipe not resolved in the game data)")
			}
			if m := s.materialsText(ds, stock, have); m != "" {
				b.WriteString("; materials: " + m)
			}
		}
		if s.LearnedFrom != "" && k == i && !s.Gather && !c.KnownSpells[s.SpellID] {
			b.WriteString("; learned from: " + s.LearnedFrom)
		}
		if s.Notes != "" {
			b.WriteString("; note: " + s.Notes)
		}
		b.WriteString(".\n")
		for _, a := range route.Alts[s.Step] {
			if a.To <= have {
				continue // an alternative whose skill range the player has already passed has no crafts left
			}
			fmt.Fprintf(&b, "  Alternative for that step: %d x %s", a.craftsLeft(have), a.Recipe)
			if m := a.materialsText(ds, stock, have); m != "" {
				b.WriteString("; materials: " + m)
			}
			b.WriteString(".\n")
		}
	}
	if tier, at, need := tierTraining(sk, route, i, end, notes, have); need {
		gate := route.Steps[at]
		lead := "Before the skill " + fmt.Sprintf("%d to %d", max(have, gate.From), gate.To) + " step"
		if gate.Recipe != "" {
			lead = "Before step " + gate.Recipe
		}
		// the rank is gained at the skill and character level the dataset's profession_tiers gives (the notes' ranks when
		// it has no row); the rank before it stops at tier.SkillMin, which is when it is needed, not when it opens
		g, _ := tierGate(ds, profession, tier.Name, c.Faction)
		level := g.Level
		if level == 0 {
			level = tier.MinLevel
		}
		needs := fmt.Sprintf("character level %d, skill %d; the rank you hold stops at %d", level, g.Skill, tier.SkillMin)
		if g.Skill == 0 {
			needs = fmt.Sprintf("character level %d; the game data holds no skill gate for it, and the rank you hold stops at %d", level, tier.SkillMin)
		}
		if c.Level != nil && *c.Level < level {
			needs += fmt.Sprintf("; you are level %d, so not yet", *c.Level)
		}
		if how, text := tierAccess(ds, profession, tier.Name, c.Faction); how != "" {
			fmt.Fprintf(&b, "%s you must get %s %s (%s): %s. %s\n", lead, profession, tier.Name, needs, text, g.price())
		} else {
			fmt.Fprintf(&b, "%s you must train %s %s (%s). %s ", lead, profession, tier.Name, needs, g.price())
			if t := tierTrainer(ds, profession, tier.Name, c.Faction); t != "" {
				b.WriteString(profession + " " + tier.Name + " trainer: " + t + ".")
			} else {
				b.WriteString(trainerLine(c, ds, qx, profession))
			}
			b.WriteString("\n")
		}
	} else if k := firstUnknown(c, route, i, end); k >= 0 && strings.Contains(strings.ToLower(route.Steps[k].LearnedFrom), "trainer") {
		b.WriteString("Learn " + route.Steps[k].Recipe + " at a " + profession + " trainer. " + trainerLine(c, ds, qx, profession) + "\n")
	}
	if has, needed := HasTool(c, profession); needed && !has {
		tool := toolItems[strings.ToLower(profession)]
		if tool == "" {
			tool = profession + " tool"
		}
		b.WriteString("They have no " + tool + " (the " + profession + " tool) in bags, bank or worn: they need one; ")
		hint := "buy it on the Auction House or from a vendor."
		if h := toolHint(ds, profession); h != "" {
			hint = h
		}
		for _, s := range route.Steps {
			if strings.EqualFold(profession, "Enchanting") && strings.Contains(strings.ToLower(s.Notes), "rod") || strings.Contains(strings.ToLower(s.Notes), "tool") {
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
// reaches past their skill cap. It returns the tier that opens at the cap and the index of the first step that
// needs it.
func tierTraining(sk character.Skill, r Route, i, end int, n Notes, have int) (Tier, int, bool) {
	if sk.Max == nil || len(n.Tiers) == 0 {
		return Tier{}, 0, false
	}
	for k := i; k < end; k++ {
		if r.Steps[k].To > *sk.Max && *sk.Max < n.Tiers[len(n.Tiers)-1].SkillCap {
			return n.TierAt(*sk.Max), k, true
		}
	}
	return Tier{}, 0, false
}

// tierTrainer is the trainer the dataset's profession_tiers names for the tier on the character's side, with its
// place ("Doctor Herbert Halsey in Undercity"); "" when no row of the tier names one. Several are joined by "; ".
func tierTrainer(ds store.DataStore, profession, tier, faction string) string {
	rows, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: profession, Max: profession})
	var out []string
	for _, r := range rows {
		name, place := asText(r["trainer_name"]), asText(r["place"])
		if name == "" || !strings.EqualFold(asText(r["tier"]), tier) || asText(r["how"]) != "trainer" {
			continue
		}
		if f := asText(r["faction"]); f != "" && !strings.EqualFold(f, "both") && faction != "" && !strings.EqualFold(f, faction) {
			continue
		}
		if place != "" {
			name += " in " + place
		}
		out = append(out, name)
	}
	return strings.Join(out, "; ")
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
