package professions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline profession-rank benchmark: the rank a character must train next, where it is trained and what it costs, the
// skill cap each character level allows, the secondary professions' book and quest ranks, and the specialization recipes a plan
// leaves out, beside the dataset's rows, with no network. It reads testdata/places_snapshot (profession_tiers: tier, skill
// needed, character level, cost, trainer, place, how) with testdata/advice_snapshot (profession_notes ranks: skill_cap; recipes).
// The dataset holds a price for the Expert rank of most crafts and for Enchanting's Artisan only, a trainer name for the
// crafts only (none for Mining, Skinning or Herbalism), and no specialization table, so a case whose answer the tables lack
// checks that the advisor says so and states nothing. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	rankSrc     = "dataset profession_tiers (tier, skill_required, min_level, cost_copper, how, trainer_name, place, faction), profession_notes ranks"
	rankCapSrc  = "dataset profession_notes ranks (skill_cap), profession_tiers (min_level)"
	rankSpecSrc = "game specializations (public: Dragonscale Leatherworking, Gnomish and Goblin Engineering); dataset recipes (name, learned_from); the dataset has no specialization table"
)

// rankCase is a character whose rank cap is below what the level allows: the rank after the cap is due now.
type rankCase struct {
	prof, faction, tier string
	skill, max, level   int
}

func rankChar(c rankCase) character.Character {
	lv, rk, mx := c.level, c.skill, c.max
	return character.Character{Name: "Rank", Class: "MAGE", Faction: c.faction, Level: &lv,
		Professions: []character.Skill{{Name: c.prof, Rank: &rk, Max: &mx}}}
}

// rankWhere is the "where you are" text of a profession path, built the way ComputePath does, without planning the bands.
func rankWhere(c rankCase) string {
	ch := rankChar(c)
	ds := placesStore()
	p := &planner{c: ch, ds: ds, notes: LoadNotes(ds), prof: c.prof, skill: c.skill}
	allowed, tier := p.notes.CapAt(c.level)
	var pa Path
	p.where(&pa, ch.Professions[0], allowed, tier)
	return strings.Join(pa.Where, " ")
}

// rankRef is what profession_tiers holds for a rank on the character's side: the needs, the price and who gives it.
type rankRef struct {
	skill, level, cost int64
	how                string
	who, other         []string // "name in place" for the side, and for the other side
	found              bool
}

func readRank(t *testing.T, prof, tier, faction string) rankRef {
	t.Helper()
	rows, err := placesStore().Filter("profession_tiers", store.Range{Column: "profession", Min: prof, Max: prof})
	if err != nil {
		t.Fatal(err)
	}
	var r rankRef
	for _, row := range rows {
		if asText(row["tier"]) != tier {
			continue
		}
		r.found = true
		name := asText(row["trainer_name"])
		if p := asText(row["place"]); name != "" && p != "" {
			name += " in " + p
		}
		if f := asText(row["faction"]); f != "" && f != "Both" && !strings.EqualFold(f, faction) {
			if name != "" {
				r.other = append(r.other, name)
			}
			continue
		}
		r.skill, r.level, r.how = asI64(row["skill_required"]), asI64(row["min_level"]), asText(row["how"])
		if c := asI64(row["cost_copper"]); c > 0 && (r.cost == 0 || c < r.cost) {
			r.cost = c
		}
		if name != "" {
			r.who = append(r.who, name)
		}
	}
	return r
}

func (r rankRef) String() string {
	cost := "no price"
	if r.cost > 0 {
		cost = Money(r.cost)
	}
	who := strings.Join(r.who, "; ")
	if who == "" {
		who = "no one named"
	}
	return fmt.Sprintf("%s: level %d, skill %d, %s, %s", r.how, r.level, r.skill, cost, who)
}

