package paths_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline secondary-skill benchmark: the advice for Fishing, Cooking, First Aid and Lockpicking beside the dataset's rows and the game's own tables, with no network. It reads
// testdata/tradeskills_snapshot (recipes, items, profession_routes), testdata/consumables_snapshot (bandage, anti-venom and lockbox items), testdata/companions_snapshot
// (fishing_catches) and testdata/usability_snapshot (class_spells of the rogue). It adds to offline_gathersecondary_benchmark_test.go (the skill a Cooking or First Aid recipe needs, a zone's catches)
// and the profession ones (routes, ranks, caps): other recipes, other questions, the bandages in the bag review, how a leveling question names a secondary skill, and the questions the
// dataset cannot answer. The dataset holds no heal amount, no channel time, no lock skill, no Pick Lock spell row, no cooking fire and no food effect, so those cases check that the
// answer is framed with the line that says so (ask.LimitFacts) and that nothing is stated from memory. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	ssRecipeSrc = "dataset recipes (skill_line_name, learn_skill, learn_skill_source, learned_from); game: Warcraft Wiki Cooking and First Aid recipe tables"
	ssBagSrc    = "dataset items (class_id 0 subclass_id 7 bandage, 8 other; binding, sell_price); game: any character can use a bandage or an anti-venom (Warcraft Wiki, First Aid)"
	ssLockSrc   = "dataset items (Mysterious Lockbox: class 0 subclass 8, binding 1, no sell price); class_spells (class_id 4: no Pick Lock row); game: a rogue's Pick Lock opens a lockbox (Warcraft Wiki, Lockpicking)"
	ssToolSrc   = "dataset items (Fishing Pole items: required_level, binding, item_level); no fishing skill requirement column"
	ssAskSrc    = "advisor question reading (ask.ProfessionAsked) with the dataset's profession_routes for the profession; game: First Aid, Cooking and Fishing are secondary skills a player names in several ways"
	ssNoneSrc   = "the dataset has no such row, column or table (items: no heal, lock or use-effect column; class_spells: no Pick Lock; no lock table)"
	ssCatchSrc  = "dataset fishing_catches (zone_name, item_id, casts, rate), items (name)"
)

// ssRecipeCase is a recipe, the question that asks for its skill, and the skill the game gives.
type ssRecipeCase struct {
	recipe, prof, question string
	game                   int
}

var ssCooking = []ssRecipeCase{
	{"Roasted Boar Meat", "Cooking", "can I make %s yet?", 1}, {"Charred Wolf Meat", "Cooking", "when do I learn %s?", 1},
	{"Boiled Clams", "Cooking", "how high must my skill be to make %s?", 50}, {"Coyote Steak", "Cooking", "what skill do I need to cook %s?", 50},
	{"Crocolisk Steak", "Cooking", "when can I cook %s?", 80}, {"Smoked Sagefish", "Cooking", "what skill is required for %s?", 80},
	{"Clam Chowder", "Cooking", "can I make %s yet?", 90}, {"Redridge Goulash", "Cooking", "how high must my skill be to cook %s?", 100},
	{"Big Bear Steak", "Cooking", "what skill do I need to make %s?", 110}, {"Crocolisk Gumbo", "Cooking", "when do I learn %s?", 120},
	{"Hot Lion Chops", "Cooking", "can I cook %s yet?", 125}, {"Spider Sausage", "Cooking", "what skill does %s need?", 200},
	{"Monster Omelet", "Cooking", "what skill is required to cook %s?", 225}, {"Nightfin Soup", "Cooking", "can I make %s yet?", 250},
	{"Mightfish Steak", "Cooking", "how high must my skill be for %s?", 275},
}

var ssFirstAid = []ssRecipeCase{
	{"Linen Bandage", "First Aid", "what skill does %s need?", 1}, {"Heavy Silk Bandage", "First Aid", "can I make %s yet?", 180},
	{"Mageweave Bandage", "First Aid", "when can I make %s?", 210}, {"Anti-Venom", "First Aid", "what skill do I need to make %s?", 80},
	{"Strong Anti-Venom", "First Aid", "how high must my skill be to make %s?", 130}, {"Powerful Anti-Venom", "First Aid", "what skill is required for %s?", 300},
}

