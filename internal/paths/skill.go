package paths

import (
	"fmt"
	"github.com/WoW-Companion/advisor/internal/character"
	"strings"

	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// ShortMax is the most skill points a character may be short of a recipe for a skill-up path to be
// worth offering; a larger gap is "far short" and leads with the other ways.
const ShortMax = 25

// Skill statuses.
const (
	CanMake       = "can_make"
	Short         = "short"
	Far           = "far"
	NotProfession = "not_profession"
	// NeedsRecipe: the skill reaches the recipe, but it is taught by an item the character does not have.
	NeedsRecipe = "needs_recipe"
)

// SkillCheck is a craft path's check of the character's profession skill against the recipe.
type SkillCheck struct {
	Profession  string        `json:"profession"`
	Have        int           `json:"have"` // the character's rank; 0 when they lack the profession
	Need        int           `json:"need"` // the recipe's learn skill
	Gap         int           `json:"gap"`  // points short, 0 when none
	Status      string        `json:"status" jsonschema:"can_make, needs_recipe, short, far or not_profession"`
	RecipeKnown *bool         `json:"recipe_known,omitempty" jsonschema:"whether the capture says the recipe is known; left out when the capture does not list recipes"`
	SkillUp     []SkillUpStep `json:"skill_up,omitempty" jsonschema:"when short: up to three recipes that give skill-ups now, estimates"`
}

// SkillUpStep is one recipe of a skill-up path. Crafts is an estimate: about one point per craft
// while the recipe is orange, about one per two while it is yellow.
type SkillUpStep struct {
	Recipe     string    `json:"recipe"`
	Creates    string    `json:"creates,omitempty"`
	Difficulty string    `json:"difficulty" jsonschema:"orange or yellow at the character's skill"`
	Crafts     int       `json:"crafts_estimate"`
	Reaches    int       `json:"reaches_skill"`
	Known      bool      `json:"known,omitempty"`
	LearnFirst bool      `json:"learn_first,omitempty" jsonschema:"the capture lists known recipes and this one is not among them: learn it at a trainer"`
	Missing    []Reagent `json:"missing,omitempty" jsonschema:"the materials still needed for those crafts"`
	Line       string    `json:"line"`
}

// RecipeSource says where the recipe is learned.
type RecipeSource struct {
	Kind   string         `json:"kind" jsonschema:"known, trainer, vendor, drop, quest or unknown"`
	Item   string         `json:"item,omitempty" jsonschema:"the recipe item that teaches it"`
	Line   string         `json:"line"`
	Offer  *vendors.Offer `json:"offer,omitempty"`
	Source string         `json:"source,omitempty"`
}

var crafterNames = map[string]string{
	"blacksmithing": "Blacksmiths", "tailoring": "Tailors", "leatherworking": "Leatherworkers",
	"engineering": "Engineers", "alchemy": "Alchemists", "enchanting": "Enchanters",
	"cooking": "Cooks", "jewelcrafting": "Jewelcrafters", "inscription": "Scribes",
}

func crafters(prof string) string {
	if n, ok := crafterNames[strings.ToLower(prof)]; ok {
		return n
	}
	return prof + " crafters"
}

// recipeItem finds the item that teaches a recipe: the recipe row's recipe_item_id when the dataset
// has one, else the recipe item (class 9) named for the recipe or the item it makes ("Plans: X").
func (f *finder) recipeItem(r store.Row) (int64, string) {
	if id := i64(r["recipe_item_id"]); id > 0 {
		if it, err := f.ds.Get("items", id); err == nil && str(it["name"]) != "" {
			return id, str(it["name"])
		}
	}
	names := []string{strings.ToLower(str(r["name"])), strings.ToLower(str(f.item["name"]))}
	rows, err := f.ds.Filter("items", store.Range{Column: "class_id", Min: int64(9), Max: int64(9)})
	if err != nil {
		return 0, ""
	}
	var best int64
	var bestName string
	for _, it := range rows {
		n := str(it["name"])
		i := strings.Index(n, ": ")
		if i < 0 {
			continue
		}
		tail := strings.ToLower(n[i+2:])
		for _, want := range names {
			if want != "" && tail == want && (best == 0 || i64(it["id"]) < best) {
				best, bestName = i64(it["id"]), n
			}
		}
	}
	return best, bestName
}

// recipeSource says where to learn the recipe: from the recipe item's vendor, drop or quest when
// the recipe comes from an item, else from a profession trainer.
func (f *finder) recipeSource(r store.Row, prof string) RecipeSource {
	id, name := f.recipeItem(r)
	if id == 0 {
		if res := learn.Recipe(f.c, f.ds, f.opt.Questie, r); len(res.Ways) > 0 && res.Ways[0].Kind == learn.Trainer {
			return RecipeSource{Kind: "trainer", Line: strings.TrimPrefix(res.Ways[0].Line, "Learn it ")}
		}
		return RecipeSource{Kind: "trainer", Line: fmt.Sprintf("from a %s trainer", prof)}
	}
	if o, ok := vendors.BestFor(f.c, f.ds, id); ok {
		l := fmt.Sprintf("buy the recipe %s from %s", name, o.Vendor)
		if o.Zone != "" {
			l += " in " + o.Zone
		}
		if o.Price != "" {
			l += " for " + o.Price
		}
		if o.Seen() {
			l += " (" + character.SeenByYou + ")"
		}
		return RecipeSource{Kind: "vendor", Item: name, Line: l, Offer: &o, Source: o.Source}
	}
	rows, _ := f.ds.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
	for _, l := range rows {
		switch {
		case kindHas(str(l["source_kind"]), "creature", "npc"):
			if n, err := f.ds.Get("npcs", i64(l["source_id"])); err == nil && str(n["name"]) != "" {
				s := fmt.Sprintf("the recipe %s drops from %s", name, str(n["name"]))
				if z := str(n["zone_name"]); z != "" {
					s += " in " + z
				}
				return RecipeSource{Kind: "drop", Item: name, Line: s}
			}
		case kindHas(str(l["source_kind"]), "quest"):
			if q, err := f.ds.Get("quests", i64(l["source_id"])); err == nil && str(q["name"]) != "" {
				return RecipeSource{Kind: "quest", Item: name, Line: fmt.Sprintf("the recipe %s is a reward of the quest %s", name, str(q["name"]))}
			}
		}
	}
	return RecipeSource{Kind: "unknown", Item: name, Line: fmt.Sprintf("it is taught by the recipe %s; the game data does not say where it comes from", name)}
}

// ahLine is the Auction House advice for a crafted item: only when it can be traded.
func (f *finder) tradeable() bool {
	return Tradeable(int(i64(f.item["bonding"])), int(i64(f.item["quality"])))
}

func (f *finder) skillUpSteps(prof string, have, need int, spell int64) []SkillUpStep {
	var out []SkillUpStep
	for _, s := range professions.SkillUp(f.c, f.ds, prof, need, spell) {
		st := SkillUpStep{Recipe: s.Recipe, Creates: s.Creates, Difficulty: s.Difficulty, Crafts: s.Crafts,
			Reaches: s.Reaches, Known: s.Known, LearnFirst: s.LearnFirst}
		var miss []string
		for _, g := range s.Reagents {
			if g.Missing > 0 {
				st.Missing = append(st.Missing, Reagent{Name: g.Name, Need: g.Need, Have: g.Have, Missing: g.Missing, ItemID: g.ItemID})
				miss = append(miss, fmt.Sprintf("%d %s", g.Missing, g.Name))
			}
		}
		per := "about 1 point per craft"
		if s.Difficulty == professions.Yellow {
			per = "about 1 point per 2 crafts"
		}
		st.Line = fmt.Sprintf("%s (%s, %s): about %d crafts to reach %d", s.Recipe, s.Difficulty, per, s.Crafts, s.Reaches)
		if s.LearnFirst {
			st.Line += "; learn it at a trainer first"
		}
		if len(miss) == 0 {
			st.Line += "; you have the materials"
		} else {
			st.Line += "; you still need " + strings.Join(miss, ", ")
		}
		out = append(out, st)
	}
	return out
}