var rankTrainerCases = []rankCase{
	{"Alchemy", "Alliance", "Expert", 140, 150, 20},
	{"Tailoring", "Horde", "Artisan", 215, 225, 40},
	{"Alchemy", "Horde", "Journeyman", 70, 75, 12},
	{"Enchanting", "Horde", "Artisan", 195, 225, 36},
	{"Blacksmithing", "Alliance", "Expert", 140, 150, 22},
	{"Leatherworking", "Horde", "Expert", 145, 150, 30},
	{"Engineering", "Alliance", "Artisan", 190, 225, 38},
}

func TestOfflineBenchmarkRankTrainerA(t *testing.T) { rankTrainers(t, "a", rankTrainerCases[:4]) }
func TestOfflineBenchmarkRankTrainerB(t *testing.T) { rankTrainers(t, "b", rankTrainerCases[4:]) }

// rankTrainers: the rank due now is the one after the cap the character holds, and its line gives the skill and character level
// the dataset's tier row needs, the training price (or says the dataset has none), and a trainer the tier row names for the
// side, never one of the other side's or of another rank.
func rankTrainers(t *testing.T, part string, cases []rankCase) {
	var out []benchRow
	for _, tc := range cases {
		words := rankWhere(tc)
		ref := readRank(t, tc.prof, tc.tier, tc.faction)
		label := fmt.Sprintf("%s %s (%s, skill %d/%d, level %d)", tc.prof, tc.tier, strings.ToLower(tc.faction), tc.skill, tc.max, tc.level)
		gate := firstSentence(words, "Train now: ")
		okGate := strings.HasPrefix(gate, fmt.Sprintf("Train now: %s %s (character level %d, skill %d,", tc.prof, tc.tier, ref.level, ref.skill))
		out = append(out, benchRow{label + ": the rank due, its skill and level", gate, ref.String(), rankSrc, resultIf(okGate, "another rank, skill or character level than the tier row")})
		okPrice := strings.Contains(words, "lists no training price") && ref.cost == 0 && !strings.Contains(words, "Training costs")
		if ref.cost > 0 {
			okPrice = strings.Contains(words, "Training costs "+Money(ref.cost)+".") && !strings.Contains(words, "lists no training price")
		}
		out = append(out, benchRow{label + ": the training price", rankPrice(words), ref.String(), rankSrc, resultIf(okPrice, "a price other than the tier row's, or a price made up")})
		line := firstSentence(words, "Trainer to visit: ")
		okWho := false
		for _, n := range ref.who {
			okWho = okWho || strings.Contains(line, n)
		}
		for _, n := range ref.other {
			okWho = okWho && !strings.Contains(line, n)
		}
		out = append(out, benchRow{label + ": the trainer", line, ref.String(), rankSrc, resultIf(okWho && !strings.Contains(line, "name not in the dataset"), "a trainer the tier row does not name for this rank and side")})
	}
	finishOffline(t, "advice-rank-trainer-"+part, "Offline profession rank: trainer, skill and price ("+part+")", out)
}

// firstSentence is the sentence of words that starts with prefix ("" when there is none), up to the end of the sentence.
func firstSentence(words, prefix string) string {
	i := strings.Index(words, prefix)
	if i < 0 {
		return ""
	}
	rest := words[i:]
	if j := strings.Index(rest, ". "); j >= 0 {
		rest = rest[:j+1]
	}
	return rest
}

func rankPrice(words string) string {
	if s := firstSentence(words, "Training costs "); s != "" {
		return s
	}
	return firstSentence(words, "The game data lists no training price")
}

// ---- the skill cap each character level allows ----

var rankCapCases = []struct {
	prof  string
	level int
}{
	{"Alchemy", 9}, {"Blacksmithing", 10}, {"Tailoring", 19}, {"Enchanting", 20}, {"Engineering", 34}, {"Leatherworking", 35}, {"Cooking", 22}, {"Fishing", 12},
}

func TestOfflineBenchmarkSkillCapByLevelA(t *testing.T) { skillCaps(t, "a", rankCapCases[:4]) }
func TestOfflineBenchmarkSkillCapByLevelB(t *testing.T) { skillCaps(t, "b", rankCapCases[4:]) }

