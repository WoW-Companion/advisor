package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline recipe-learn benchmark: what a player needs before a recipe can be learned (the skill, the profession rank that skill
// belongs to and the character level that rank is trained from, and where the recipe is taught), which recipes are held behind a
// reputation standing, and which recipes a character of a given level and skill can already use, beside the dataset's rows, with no
// network. Skills read testdata/tradeskills_snapshot (recipes.learn_skill, trainer_spells, recipe_items, recipe_reagents) and the
// ranks of its profession_notes (Journeyman from skill 75 at level 10, Expert from 150 at level 20, Artisan from 225 at level 35),
// which the Forever guide's route steps repeat as "needs Expert, level 20". The dataset holds no faction or standing for a recipe or a
// vendor, so the reputation cases check that the advisor names none and says where the dataset is silent; a difference the dataset's
// own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	rlSkillSrc  = "dataset recipes (learn_skill, learn_skill_source, learned_from), trainer_spells, recipe_items; profession_notes ranks (skill_min, skill_cap, min_level); guide route notes (\"needs Expert, level 20\")"
	rlRepSrc    = "dataset recipes (learned_from item), recipe_items; Warcraft Wiki, Thorium Brotherhood (Lokhtos Darkbargainer sells the plans by standing); the dataset has no faction or standing column"
	rlUseSrc    = "dataset recipes (learn_skill), profession_notes ranks (min_level); the character's skill and its cap in the capture"
	rlNoneSrc   = "the dataset has no such row, column or table"
	rlNoRankMsg = "no rank stated"
)

// rlRank is the rank a skill needs by the dataset's profession_notes, read here straight from the rows: the rank whose range holds the
// skill above its floor, with the floor and the level; none for a skill the first rank reaches.
func rlRank(t *testing.T, ds store.DataStore, skill int64) (name string, floor, level int64) {
	t.Helper()
	rows, err := ds.Filter("profession_notes", store.Range{Column: "section", Min: "ranks", Max: "ranks"})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		lo, hi := num(r["skill_min"]), num(r["skill_cap"])
		if lo > 1 && lo < skill && skill <= hi {
			return text(r["subject"]), lo, num(r["min_level"])
		}
	}
	return "", 0, 0
}

func rlWant(name string, floor, level int64) string {
	if name == "" {
		return rlNoRankMsg
	}
	return fmt.Sprintf("above %d the %s rank, from level %d", floor, name, level)
}

// ---- what is needed to learn a recipe ----

type rlLearnCase struct {
	recipe, prof string
	level, skill int // the character's level, and its skill in the profession (0 for none)
}

var rlLearnCases = []rlLearnCase{
	{"Heavy Linen Bandage", "First Aid", 5, 0}, {"Copper Chain Belt", "Blacksmithing", 10, 0}, {"Silk Bandage", "First Aid", 8, 0},
	{"Silk Bandage", "First Aid", 30, 0}, {"Mageweave Bag", "Tailoring", 15, 0}, {"Mageweave Bag", "Tailoring", 40, 0},
	{"Mageweave Bag", "Tailoring", 15, 190}, {"Thorium Bracers", "Blacksmithing", 25, 0}, {"Thorium Belt", "Blacksmithing", 30, 0},
	{"Imperial Plate Bracers", "Blacksmithing", 30, 0}, {"Imperial Plate Bracers", "Blacksmithing", 50, 0}, {"Superior Mana Potion", "Alchemy", 50, 0},
}

func TestOfflineBenchmarkRecipeLearnRequirementA(t *testing.T) { rlLearn(t, "a", rlLearnCases[:4]) }
func TestOfflineBenchmarkRecipeLearnRequirementB(t *testing.T) { rlLearn(t, "b", rlLearnCases[4:8]) }
func TestOfflineBenchmarkRecipeLearnRequirementC(t *testing.T) { rlLearn(t, "c", rlLearnCases[8:]) }