// ssRecipeRow is the dataset row of a recipe of a profession.
func ssRecipeRow(t *testing.T, name, prof string) store.Row {
	t.Helper()
	for _, r := range tsRows(t, "recipes", "name", name) {
		if text(r["skill_line_name"]) == prof {
			return r
		}
	}
	t.Fatalf("recipes: no %s recipe named %q", prof, name)
	return nil
}

func ssRecipes(t *testing.T, id, title string, cases []ssRecipeCase) {
	t.Helper()
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range cases {
		q := fmt.Sprintf(tc.question, tc.recipe)
		facts := ask.RecipeSkillFacts(ds, q)
		line := firstLineMatching(facts, tc.recipe+" ("+tc.prof+")")
		rec := ssRecipeRow(t, tc.recipe, tc.prof)
		need := num(rec["learn_skill"])
		src := text(rec["learn_skill_source"])
		about := map[bool]string{true: "about ", false: ""}[src == "estimate"]
		ref := fmt.Sprintf("dataset %d (%s); game %d", need, src, tc.game)
		var ok bool
		res := ""
		switch {
		case need <= 1:
			// a client minimum of 1 is no gate: the answer states no number
			ok = strings.Contains(line, "The trainer will show whether you can learn it yet.") && !strings.Contains(line, "It needs")
			ref += "; a minimum of 1 states no number"
		default:
			ok = strings.Contains(line, fmt.Sprintf("It needs %s%s %d.", about, tc.prof, need)) && strings.Count(facts, "\n- ") == 1
		}
		res = resultIf(ok, "no line for the recipe, or the skill or its estimate wording")
		if res == "match" && need > 1 && need != int64(tc.game) {
			res = fmt.Sprintf("differ (data): the dataset's own estimate is %d, the game %d", need, tc.game)
		}
		out = append(out, benchRow{q, line, ref, ssRecipeSrc, res})
	}
	finish(t, id, title, out)
}

func TestOfflineBenchmarkSecondaryRecipesCookingA(t *testing.T) {
	ssRecipes(t, "advice-secondary-recipes-cooking-a", "Offline Cooking: the skill a recipe needs (a)", ssCooking[:5])
}

func TestOfflineBenchmarkSecondaryRecipesCookingB(t *testing.T) {
	ssRecipes(t, "advice-secondary-recipes-cooking-b", "Offline Cooking: the skill a recipe needs (b)", ssCooking[5:10])
}

func TestOfflineBenchmarkSecondaryRecipesCookingC(t *testing.T) {
	ssRecipes(t, "advice-secondary-recipes-cooking-c", "Offline Cooking: the skill a recipe needs (c)", ssCooking[10:])
}

func TestOfflineBenchmarkSecondaryRecipesFirstAid(t *testing.T) {
	ssRecipes(t, "advice-secondary-recipes-first-aid", "Offline First Aid: the skill a bandage or anti-venom needs", ssFirstAid)
}

// ---- the bag review: bandages and anti-venoms ----

// ssChar is a character of the class holding the named items (from the consumables snapshot), with the professions named.
func ssChar(t *testing.T, class, name string, count int, profs []string, items ...string) character.Character {
	t.Helper()
	c := usabChar(class, 40, "Horde")
	c.Name, c.BagsCaptured, c.BankCaptured, c.Equipment = name, true, true, map[string]character.EquippedItem{}
	rank, top := 100, 150
	for _, p := range profs {
		c.Professions = append(c.Professions, character.Skill{Name: p, Rank: &rank, Max: &top})
	}
	for _, it := range items {
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(num(rowByName(t, consStore(), "items", it)["id"])), Count: count, Name: it})
	}
	return c
}

func ssReview(c character.Character) (inventory.Group, string) {
	var g inventory.Group
	var why string
	for gr, es := range inventory.Review(c, consStore()).Groups {
		for _, e := range es {
			g, why = gr, e.Reason
		}
	}
	return g, why
}

var ssBandages = []struct{ item, class string }{
	{"Linen Bandage", "MAGE"}, {"Heavy Silk Bandage", "WARRIOR"}, {"Heavy Runecloth Bandage", "PRIEST"}, {"Crystal Infused Bandage", "ROGUE"},
	{"Warsong Gulch Silk Bandage", "PALADIN"}, {"Anti-Venom", "HUNTER"}, {"Strong Anti-Venom", "DRUID"}, {"Powerful Anti-Venom", "WARLOCK"},
}