// refCap is the skill cap of the highest rank a level may train, read from the dataset: a rank opens at the character level of its
// tier row (the notes' rank level when the row has none) and caps at the notes' skill_cap; the first rank has no level.
func refCap(t *testing.T, prof string, level int) (tier string, cap int64, next string, nextLevel int64) {
	t.Helper()
	ds := placesStore()
	ranks, _ := ds.Filter("profession_notes", store.Range{Column: "section", Min: "ranks", Max: "ranks"})
	rows, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: prof, Max: prof})
	for _, r := range ranks {
		name := asText(r["subject"])
		need := asI64(r["min_level"])
		for _, row := range rows {
			if asText(row["tier"]) == name && asI64(row["min_level"]) > 0 {
				need = asI64(row["min_level"])
			}
		}
		switch {
		case need <= int64(level) && asI64(r["skill_cap"]) >= cap:
			tier, cap = name, asI64(r["skill_cap"])
		case next == "" || need < nextLevel:
			if need > int64(level) {
				next, nextLevel = name, need
			}
		}
	}
	return tier, cap, next, nextLevel
}

// skillCaps: the most skill a level may train a profession to, and the level the next rank opens at: the planner stops at the cap
// and names the rank, and the character's own skill is read as at the cap so no band is planned.
func skillCaps(t *testing.T, part string, cases []struct {
	prof  string
	level int
}) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range cases {
		tier, cap, next, nextLevel := refCap(t, tc.prof, tc.level)
		c := rankCase{tc.prof, "Horde", "", int(cap), int(cap), tc.level}
		pa := ComputePath(rankChar(c), ds, tc.prof, 300, 1)
		note := ""
		for _, n := range pa.Notes {
			if strings.HasPrefix(n, "Level ") {
				note = n
			}
		}
		ok := int64(pa.Target) == cap && (cap == 300 || strings.Contains(note, fmt.Sprintf("can train %s up to %d", tier, cap)))
		label := fmt.Sprintf("%s at level %d: the skill cap", tc.prof, tc.level)
		out = append(out, benchRow{label, fmt.Sprintf("goal %d; %s", pa.Target, note), fmt.Sprintf("%s, cap %d", tier, cap), rankCapSrc, resultIf(ok, "another cap than the highest rank the level may train")})
		if next != "" {
			words := strings.Join(pa.Where, " ")
			want := fmt.Sprintf("Next tier: %s, cap ", next)
			line := firstSentence(words, "Next tier: ")
			okNext := strings.HasPrefix(line, want) && strings.Contains(line, fmt.Sprintf("level %d", nextLevel))
			out = append(out, benchRow{fmt.Sprintf("%s at level %d: the next rank", tc.prof, tc.level), line, fmt.Sprintf("%s opens at level %d", next, nextLevel), rankCapSrc, resultIf(okNext, "another next rank or level than the dataset's")})
		}
	}
	finishOffline(t, "advice-skill-cap-by-level-"+part, "Offline skill cap by character level ("+part+")", out)
}

// ---- secondary professions: books and quests, not trainers ----

var rankSecondaryCases = []rankCase{
	{"First Aid", "Alliance", "Expert", 140, 150, 25},
	{"Cooking", "Horde", "Expert", 140, 150, 22},
	{"Fishing", "Alliance", "Expert", 140, 150, 22},
	{"First Aid", "Horde", "Artisan", 215, 225, 40},
	{"Cooking", "Alliance", "Artisan", 224, 225, 40},
}