// rlLearn: the skill the recipe answer asks for is the dataset's, the rank and the character level that skill needs are the
// profession_notes', and a character under that level who has not reached the skill is told "not yet" (never one who has).
func rlLearn(t *testing.T, part string, cases []rlLearnCase) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range cases {
		rec := ccRecipe(t, ds, lgItemID(t, ds, tc.recipe), tc.prof)
		need := num(rec["learn_skill"])
		c := tsChar("Horde", tc.level)
		if tc.skill > 0 {
			c = tsChar("Horde", tc.level, tsSkill{tc.prof, tc.skill, 225})
		}
		res, err := learn.Find(c, ds, nil, tc.recipe)
		name, floor, level := rlRank(t, ds, need)
		want := rlWant(name, floor, level)
		got := rlNoRankMsg
		if res.Gate.Rank != "" {
			got = fmt.Sprintf("above %d the %s rank, from level %d", floor, res.Gate.Rank, res.Gate.Level)
		}
		ok := err == nil && int64(res.Gate.Skill) == need && res.Gate.Rank == name && int64(res.Gate.Level) == level &&
			strings.Contains(res.Text, fmt.Sprintf("It needs %s%s %d.", map[bool]string{true: "about ", false: ""}[text(rec["learn_skill_source"]) == "estimate"], tc.prof, need))
		if name != "" {
			ok = ok && strings.Contains(res.Text, fmt.Sprintf("Skill above %d needs the %s rank, trained from character level %d.", floor, name, level))
			notYet := int64(tc.level) < level && tc.skill < int(need)
			ok = ok && strings.Contains(res.Text, fmt.Sprintf("You are level %d, so not yet.", tc.level)) == notYet
		} else {
			ok = ok && !strings.Contains(res.Text, "rank")
		}
		label := fmt.Sprintf("%s (%s): learn at level %d", tc.recipe, tc.prof, tc.level)
		if tc.skill > 0 {
			label += fmt.Sprintf(", skill %d", tc.skill)
		}
		out = append(out, benchRow{label, fmt.Sprintf("skill %d, %s", res.Gate.Skill, got), fmt.Sprintf("learn_skill %d (%s); %s", need, text(rec["learn_skill_source"]), want), rlSkillSrc,
			resultIf(ok, "the skill, the rank or the character level that skill needs, or a \"not yet\" said to a character who is past it")})
	}
	finish(t, "advice-recipe-learn-requirement-"+part, "Offline recipe learning: the skill, rank and character level a recipe needs ("+part+")", out)
}

// ---- recipes held behind a reputation standing ----

var rlRepCases = []struct{ recipe, prof, standing string }{
	{"Corehound Boots", "Leatherworking", "Friendly"}, {"Flarecore Gloves", "Tailoring", "Friendly"}, {"Dark Iron Helm", "Blacksmithing", "Honored"},
	{"Lava Belt", "Leatherworking", "Honored"}, {"Chromatic Gauntlets", "Leatherworking", "Revered"}, {"Dark Iron Boots", "Blacksmithing", "Exalted"},
}

func TestOfflineBenchmarkReputationRecipeA(t *testing.T) { rlReputation(t, "a", rlRepCases[:3]) }
func TestOfflineBenchmarkReputationRecipeB(t *testing.T) { rlReputation(t, "b", rlRepCases[3:]) }