// TestOfflineBenchmarkBandagesKept: a bandage or an anti-venom in the bags is kept, whatever the character's class and whether or not they have First Aid: the bag review before the
// fix left every one of them "no rule places it".
func TestOfflineBenchmarkBandagesKept(t *testing.T) {
	var out []benchRow
	for i, b := range ssBandages {
		profs := []string{}
		if i%2 == 1 {
			profs = []string{"First Aid"}
		}
		row := rowByName(t, consStore(), "items", b.item)
		g, why := ssReview(ssChar(t, b.class, "Bandager", 20, profs, b.item))
		out = append(out, benchRow{fmt.Sprintf("%s in a %s's bags (First Aid: %v): the bag review", b.item, strings.ToLower(b.class), len(profs) > 0), fmt.Sprintf("%s: %s", g, why),
			fmt.Sprintf("item class %d subclass %d, bonding %d; game: anyone can use it: kept", num(row["class_id"]), num(row["subclass_id"]), num(row["bonding"])), ssBagSrc,
			resultIf(g == inventory.Keep && strings.Contains(why, "First Aid supplies"), "a bandage was not kept")})
	}
	finish(t, "advice-bandages-kept", "Offline First Aid: bandages and anti-venoms are kept", out)
}

// TestOfflineBenchmarkBandagesAlts: the review across characters keeps a bandage with its holder and sells or mails nothing for want of First Aid.
func TestOfflineBenchmarkBandagesAlts(t *testing.T) {
	var out []benchRow
	for _, item := range []string{"Heavy Linen Bandage", "Wool Bandage", "Mageweave Bandage", "Anti-Venom"} {
		res := inventory.ReviewAll([]character.Character{ssChar(t, "MAGE", "Mage", 20, nil, item), ssChar(t, "WARRIOR", "War", 1, nil)}, consStore())
		l, _ := kaLine(res, "Mage", item)
		out = append(out, benchRow{item + " in a mage's bags, no character has First Aid: the review across both", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "keep: any character uses it; not sold for want of First Aid", ssBagSrc,
			resultIf(l.Action == inventory.Keep_ && !l.AH && l.To == "", "a bandage was sold or mailed")})
	}
	row := rowByName(t, consStore(), "items", "Dense Runecloth Bandage")
	g, why := ssReview(ssChar(t, "MAGE", "Mage", 20, nil, "Dense Runecloth Bandage"))
	out = append(out, benchRow{"Dense Runecloth Bandage in a mage's bags: the bag review", fmt.Sprintf("%s: %s", g, why), fmt.Sprintf("item class %d subclass %d (the dataset files it as other, not as a bandage); game: a bandage", num(row["class_id"]), num(row["subclass_id"])), ssBagSrc,
		map[bool]string{true: "differ (data): the dataset files it as class 0 subclass 8, not 7; the advisor keeps it by its name", false: "differ: not kept"}[g == inventory.Keep]})
	finish(t, "advice-bandages-alts", "Offline First Aid: bandages across characters", out)
}

// ---- lockboxes ----

