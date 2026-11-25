// Package professions plans profession leveling and lists the materials worth keeping, from the
// dataset's recipes and recipe_reagents tables and the captured skills, bags and bank. The rules
// are in docs/professions.md.
//
// Nothing is inferred: a recipe with no skill-up thresholds, a recipe with no reagent rows and a
// bank that was never captured are each reported as unknown.
package professions

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/items"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Plan sizes: the fewest and most recipes a plan lists.
const (
	MinRecipes = 3
	MaxRecipes = 5
)

// KeepWindow is how many skill points above the character's skill a recipe may be learned and
// still count for the keep list.
const KeepWindow = 50

// Difficulty of a recipe at the character's skill.
const (
	Orange  = "orange"  // below the yellow threshold: always a skill-up
	Yellow  = "yellow"  // from the yellow threshold up to grey: still a skill-up
	Unknown = "unknown" // the dataset has no thresholds for the recipe
)

// ErrNoRecipes is returned when the dataset has no recipes for a profession.
var ErrNoRecipes = errors.New("the dataset has no recipes for the profession")

// Recipe is the part of a recipes row the advisor uses. Rank fields are nil when unknown.
type Recipe struct {
	SpellID     int64
	Name        string
	CreatesItem int64 // 0 when the recipe makes no item
	LearnSkill  *int
	YellowSkill *int
	GreySkill   *int
	LearnedFrom string // trainer, item or both; empty when the column is absent
	LearnSource string // trainer, recipe_item, client or estimate; empty when the column is absent
}

// Reagent is one reagent of a planned recipe. Have counts bags and, when captured, bank.
type Reagent struct {
	ItemID  int64
	Name    string
	Need    int
	Have    int
	Missing int
}

// Step is one recipe in a plan.
type Step struct {
	Recipe        Recipe
	CreatesName   string // empty when the recipe makes no item or the dataset has no name for it
	Difficulty    string
	Reagents      []Reagent
	ReagentsKnown bool // false when the dataset lists no reagents for the recipe
}

// Leveling is the leveling plan for one profession, the result of Plan.
type Leveling struct {
	Profession string
	Skill      *int
	Max        *int
	Estimated  bool // Skill is estimated from the recipe colours, not captured: say "about"
	Steps      []Step
	BankKnown  bool     // false when the bank was never captured: Have counts bags only
	Notes      []string // everything unknown or missing, in plain words
}

// Keep is a bag or bank item that is a reagent for a recipe still worth making.
type Keep struct {
	ItemID      int64
	Name        string
	Count       int // bags plus bank
	Bags, Bank  int
	Professions []string
}

func asInt(v any) *int {
	n, ok := v.(int64)
	if !ok {
		return nil
	}
	i := int(n)
	return &i
}

func asText(v any) string {
	s, _ := v.(string)
	return s
}

// titleCase upper-cases the first letter of each word ("first aid" is "First Aid").
func titleCase(s string) string {
	w := strings.Fields(strings.ToLower(s))
	for i, x := range w {
		w[i] = strings.ToUpper(x[:1]) + x[1:]
	}
	return strings.Join(w, " ")
}

// Recipes returns the profession's recipes from the dataset, once per spell, or ErrNoRecipes.
func Recipes(ds store.DataStore, profession string) ([]Recipe, error) {
	if ds == nil {
		return nil, ErrNoRecipes
	}
	var rows []store.Row
	for _, name := range []string{profession, titleCase(profession)} {
		var err error
		rows, err = ds.Filter("recipes", store.Range{Column: "skill_line_name", Min: name, Max: name})
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNoRecipes, err)
		}
		if len(rows) > 0 {
			break
		}
	}
	seen := map[int64]bool{}
	var out []Recipe
	for _, r := range rows {
		id, _ := r["spell_id"].(int64)
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		if items.Unobtainable(asText(r["name"])) {
			continue
		}
		rc := Recipe{SpellID: id, Name: asText(r["name"]), LearnSkill: asInt(r["learn_skill"]),
			YellowSkill: asInt(r["yellow_skill"]), GreySkill: asInt(r["grey_skill"]), LearnedFrom: asText(r["learned_from"]),
			LearnSource: asText(r["learn_skill_source"])}
		if n := asInt(r["creates_item_id"]); n != nil {
			rc.CreatesItem = int64(*n)
		}
		rc.LearnSkill = estimateFloor(rc)
		if rc.CreatesItem != 0 {
			if it, err := ds.Get("items", rc.CreatesItem); err == nil && items.Unobtainable(asText(it["name"])) {
				continue
			}
		}
		out = append(out, rc)
	}
	if len(out) == 0 {
		return nil, ErrNoRecipes
	}
	return out, nil
}