// rlReputation: a recipe the dataset says an item teaches, whose plan a reputation vendor sells: for a crafter who has the skill the
// craft answer says the recipe comes from an item the dataset does not place, never from a trainer, states no faction or standing, and
// names the rank and level the skill needs; the standing itself is a dataset gap.
func rlReputation(t *testing.T, part string, cases []struct{ recipe, prof, standing string }) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range cases {
		rec := ccRecipe(t, ds, lgItemID(t, ds, tc.recipe), tc.prof)
		need := num(rec["learn_skill"])
		c := tsChar("Horde", 60, tsSkill{tc.prof, 300, 300})
		p, found := ccCraft(paths.For(c, ds, lgItemID(t, ds, tc.recipe), paths.Options{}))
		line, kind := "no recipe source", ""
		if found && p.Recipe != nil {
			line, kind = p.Recipe.Line, p.Recipe.Kind
		}
		res, err := learn.Find(c, ds, nil, tc.recipe)
		name, floor, level := rlRank(t, ds, need)
		ok := found && err == nil && text(rec["learned_from"]) == "item" && kind == "unknown" && !strings.Contains(strings.ToLower(line), "trainer") &&
			!standing.MatchString(line+" "+res.Text) && res.Gate.Rank == name && int64(res.Gate.Level) == level &&
			strings.Contains(res.Text, "names no place that sells or drops it")
		out = append(out, benchRow{tc.recipe + ": where the recipe is learned, and what it needs", line + " / " + res.Gate.Line + " " + rlWant(res.Gate.Rank, floor, int64(res.Gate.Level)),
			fmt.Sprintf("%s %d (%s), learned from %s; rank %s", tc.prof, need, text(rec["learn_skill_source"]), text(rec["learned_from"]), rlWant(name, floor, level)), rlRepSrc,
			resultIf(ok, "a trainer, a place or a standing was stated, or the rank and level are missing")})
		out = append(out, benchRow{tc.recipe + ": the standing that unlocks the plan", "none stated", "Thorium Brotherhood, " + tc.standing, rlRepSrc, "differ (data): the dataset has no faction or standing for a plan or a vendor"})
	}
	finish(t, "advice-recipe-learn-reputation-"+part, "Offline recipe learning: recipes held behind a reputation standing ("+part+")", out)
}

// ---- recipes a character of a level and skill can use ----

type rlUseCase struct {
	label, item, prof string
	level, rank, max  int
	want              string // can, short or far
}

var rlUseCases = []rlUseCase{
	{"level 15, Tailoring 140 of 150", "Mageweave Bag", "Tailoring", 15, 140, 150, "far"},
	{"level 25, Tailoring 150 of 150", "Mageweave Bag", "Tailoring", 25, 150, 150, "far"},
	{"level 40, Blacksmithing 225 of 225", "Thorium Bracers", "Blacksmithing", 40, 225, 225, "short"},
	{"level 8, Alchemy 70 of 75", "Elixir of Wisdom", "Alchemy", 8, 70, 75, "short"},
	{"level 20, First Aid 150 of 150, recipe within the rank", "Silk Bandage", "First Aid", 20, 150, 150, "can"},
	{"level 60, Blacksmithing 300 of 300", "Thorium Bracers", "Blacksmithing", 60, 300, 300, "can"},
}

func TestOfflineBenchmarkRecipeUsableByLevelA(t *testing.T) { rlUse(t, "a", rlUseCases[:3]) }
func TestOfflineBenchmarkRecipeUsableByLevelB(t *testing.T) { rlUse(t, "b", rlUseCases[3:]) }

// rlUse: a recipe past the rank cap the character has trained says which rank it needs and the character level that rank is trained from
// (and "not yet" under that level); a recipe within the cap says nothing of ranks and is makeable or short by the skill alone.
func rlUse(t *testing.T, part string, cases []rlUseCase) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range cases {
		id := lgItemID(t, ds, tc.item)
		rec := ccRecipe(t, ds, id, tc.prof)
		need := int(num(rec["learn_skill"]))
		p, found := ccCraft(paths.For(tsChar("Horde", tc.level, tsSkill{tc.prof, tc.rank, tc.max}), ds, id, paths.Options{}))
		if !found || p.Skill == nil {
			out = append(out, benchRow{tc.label + ": " + tc.item, "no way", "", rlUseSrc, "differ: no craft way"})
			continue
		}
		name, floor, level := rlRank(t, ds, int64(need))
		past := need > tc.max
		gap := need - tc.rank
		status := map[string]string{"can": paths.CanMake, "short": paths.Short, "far": paths.Far}[tc.want]
		ok := p.Skill.Status == status && (tc.want == "can") == (gap <= 0)
		if past {
			sentence := fmt.Sprintf("Skill above %d needs the %s rank, trained from character level %d.", floor, name, level)
			ok = ok && p.Skill.Rank == name && int64(p.Skill.RankLevel) == level && strings.Contains(p.Line, sentence) &&
				strings.Contains(p.Line, fmt.Sprintf("You are level %d, so not yet.", tc.level)) == (int64(tc.level) < level)
		} else {
			ok = ok && p.Skill.Rank == "" && !strings.Contains(p.Line, "rank")
		}
		ref := fmt.Sprintf("needs skill %d; the character has %d of %d; %s", need, tc.rank, tc.max, rlWant(map[bool]string{true: name, false: ""}[past], floor, level))
		out = append(out, benchRow{tc.label + ": " + tc.item, firstLine(p.Line), ref, rlUseSrc, resultIf(ok, "the skill status, or the rank and level a recipe past the character's rank cap needs")})
	}
	finish(t, "advice-recipe-learn-usable-"+part, "Offline recipe learning: recipes usable at a level and skill ("+part+")", out)
}