// TestOfflineBenchmarkLockboxes: a rogue keeps a lockbox, an alt's lockbox goes to the rogue by mail, and a lockbox bound to its holder or with no rogue is left unplaced.
func TestOfflineBenchmarkLockboxes(t *testing.T) {
	const box = "Mysterious Lockbox"
	row := rowByName(t, consStore(), "items", box)
	ref := fmt.Sprintf("item class %d subclass %d, bonding %d, no sell price", num(row["class_id"]), num(row["subclass_id"]), num(row["bonding"]))
	var out []benchRow
	g, why := ssReview(ssChar(t, "ROGUE", "Rogue", 3, nil, box))
	out = append(out, benchRow{box + " in a rogue's bags: the bag review", fmt.Sprintf("%s: %s", g, why), ref + "; game: a rogue picks it open: kept", ssLockSrc, resultIf(g == inventory.Keep && strings.Contains(why, "a lockbox"), "a rogue's lockbox was not kept")})
	for _, class := range []string{"WARRIOR", "MAGE"} {
		g, why := ssReview(ssChar(t, class, "Other", 3, nil, box))
		out = append(out, benchRow{box + " in a " + strings.ToLower(class) + "'s bags: the bag review", fmt.Sprintf("%s: %s", g, why), ref + "; no rule: only a rogue picks a lock", ssLockSrc, resultIf(g == inventory.Unknown && why == "no rule places it", "a non-rogue kept a lockbox")})
	}
	res := inventory.ReviewAll([]character.Character{ssChar(t, "ROGUE", "Rogue", 3, nil, box), ssChar(t, "MAGE", "Mage", 1, nil)}, consStore())
	l, _ := kaLine(res, "Rogue", box)
	out = append(out, benchRow{box + " in a rogue's bags, a mage alt: the review across both", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "keep: the rogue picks it", ssLockSrc, resultIf(l.Action == inventory.Keep_ && l.To == "", "the rogue's lockbox was sent away")})
	res = inventory.ReviewAll([]character.Character{ssChar(t, "WARRIOR", "War", 3, nil, box), ssChar(t, "ROGUE", "Rogue", 1, nil)}, consStore())
	l, _ = kaLine(res, "War", box)
	out = append(out, benchRow{box + " in a warrior's bags, a rogue alt: the review across both", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "bonding 1: bound to its holder, so it cannot be mailed; left unplaced", ssLockSrc, resultIf(l.Action == inventory.Unsure && l.To == "", "a bound lockbox was mailed or sold")})
	finish(t, "advice-lockboxes", "Offline lockpicking: a rogue keeps a lockbox", out)
}

// TestOfflineBenchmarkLockpickingNoAnswer: the dataset holds no lockpicking skill, no Pick Lock row and no lock table; the answer says so and names no skill, level or contents.
func TestOfflineBenchmarkLockpickingNoAnswer(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what lockpicking skill do I need for a Large Iron Lockbox?",
		"when does a rogue learn Pick Lock?",
		"how do I level lockpicking as a rogue?",
		"what is in a Battered Junkbox?",
		"how do I raise my lockpicking skill faster?",
	} {
		facts := ask.LimitFacts(q)
		ok := strings.Contains(facts, "Lockpicking:") && strings.Contains(facts, "no lockpicking skill") && strings.Contains(facts, "Say so; name no lockpicking skill number")
		out = append(out, benchRow{"\"" + q + "\"", lastLine(facts), "states none: no lockpicking, Pick Lock or lock row", ssNoneSrc, resultIf(ok, "no line that the data has none")})
	}
	spells, _ := usabStore().Filter("class_spells", store.Range{Column: "class_id", Min: int64(4), Max: int64(4)})
	var pick []string
	var rogue []string
	for _, r := range spells {
		if n := text(r["name"]); strings.Contains(strings.ToLower(n), "pick lock") {
			pick = append(pick, n)
		} else if n == "Pick Pocket" || n == "Distract" || n == "Disarm Trap" {
			rogue = append(rogue, fmt.Sprintf("%s at level %d", n, num(r["level"])))
		}
	}
	out = append(out, benchRow{"class_spells: a Pick Lock row for the rogue", fmt.Sprintf("%d rows; the rogue's neighbours: %s", len(pick), strings.Join(rogue, ", ")), "game: a rogue learns Pick Lock at level 16", ssLockSrc,
		"differ (data): the dataset has no Pick Lock row, so the advisor cannot say when a rogue learns it"})
	cols := brColumns(consStore(), "items", "id")
	bad := brHasColumn(cols, "lock", "skill_req", "required_skill")
	out = append(out, benchRow{"items: a column for the lock skill an item needs", fmt.Sprintf("%d columns: none names a lock or skill requirement", len(cols)), "no such column", ssNoneSrc, resultIf(bad == "", "a column names "+bad)})
	finish(t, "advice-lockpicking-no-answer", "Offline lockpicking: no answer in the dataset", out)
}

// ---- skill requirements of fishing gear ----