// TestOfflineBenchmarkSecondaryRanks: First Aid, Cooking and Fishing rank after Apprentice is a book or a quest (profession_tiers
// how), so the answer names who gives it and where, never a trainer to visit, and the skill and level it needs.
func TestOfflineBenchmarkSecondaryRanks(t *testing.T) {
	var out []benchRow
	for _, tc := range rankSecondaryCases {
		words := rankWhere(tc)
		ref := readRank(t, tc.prof, tc.tier, tc.faction)
		gate := firstSentence(words, "Train now: ")
		given := firstSentence(words, "It is not taught by a trainer")
		ok := ref.how != "trainer" && len(ref.who) == 1 && strings.Contains(given, ref.who[0]) && !strings.Contains(words, "Trainer to visit") &&
			strings.HasPrefix(gate, fmt.Sprintf("Train now: %s %s (character level %d, skill %d,", tc.prof, tc.tier, ref.level, ref.skill))
		kind := map[string]string{"book": "a book", "quest": "a quest"}[ref.how]
		ok = ok && strings.Contains(given, map[string]string{"book": "it is a book", "quest": "it comes from a quest"}[ref.how])
		out = append(out, benchRow{fmt.Sprintf("%s %s (%s, skill %d/%d, level %d): who gives it", tc.prof, tc.tier, strings.ToLower(tc.faction), tc.skill, tc.max, tc.level), gate + " " + given, kind + " from " + ref.String(), rankSrc,
			resultIf(ok, "a trainer is named for a book or quest rank, or the giver, skill or level differs")})
	}
	finishOffline(t, "advice-secondary-ranks", "Offline Cooking, First Aid and Fishing: the book or quest rank", out)
}

// TestOfflineBenchmarkRouteRankGate: the skill-up route says the same gate, price and giver as the path, for a trainer rank and a book.
func TestOfflineBenchmarkRouteRankGate(t *testing.T) {
	ds := placesStore()
	var out []benchRow
	for _, tc := range []rankCase{{"Alchemy", "Alliance", "Expert", 140, 150, 20}, {"First Aid", "Horde", "Expert", 140, 150, 25}, {"Tailoring", "Horde", "Artisan", 215, 225, 40}} {
		got := ""
		for _, l := range strings.Split(RouteFacts(rankChar(tc), ds, nil, tc.prof), "\n") {
			if strings.HasPrefix(l, "Before ") {
				got = l
			}
		}
		ref := readRank(t, tc.prof, tc.tier, tc.faction)
		want := fmt.Sprintf("you must %s %s %s (character level %d, skill %d;", map[bool]string{true: "train", false: "get"}[ref.how == "trainer"], tc.prof, tc.tier, ref.level, ref.skill)
		ok := strings.Contains(got, want) && len(ref.who) > 0 && strings.Contains(got, ref.who[0])
		if ref.cost > 0 {
			ok = ok && strings.Contains(got, "Training costs "+Money(ref.cost)+".")
		} else {
			ok = ok && strings.Contains(got, "lists no training price")
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s (%s, skill %d/%d, level %d): the route's gate", tc.prof, tc.tier, strings.ToLower(tc.faction), tc.skill, tc.max, tc.level), got, ref.String(), rankSrc,
			resultIf(ok, "the route states another skill, level, price or giver than the tier row")})
	}
	finishOffline(t, "advice-route-rank-gate", "Offline skill-up route: the rank gate", out)
}

// ---- specialization ----

var rankSpecCases = []struct {
	label, prof, recipe string
	known, left         bool
}{
	{"Engineering, Gnomish Cloaking Device, not known", "Engineering", "Gnomish Cloaking Device", false, true},
	{"Engineering, Gnomish Cloaking Device, known", "Engineering", "Gnomish Cloaking Device", true, false},
	{"Leatherworking, Blue Dragonscale Leggings, not known", "Leatherworking", "Blue Dragonscale Leggings", false, true},
	{"Leatherworking, Dragonscale Gauntlets, known", "Leatherworking", "Dragonscale Gauntlets", true, false},
	{"Blacksmithing, Elemental Sharpening Stone (any smith)", "Blacksmithing", "Elemental Sharpening Stone", false, false},
}