// ---- where the dataset has no answer ----

// rlNoRanks hides the ranks of the notes: a dataset that lists no ranks.
type rlNoRanks struct{ store.DataStore }

func (n rlNoRanks) Filter(table string, r ...store.Range) ([]store.Row, error) {
	if table == "profession_notes" {
		return nil, nil
	}
	return n.DataStore.Filter(table, r...)
}

// TestOfflineBenchmarkRecipeLearnNoAnswer: where the dataset gives no number or no row the advisor states none: a trainer recipe whose
// only skill is the client minimum, a dataset that lists no ranks, a character whose rank cap the capture does not hold, a recipe the
// dataset lacks, and the standing of a plan.
func TestOfflineBenchmarkRecipeLearnNoAnswer(t *testing.T) {
	ds := consVendors{tsStore()}
	var out []benchRow
	{
		res, err := learn.Find(tsChar("Horde", 30), ds, nil, "Minor Healing Potion")
		rec := ccRecipe(t, ds, lgItemID(t, ds, "Minor Healing Potion"), "First Aid")
		ok := err == nil && num(rec["learn_skill"]) <= 1 && res.Gate.Unknown && res.Gate.Skill == 0 && res.Gate.Rank == "" && !strings.Contains(res.Text, "needs")
		out = append(out, benchRow{"Minor Healing Potion: a trainer recipe at the client minimum", firstLine(res.Gate.Line), fmt.Sprintf("learn_skill %d on a trainer recipe: no number, no rank", num(rec["learn_skill"])), rlNoneSrc,
			resultIf(ok, "a skill, a rank or a level was stated")})
	}
	{
		res, err := learn.Find(tsChar("Horde", 15), rlNoRanks{ds}, nil, "Mageweave Bag")
		ok := err == nil && res.Gate.Skill == 190 && res.Gate.Rank == "" && res.Gate.Level == 0 && !strings.Contains(res.Text, "rank")
		out = append(out, benchRow{"Mageweave Bag: a dataset with no profession ranks", res.Gate.Line, "no profession_notes rows: the skill is stated, no rank or level", rlNoneSrc, resultIf(ok, "a rank or a level was invented")})
	}
	{
		lvl := 15
		rank := 140
		c := character.Character{Faction: "Horde", Class: "MAGE", Level: &lvl, Professions: []character.Skill{{Name: "Tailoring", Rank: &rank}}}
		id := lgItemID(t, ds, "Mageweave Bag")
		p, found := ccCraft(paths.For(c, ds, id, paths.Options{}))
		ok := found && p.Skill != nil && p.Skill.Status == paths.Far && p.Skill.Rank == "" && !strings.Contains(p.Line, "rank")
		out = append(out, benchRow{"Mageweave Bag: a skill whose rank cap the capture does not hold", firstLine(p.Line), "no cap in the capture: nothing says which rank is trained", rlNoneSrc, resultIf(ok, "a rank was named without the character's cap")})
	}
	{
		_, err := learn.Find(tsChar("Horde", 60), ds, nil, "Elixir of Imaginary Wonders")
		out = append(out, benchRow{"a recipe the dataset lacks", fmt.Sprint(err), "no recipes row of that name", rlNoneSrc, resultIf(err != nil, "a requirement was invented for a recipe the dataset does not hold")})
	}
	finish(t, "advice-recipe-learn-no-answer", "Offline recipe learning: where the dataset has no answer", out)
}