// TestOfflineBenchmarkFishingPoleRequirements: the required level a fishing pole's tooltip gives is the dataset's, and the question does not invent a Fishing skill for it.
func TestOfflineBenchmarkFishingPoleRequirements(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, name := range []string{"Fishing Pole", "Strong Fishing Pole", "Big Iron Fishing Pole", "Arcanite Fishing Pole", "Dwarven Fishing Pole"} {
		row := rowByName(t, ds, "items", name)
		facts := ask.ItemTooltipFacts(ds, "what level does ["+name+"] require?")
		line := firstLineMatching(facts, "- "+name+":")
		if line == "" {
			line = firstLineMatching(facts, name+":")
		}
		rl := num(row["required_level"])
		ok := line != ""
		if rl > 0 {
			ok = ok && strings.Contains(line, fmt.Sprintf("requires level %d", rl))
		}
		ok = ok && !regexp.MustCompile(`(?i)fishing (skill )?\d`).MatchString(line)
		out = append(out, benchRow{"[" + name + "]: the level it requires", line, fmt.Sprintf("dataset required_level %d, item_level %d, bonding %d", rl, num(row["item_level"]), num(row["bonding"])), ssToolSrc, resultIf(ok, "the tooltip's level, or a Fishing skill that is not in the data")})
	}
	finish(t, "advice-fishing-pole-requirements", "Offline fishing: the level a pole requires", out)
}

// TestOfflineBenchmarkFishingCatchesMore: zones the first catch benchmark does not use, and a zone the reports do not cover.
func TestOfflineBenchmarkFishingCatchesMore(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, z := range []string{"Silverpine Forest", "Wetlands", "Stonetalon Mountains"} {
		facts := ask.GatherFacts(ds, "which fish can I catch in "+z+"?")
		line := firstLineMatching(facts, "Catches reported in "+z)
		rows := compRows(t, "fishing_catches", "zone_id", num(rowByCol(t, "fishing_catches", "zone_name", z)["zone_id"]))
		best := 0.0
		bestName := ""
		for _, r := range rows {
			if x, _ := r["rate"].(float64); x > best {
				it, _ := ds.Get("items", num(r["item_id"]))
				best, bestName = x, text(it["name"])
			}
		}
		out = append(out, benchRow{z + ": what to catch", line, fmt.Sprintf("best rate %.0f%%: %s (%d rows)", 100*best, bestName, len(rows)), ssCatchSrc,
			resultIf(strings.Contains(line, bestName+fmt.Sprintf(" (%.0f%%", 100*best)), "the best catch is not first or not named")})
	}
	facts := ask.GatherFacts(ds, "which fish can I catch in Westfall?")
	out = append(out, benchRow{"Westfall: what to catch", lastLine(facts), "no fishing report for Westfall: say there is none", ssCatchSrc,
		resultIf(strings.Contains(facts, "cover only these zones") && !strings.Contains(facts, "Catches reported in Westfall"), "a catch was named for a zone with no report")})
	finish(t, "advice-fishing-catches-more", "Offline fishing: more zones, and a zone with no report", out)
}

// ---- questions the dataset cannot answer ----

// TestOfflineBenchmarkSecondaryNoAnswer: First Aid bandage effects, cooking fires, food effects and fishing lures, bait and bite times: the answer says the data holds none.
func TestOfflineBenchmarkSecondaryNoAnswer(t *testing.T) {
	cases := []struct{ q, facts, must, ref string }{
		{"how much does a Heavy Silk Bandage heal?", ask.LimitFacts("how much does a Heavy Silk Bandage heal?"), "First Aid bandages:", "no heal amount column or spell effect"},
		{"how long does bandaging take, and what is Recently Bandaged?", ask.LimitFacts("how long does bandaging take, and what is Recently Bandaged?"), "no Recently Bandaged rule", "no channel time or debuff row"},
		{"what does Strong Anti-Venom cure?", ask.LimitFacts("what does Strong Anti-Venom cure?"), "no Anti-Venom", "no cure or effect row"},
		{"can I use a bandage in combat?", ask.LimitFacts("can I use a bandage in combat?"), "no rule on bandaging in combat", "no combat rule"},
		{"do I need a cooking fire to cook?", ask.LimitFacts("do I need a cooking fire to cook?"), "Cooking fires and food effects:", "no campfire row"},
		{"what does Well Fed give?", ask.LimitFacts("what does Well Fed give?"), "what Well Fed gives", "no buff row"},
		{"how much health does Smoked Bear Meat restore?", ask.LimitFacts("how much health does Smoked Bear Meat restore?"), "no restore amount", "no food effect column"},
		{"what bait or lure should I use to fish in the Barrens?", ask.GatherFacts(compStore(), "what bait or lure should I use to fish in the Barrens?"), "no lure or bait effect", "no lure row"},
		{"how long until a fish bites, and how many skill points per catch?", ask.GatherFacts(compStore(), "how long until a fish bites, and how many skill points per catch?"), "no bite time and no skill gained per cast or catch", "no bite time or skill gain"},
	}
	var out []benchRow
	for _, c := range cases {
		never := regexp.MustCompile(`(?i)\d+ ?(hp|health|seconds|sec)\b|\+\d+ fishing`)
		ok := strings.Contains(c.facts, c.must) && !never.MatchString(c.facts)
		out = append(out, benchRow{"\"" + c.q + "\"", firstLineMatching(c.facts, c.must), "states none: " + c.ref, ssNoneSrc, resultIf(ok, "no line that the data has none, or a figure was stated")})
	}
	finish(t, "advice-secondary-no-answer", "Offline First Aid, Cooking and Fishing: no answer in the dataset", out)
}