// difficulty classifies a recipe at a skill: orange below the yellow threshold, yellow from there
// up to the grey threshold, Unknown when either is missing, "" when it gives no skill-up.
func difficulty(r Recipe, skill int) string {
	if r.YellowSkill == nil || r.GreySkill == nil {
		return Unknown
	}
	switch {
	case skill < *r.YellowSkill:
		return Orange
	case skill < *r.GreySkill:
		return Yellow
	}
	return ""
}

// estimateMargin is how far below its yellow skill an estimated recipe may be scheduled: an orange
// recipe turns yellow 15 to 20 skill points after it is learnt, and the dataset's estimates sit up to
// 30 below the yellow start, so a recipe only a guess puts at its learn skill is held back to this.
const estimateMargin = 10

// estimateFloor is the learn skill a recipe is planned at: the dataset's when it is a trainer's, a
// recipe item's or the client's, and for an estimate no lower than estimateMargin under its yellow
// skill, so a plan never uses a recipe far below the skill it is really learnt at. An estimate of
// 30 or less is the first recipes of a rank, taught at the start, and is left alone.
func estimateFloor(r Recipe) *int {
	if r.LearnSource != "estimate" || r.LearnSkill == nil || r.YellowSkill == nil || *r.LearnSkill <= 30 {
		return r.LearnSkill
	}
	if f := *r.YellowSkill - estimateMargin; f > *r.LearnSkill {
		return &f
	}
	return r.LearnSkill
}

func learnOf(r Recipe) int {
	if r.LearnSkill == nil {
		return -1
	}
	return *r.LearnSkill
}

// Select picks the recipes a character at skill should make next: those they can already learn
// (a recipe with no learn skill counts as learnable, and is noted), orange before yellow, and
// within a difficulty the highest learn skill first, then the lowest spell id. Recipes with no
// thresholds follow in the same learn-skill order; at most MaxRecipes are returned.
func Select(recipes []Recipe, skill int) []Step {
	var tiers = map[string][]Recipe{}
	for _, r := range recipes {
		if r.LearnSkill != nil && *r.LearnSkill > skill {
			continue
		}
		if d := difficulty(r, skill); d != "" {
			tiers[d] = append(tiers[d], r)
		}
	}
	var out []Step
	for _, d := range []string{Orange, Yellow, Unknown} {
		l := tiers[d]
		sort.Slice(l, func(i, j int) bool {
			if learnOf(l[i]) != learnOf(l[j]) {
				return learnOf(l[i]) > learnOf(l[j])
			}
			return l[i].SpellID < l[j].SpellID
		})
		for _, r := range l {
			if len(out) < MaxRecipes {
				out = append(out, Step{Recipe: r, Difficulty: d})
			}
		}
	}
	return out
}

// held counts an item over bags and bank.
func held(c character.Character) (bags, bank map[int64]int) {
	bags, bank = map[int64]int{}, map[int64]int{}
	for _, s := range c.Bags {
		bags[int64(s.ItemID)] += s.Count
	}
	for _, s := range c.Bank {
		bank[int64(s.ItemID)] += s.Count
	}
	return
}

func itemName(ds store.DataStore, id int64) string {
	if row, err := ds.Get("items", id); err == nil {
		if n := asText(row["name"]); n != "" {
			return n
		}
	}
	return ""
}