// TestOfflineBenchmarkRankSpecialization: real dataset recipe rows named for a specialization are left out of a plan unless the
// character knows them, and a recipe whose name carries another profession's specialization word is planned.
func TestOfflineBenchmarkRankSpecialization(t *testing.T) {
	ds := store.NewDir("../../testdata/tradeskills_snapshot")
	var out []benchRow
	for _, tc := range rankSpecCases {
		rows, err := ds.Filter("recipes", store.Range{Column: "name", Min: tc.recipe, Max: tc.recipe})
		if err != nil || len(rows) == 0 {
			t.Fatalf("%s: %v", tc.recipe, err)
		}
		rc := recipeOf(rows[0])
		p := &planner{prof: tc.prof, ds: ds, known: map[int64]bool{}, names: map[int64]string{}}
		if tc.known {
			p.known[rc.SpellID] = true
		}
		got := p.blockedRecipe(rc)
		word := map[bool]string{true: "left out", false: "planned"}
		ref := fmt.Sprintf("%s; row: %s %d, learned from %s", word[tc.left], asText(rows[0]["skill_line_name"]), asI64(rows[0]["learn_skill"]), rc.LearnedFrom)
		out = append(out, benchRow{tc.label, word[got], ref, rankSpecSrc, resultIf(got == tc.left, "left out when it should be planned, or planned when it should be left out")})
	}
	finishOffline(t, "advice-rank-specialization", "Offline specialization: real recipe rows", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkRankNoAnswer: what the tier table lacks is said, never guessed: a rank with no price, a rank with no named
// trainer, a profession with no tier row at all, and the choice of a specialization.
func TestOfflineBenchmarkRankNoAnswer(t *testing.T) {
	var out []benchRow
	priceless := rankCase{"Alchemy", "Horde", "Journeyman", 70, 75, 12}
	words := rankWhere(priceless)
	out = append(out, benchRow{"Alchemy Journeyman: what it costs", rankPrice(words), "cost_copper is empty in every tier row", rankSrc,
		resultIf(readRank(t, "Alchemy", "Journeyman", "Horde").cost == 0 && strings.Contains(words, "lists no training price") && !strings.Contains(words, "Training costs"), "a price was given")})
	for _, tc := range []rankCase{{"Mining", "Horde", "Expert", 120, 150, 25}, {"Herbalism", "Alliance", "Artisan", 190, 225, 40}} {
		words := rankWhere(tc)
		ref := readRank(t, tc.prof, tc.tier, tc.faction)
		line := firstSentence(words, "Trainer to visit: ")
		okNone := len(ref.who) == 0 && ref.cost == 0 && strings.Contains(line, "name not in the dataset") && strings.Contains(words, "lists no training price")
		out = append(out, benchRow{fmt.Sprintf("%s %s: the trainer and the price", tc.prof, tc.tier), line + " " + rankPrice(words), "the tier row names no trainer and no price", rankSrc, resultIf(okNone, "a trainer name or a price was made up")})
	}
	jewel := rankCase{"Jewelcrafting", "Horde", "Journeyman", 70, 75, 15}
	words = rankWhere(jewel)
	gate := firstSentence(words, "Train now: ")
	out = append(out, benchRow{"Jewelcrafting (no tier rows): the skill gate", gate, "no profession_tiers row for Jewelcrafting", rankSrc,
		resultIf(!readRank(t, "Jewelcrafting", "Journeyman", "Horde").found && strings.Contains(gate, "holds no skill gate") && !strings.Contains(gate, "skill 75") && strings.Contains(words, "lists no training price"), "a skill gate or price was given")})
	spec := rankCase{"Engineering", "Horde", "Artisan", 205, 225, 40}
	words = rankWhere(spec)
	named := ""
	for _, w := range []string{"gnomish", "goblin", "specializ", "specialis"} {
		if strings.Contains(strings.ToLower(words), w) {
			named = w
		}
	}
	out = append(out, benchRow{"Engineering at level 40: which specialization", words, "no specialization data: none is named", rankSpecSrc, resultIf(named == "", "a specialization was named: "+named)})
	finishOffline(t, "advice-rank-no-answer", "Offline profession ranks: no answer in the dataset", out)
}