// TestOfflineBenchmarkSecondaryQuiet: a question that asks for what the data holds (a bandage's skill or price, the order to make them) gets none of the no-answer lines.
func TestOfflineBenchmarkSecondaryQuiet(t *testing.T) {
	for _, q := range []string{
		"which bandage should I make to level First Aid?", "how much does a Silk Bandage cost?", "what skill do I need to make Heavy Silk Bandage?", "where do I learn Expert First Aid?",
		"which food should I cook to reach 150 Cooking?", "where do I learn Expert Cooking?", "how do I level Fishing from 1 to 75?", "where is the Cooking trainer?",
	} {
		if got := ask.LimitFacts(q); strings.Contains(got, "First Aid bandages:") || strings.Contains(got, "Lockpicking:") || strings.Contains(got, "Cooking fires and food effects:") {
			t.Errorf("%q: facts %q", q, got)
		}
	}
}

// ---- how a leveling question names the skill ----

var ssAskCases = []struct{ q, prof string }{
	{"how do I level first-aid?", "First Aid"},
	{"How do I level up my bandaging?", "First Aid"},
	{"get my First Aid to 150", "First Aid"},
	{"how do I level FirstAid?", "First Aid"},
	{"level my fishing!", "Fishing"},
	{"how do I raise my angling skill?", "Fishing"},
	{"how can I get cooking to 150?", "Cooking"},
	{"how do I level up my cooking: what recipes?", "Cooking"},
	{"what should I cook to level up?", "Cooking"},
	{"how do I level my Cooking's skill?", "Cooking"},
}

// TestOfflineBenchmarkSecondaryLevelingQuestions: a leveling question that names Fishing, Cooking or First Aid by another spelling, with punctuation after it or by the bandaging the skill is
// used for gets that profession, and with it the route's facts, as the plain "how do I level first aid?" does.
func TestOfflineBenchmarkSecondaryLevelingQuestions(t *testing.T) {
	ds := tsStore()
	c := tsChar("Horde", 40, tsSkill{"First Aid", 60, 75}, tsSkill{"Fishing", 60, 75}, tsSkill{"Cooking", 60, 75})
	var out []benchRow
	for _, tc := range ssAskCases {
		got := ask.ProfessionAsked(c, tc.q)
		route := professions.RouteFacts(c, ds, nil, tc.prof)
		ok := got == tc.prof && strings.Contains(route, "Now: ")
		out = append(out, benchRow{"\"" + tc.q + "\"", fmt.Sprintf("profession %q, route facts: %v", got, route != ""), "profession " + tc.prof + "; profession_routes has a step for skill 60", ssAskSrc, resultIf(ok, "another profession, or none, or no route")})
	}
	// questions that are not about raising one of the character's skills name none
	for _, q := range []string{"what should I do next?", "where can I get fish for cooking?", "how do I level mining?"} {
		got := ask.ProfessionAsked(c, q)
		out = append(out, benchRow{"\"" + q + "\"", fmt.Sprintf("profession %q", got), "none: not a question on raising First Aid, Fishing or Cooking", ssAskSrc, resultIf(got == "", "a profession was named")})
	}
	finish(t, "advice-secondary-leveling-questions", "Offline First Aid, Cooking and Fishing: naming the skill in a leveling question", out)
}