// namesTableItem is the name the dataset's names table holds for an item the items table has no
// name for; empty when neither does.
func namesTableItem(ds store.DataStore, id int64) string {
	for _, row := range rowsBy(ds, "names", "id", id) {
		if strings.EqualFold(asText(row["table"]), "items") || strings.EqualFold(asText(row["table"]), "item") {
			if n := asText(row["name"]); n != "" {
				return n
			}
		}
	}
	return ""
}

// reagentsOf reads the reagent rows of one spell, in item id order.
func reagentsOf(ds store.DataStore, spell int64) []store.Row {
	rows, err := ds.Filter("recipe_reagents", store.Range{Column: "spell_id", Min: spell, Max: spell})
	if err != nil {
		return nil
	}
	sort.Slice(rows, func(i, j int) bool {
		a, _ := rows[i]["reagent_item_id"].(int64)
		b, _ := rows[j]["reagent_item_id"].(int64)
		return a < b
	})
	return rows
}

// skillOf finds the captured profession by name, case-insensitively.
func skillOf(c character.Character, profession string) (character.Skill, bool) {
	for _, p := range c.Professions {
		if strings.EqualFold(p.Name, profession) {
			return p, true
		}
	}
	return character.Skill{}, false
}

// Plan lists the next recipes that still give skill-ups for one profession, with what each needs
// and how much of it the bags and bank hold.
func Plan(c character.Character, ds store.DataStore, profession string) Leveling {
	p := Leveling{Profession: profession, BankKnown: c.BankCaptured}
	sk, est, ok := skillFor(c, ds, profession)
	if ok {
		p.Profession, p.Skill, p.Max, p.Estimated = sk.Name, sk.Rank, sk.Max, est
	}
	if !ok || p.Skill == nil {
		p.Notes = append(p.Notes, "Skill level not captured: cannot say which recipes still give skill-ups.")
		return p
	}
	if est {
		p.Notes = append(p.Notes, fmt.Sprintf("Your %s skill is about %d, estimated from the recipe colours in your trade skill window.", p.Profession, *p.Skill))
	}
	recipes, err := Recipes(ds, p.Profession)
	if err != nil {
		p.Notes = append(p.Notes, "The dataset has no recipes for "+p.Profession+".")
		return p
	}
	p.Steps = Select(recipes, *p.Skill)
	bags, bank := held(c)
	unknownTiers, noReagents, noLearn := 0, 0, 0
	for i := range p.Steps {
		s := &p.Steps[i]
		if s.Recipe.CreatesItem != 0 {
			s.CreatesName = itemName(ds, s.Recipe.CreatesItem)
		}
		if s.Recipe.Name == "" {
			s.Recipe.Name = s.CreatesName
		}
		if s.Recipe.Name == "" {
			s.Recipe.Name = fmt.Sprintf("spell %d", s.Recipe.SpellID)
		}
		if s.Difficulty == Unknown {
			unknownTiers++
		}
		if s.Recipe.LearnSkill == nil {
			noLearn++
		}
		rows := reagentsOf(ds, s.Recipe.SpellID)
		s.ReagentsKnown = len(rows) > 0
		if !s.ReagentsKnown {
			noReagents++
		}
		for _, r := range rows {
			id, _ := r["reagent_item_id"].(int64)
			need := 0
			if n := asInt(r["count"]); n != nil {
				need = *n
			}
			name := itemName(ds, id)
			if name == "" {
				name = fmt.Sprintf("item %d", id)
			}
			have := bags[id] + bank[id]
			s.Reagents = append(s.Reagents, Reagent{id, name, need, have, max(0, need-have)})
		}
	}
	if len(p.Steps) == 0 {
		p.Notes = append(p.Notes, "No recipe in the dataset gives a skill-up at skill "+fmt.Sprint(*p.Skill)+".")
	}
	if unknownTiers > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("%d recipe(s) have no skill-up thresholds in the dataset: ordered by learn skill, and whether they give skill-ups is unknown.", unknownTiers))
	}
	if noLearn > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("%d recipe(s) have no learn skill in the dataset: whether you can learn them is unknown.", noLearn))
	}
	if noReagents > 0 {
		p.Notes = append(p.Notes, fmt.Sprintf("%d recipe(s) list no reagents in the dataset: materials unknown.", noReagents))
	}
	if !c.BankCaptured {
		p.Notes = append(p.Notes, "Bank not captured: open your bank once; counts are bags only.")
	}
	return p
}

// KeepList returns the bag and bank items that are reagents of the character's professions, for
// recipes learnable within KeepWindow skill points of their current skill and not yet grey, most
// numerous first. It returns ErrNoRecipes when the dataset has no recipes for any of the
// professions (the caller then falls back to a guess); a profession whose skill was not captured
// is skipped.
func KeepList(c character.Character, ds store.DataStore) ([]Keep, error) {
	if ds == nil {
		return nil, ErrNoRecipes
	}
	reagent := map[int64][]string{} // item id -> professions
	any := false
	for _, pr := range c.Professions {
		recipes, err := Recipes(ds, pr.Name)
		if err != nil {
			continue
		}
		any = true
		if pr.Rank == nil {
			continue
		}
		for _, r := range recipes {
			if r.LearnSkill == nil || *r.LearnSkill > *pr.Rank+KeepWindow || r.GreySkill != nil && *r.GreySkill <= *pr.Rank {
				continue
			}
			for _, row := range reagentsOf(ds, r.SpellID) {
				id, _ := row["reagent_item_id"].(int64)
				if l := reagent[id]; len(l) == 0 || l[len(l)-1] != pr.Name {
					reagent[id] = append(l, pr.Name)
				}
			}
		}
	}
	if !any {
		return nil, ErrNoRecipes
	}
	bags, bank := held(c)
	names := map[int64]string{}
	for _, l := range [][]character.ItemStack{c.Bags, c.Bank} {
		for _, s := range l {
			if names[int64(s.ItemID)] == "" {
				names[int64(s.ItemID)] = s.Name
			}
		}
	}
	var out []Keep
	for id, profs := range reagent {
		n := bags[id] + bank[id]
		if n == 0 {
			continue
		}
		name := names[id]
		if name == "" {
			name = itemName(ds, id)
		}
		if name == "" {
			name = fmt.Sprintf("item %d", id)
		}
		out = append(out, Keep{id, name, n, bags[id], bank[id], profs})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ItemID < out[j].ItemID
	})
	return out, nil
}

// FarMaterials returns the reagent item ids, with their professions, of recipes the character
// cannot learn for more than KeepWindow skill points: materials to bank, not to carry. A
// profession with no captured skill or no recipes in the dataset contributes nothing.
func FarMaterials(c character.Character, ds store.DataStore) map[int64][]string {
	out := map[int64][]string{}
	if ds == nil {
		return out
	}
	for _, pr := range c.Professions {
		recipes, err := Recipes(ds, pr.Name)
		if err != nil || pr.Rank == nil {
			continue
		}
		for _, r := range recipes {
			if r.LearnSkill == nil || *r.LearnSkill <= *pr.Rank+KeepWindow {
				continue
			}
			for _, row := range reagentsOf(ds, r.SpellID) {
				id, _ := row["reagent_item_id"].(int64)
				if l := out[id]; len(l) == 0 || l[len(l)-1] != pr.Name {
					out[id] = append(l, pr.Name)
				}
			}
		}
	}
	return out
}

// Trades are the professions whose recipes ReagentUsers reads.
var Trades = []string{"Alchemy", "Blacksmithing", "Enchanting", "Engineering", "Leatherworking", "Tailoring", "Cooking", "First Aid"}

// ReagentUsers maps each reagent item id to the professions whose recipes use it, at any skill. A
// profession the dataset has no recipes for contributes nothing, and ok is false when none do.
func ReagentUsers(ds store.DataStore) (users map[int64][]string, ok bool) {
	users = map[int64][]string{}
	for _, p := range Trades {
		recipes, err := Recipes(ds, p)
		if err != nil {
			continue
		}
		ok = true
		for _, r := range recipes {
			for _, row := range reagentsOf(ds, r.SpellID) {
				id, _ := row["reagent_item_id"].(int64)
				if l := users[id]; len(l) == 0 || l[len(l)-1] != p {
					users[id] = append(l, p)
				}
			}
		}
	}
	return users, ok
}
