package paths_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline reputation-vendor, class-quest, secondary-profession trainer and recipe-source benchmark: what the
// advisor answers about an item a vendor sells at a reputation discount, about a class quest and its place in a
// quest line, about the trainer and rank gate of Cooking, First Aid and Fishing, and about where a Cooking or First
// Aid recipe is learned, beside the dataset's rows, with no network. It reads testdata/places_snapshot
// (vendor_sources, quests_observed, quest_lines, trainers, profession_tiers), testdata/advice_snapshot (items, recipes)
// and testdata/consumables_snapshot (the Cooking recipes). The dataset holds no faction, standing or discount on a
// vendor row, no class on a quest, and no class quest line, so the cases it cannot answer check that the advisor says
// so and states none. A difference the dataset's own rows cause is marked "(data)". make offline-benchmark runs it;
// WOWC_BENCH_REPORT receives the tables.

const (
	repVendorSrc = "dataset vendor_sources (price_copper, limited, reporters), items (buy_price)"
	repNoneSrc   = "dataset vendor_sources has no faction, standing or discount column"
	cqSrc        = "dataset quests_observed (title, level, faction, category)"
	cqLineSrc    = "dataset quest_lines (no class line), quests_observed"
	secTrainSrc  = "dataset trainers (profession, zone_name) and profession_tiers (tier, skill_required, min_level)"
	secLearnSrc  = "dataset recipes (learn_skill, learned_from), recipe_items, vendor_sources, loot_sources"
)

// repItems are items real vendors sell: the dataset has the item's base price and the lower price some reporters
// paid at a reputation discount, and for two a limited stock.
var repItems = []string{
	"Rune Thread", "Crystal Vial", "Heavy Silken Thread", "Strong Flux", "Orange Dye", "Black Dye", "Ringed Buckler", "Scalemail Vest",
	"Lesser Magic Essence", "Pattern: Deviate Scale Cloak",
}

func TestOfflineBenchmarkReputationVendorA(t *testing.T) { reputationPrice(t, "a", repItems[:5]) }
func TestOfflineBenchmarkReputationVendorB(t *testing.T) { reputationPrice(t, "b", repItems[5:]) }

// reputationPrice: the offer the advisor gives is priced at the item's base price, what a player with no standing pays,
// never the lower price a reporter with a discount saw; a limited-stock vendor says so.
func reputationPrice(t *testing.T, part string, names []string) {
	ds := mbNamed()
	var out []benchRow
	for _, name := range names {
		it := rowByName(t, ds, "items", name)
		id := num(it["id"])
		rows := mbRows(t, ds, "vendor_sources", "item_id", id)
		lowest := int64(1 << 40)
		for _, r := range rows {
			lowest = min(lowest, num(r["price_copper"]))
		}
		best, found := paths.For(horde(40), ds, id, paths.Options{}).Best()
		got, ok := "no vendor offer", false
		if found && best.Offer != nil {
			o := best.Offer
			got = best.Line
			var limited bool
			for _, r := range rows {
				if num(r["npc_id"]) == o.NPCID {
					limited, _ = r["limited"].(bool)
				}
			}
			ok = best.Kind == paths.Vendor && o.PriceCopper == num(it["buy_price"]) && strings.Contains(best.Line, " for "+vendors.Money(num(it["buy_price"]))) && o.Limited == limited &&
				strings.Contains(best.Line, "(limited stock)") == limited
		}
		ref := fmt.Sprintf("buy price %s; %d vendor rows, lowest reported %s", vendors.Money(num(it["buy_price"])), len(rows), vendors.Money(lowest))
		out = append(out, benchRow{name + ": vendor price", got, ref, repVendorSrc, resultIf(ok, "a discounted price, a missing offer or the limited-stock mark")})
	}
	finish(t, "advice-reputation-vendor-"+part, "Offline reputation vendor: the price a player with no standing pays ("+part+")", out)
}

var standingWords = regexp.MustCompile(`(?i)\b(reputation|standing|friendly|honou?red|revered|exalted|neutral|discount|faction)\b`)

// TestOfflineBenchmarkReputationVendorNoAnswer: the dataset names no faction, standing or discount for a vendor. The
// advisor states none, names no vendor by its number, and sends no one to a vendor it cannot name.
func TestOfflineBenchmarkReputationVendorNoAnswer(t *testing.T) {
	ds := mbNamed()
	var out []benchRow
	for _, name := range []string{"Rune Thread", "Ringed Buckler", "Pattern: Deviate Scale Cloak", "Lesser Magic Essence"} {
		id := num(rowByName(t, ds, "items", name)["id"])
		txt := paths.For(horde(40), ds, id, paths.Options{}).Text()
		out = append(out, benchRow{name + ": faction and standing the vendor needs", firstLine(strings.SplitN(txt, "\n", 3)[1]), "no faction or standing column on vendor_sources", repNoneSrc,
			resultIf(!standingWords.MatchString(txt), "a faction, standing or discount was stated")})
	}
	// the snapshot names no vendor of the item: the advisor names none, and never "vendor 1347"
	raw := mbRaw()
	for _, name := range []string{"Rune Thread", "Crystal Vial"} {
		id := num(rowByName(t, raw, "items", name)["id"])
		res := paths.For(horde(40), raw, id, paths.Options{})
		txt := res.Text() + res.Reason
		out = append(out, benchRow{name + ": vendor the dataset does not name", firstLine(strings.TrimSpace(strings.TrimPrefix(txt, "How to get "+name+", best way first:"))), "vendor_sources names an npc id; npcs has no name", repNoneSrc,
			resultIf(!regexp.MustCompile(`(?i)vendor \d+|npc \d+`).MatchString(txt) && !strings.Contains(txt, "Buy from"), "a vendor was named by its number or sent to")})
	}
	finish(t, "advice-reputation-vendor-no-answer", "Offline reputation vendor: no answer in the dataset", out)
}

// classQuests are class quests of the dataset: Warlock, Paladin and Hunter headings of quests_observed.
var classQuests = []struct {
	id   int64
	name string
}{
	{1507, "Devourer of Souls"}, {1508, "Blind Cazul"}, {1740, "The Orb of Soran'ruk"}, {1801, "Tome of the Cabal"},
	{90902, "Rediscovering the Light"}, {91208, "Coming to Terms"}, {92482, "The Way of the Hunter"}, {98601, "A Difficult Path"},
}

// TestOfflineBenchmarkClassQuestFacts: a class quest's name, level, side and class heading as the advisor words them,
// beside the quest's row in quests_observed.
func TestOfflineBenchmarkClassQuestFacts(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range classQuests {
		q := qbQuest(t, tc.id)
		c := quests.OfWith(ds, nil, tc.id)
		got := firstLine(quests.Render(c, ""))
		want := fmt.Sprintf("%s (quest %d), level %d", text(q["title"]), tc.id, num(q["level"]))
		if f := text(q["faction"]); f != "" {
			want += ", " + f
		}
		want += ", " + text(q["category"])
		if tag := map[string]string{"group": "a group quest", "dungeon": "a dungeon quest"}[text(q["tag"])]; tag != "" {
			want += ", " + tag // players tagged the quest as one for a group or a dungeon
		}
		ok := c.Found && strings.Contains(got, want+".") && !strings.Contains(got, " only")
		out = append(out, benchRow{fmt.Sprintf("%s: name, level, side, class", tc.name), got, want, cqSrc, resultIf(ok, "the title, level, side or class heading")})
	}
	finish(t, "advice-class-quest-facts", "Offline class quests: the facts the advisor gives", out)
}

// TestOfflineBenchmarkClassQuestByName: a class quest asked for by its title is found in quests_observed, one title
// held by several quests lists them by number, and a title no row holds finds nothing.
func TestOfflineBenchmarkClassQuestByName(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range classQuests {
		id, name, cands := quests.ResolveWith(ds, nil, strings.ToLower(tc.name))
		same := 0
		rows, _ := ds.Filter("quests_observed", store.Range{Column: "title", Min: tc.name, Max: tc.name})
		same = len(rows)
		var want string
		var ok bool
		if same == 1 {
			want = fmt.Sprintf("%s (%d)", tc.name, tc.id)
			ok = id == tc.id && name == tc.name
		} else {
			var ids []string
			for _, r := range rows {
				ids = append(ids, fmt.Sprintf("%s (%d)", tc.name, num(r["quest_id"])))
			}
			slices.Sort(ids)
			want = "several quests: " + strings.Join(ids, ", ")
			slices.Sort(cands)
			ok = id == 0 && slices.Equal(cands, ids)
		}
		got := fmt.Sprintf("%s (%d)", name, id)
		if id == 0 {
			got = "several quests: " + strings.Join(cands, ", ")
		}
		out = append(out, benchRow{tc.name + ": by its title", got, want, cqSrc, resultIf(ok, "the quest was not found by its title, or its candidates differ")})
	}
	id, name, cands := quests.ResolveWith(ds, nil, "The Synthetic Rite of the Pale Oak")
	out = append(out, benchRow{"a quest title no row holds", fmt.Sprintf("id %d, %q, %d candidates", id, name, len(cands)), "no title in quests_observed", cqSrc, resultIf(id == 0 && len(cands) == 0, "a quest was invented")})
	finish(t, "advice-class-quest-by-name", "Offline class quests: found by title", out)
}

// TestOfflineBenchmarkClassQuestNoAnswer: the dataset has no quest line for a class quest and no class column. The
// advisor lists no next quest, names no class restriction beyond the heading, and says so for a quest the dataset lacks.
func TestOfflineBenchmarkClassQuestNoAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, id := range []int64{1507, 1740, 90902, 92482} {
		lines, _ := ds.Filter("quest_lines", store.Range{Column: "quest_id", Min: id, Max: id})
		c := quests.OfWith(ds, nil, id)
		got := quests.Render(c, "")
		out = append(out, benchRow{fmt.Sprintf("quest %d: the next quest of the class line", id), strings.Split(strings.TrimSpace(got), "\n")[1], fmt.Sprintf("%d quest_lines rows; the dataset has no class line", len(lines)), cqLineSrc,
			resultIf(len(lines) == 0 && c.NextQuestID == 0 && strings.Contains(got, "No follow-up quest is listed") && !strings.Contains(got, "Leads to"), "a next quest was named")})
	}
	c := quests.OfWith(ds, nil, 99999999)
	out = append(out, benchRow{"a quest the dataset does not hold", strings.TrimSpace(quests.Render(c, "")), "no row in quests, quests_observed or quest_lines", cqLineSrc, resultIf(!c.Found && strings.Contains(quests.Render(c, ""), "No quest line or follow-up quest is listed"), "a quest was invented")})
	finish(t, "advice-class-quest-no-answer", "Offline class quests: no answer in the dataset", out)
}

// secTrainerCases are characters of both sides with a secondary profession: skill, rank held, level.
type secTrainer struct {
	prof, faction     string
	skill, cap, level int
}

var secTrainerCases = []secTrainer{
	{"Cooking", "Horde", 40, 75, 15}, {"Cooking", "Alliance", 70, 75, 12}, {"First Aid", "Horde", 130, 150, 25}, {"First Aid", "Alliance", 60, 75, 8},
	{"Fishing", "Horde", 140, 150, 22}, {"Fishing", "Alliance", 200, 225, 30},
}

// homeZones are the zones a side's own capital cities and starting areas stand in.
var homeZones = map[string][]string{
	"Horde":    {"Orgrimmar", "Durotar", "Undercity", "Tirisfal Glades", "Silverpine Forest", "Mulgore", "Thunder Bluff"},
	"Alliance": {"Stormwind City", "Elwynn Forest", "Westfall", "Ironforge", "Dun Morogh", "Loch Modan", "Darnassus", "Teldrassil"},
}

// TestOfflineBenchmarkSecondaryTrainer: the town the advisor sends a Cooking, First Aid or Fishing character to, beside the
// zones of the dataset's trainers of the profession that lie in the character's own side's cities, and the level the
// next rank opens at beside the dataset's tier row.
func TestOfflineBenchmarkSecondaryTrainerA(t *testing.T) {
	secondaryTrainer(t, "a", secTrainerCases[:2])
}
func TestOfflineBenchmarkSecondaryTrainerB(t *testing.T) {
	secondaryTrainer(t, "b", secTrainerCases[2:4])
}
func TestOfflineBenchmarkSecondaryTrainerC(t *testing.T) {
	secondaryTrainer(t, "c", secTrainerCases[4:])
}

func secondaryTrainer(t *testing.T, part string, cases []secTrainer) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range cases {
		level, skill, top := tc.level, tc.skill, tc.cap
		c := character.Character{Faction: tc.faction, Class: "MAGE", Level: &level, Professions: []character.Skill{{Name: tc.prof, Rank: &skill, Max: &top}}}
		pa := professions.ComputePath(c, ds, tc.prof, tc.skill+3, 1)
		words := strings.Join(pa.Where, " ")
		rows, _ := ds.Filter("trainers", store.Range{Column: "profession", Min: tc.prof, Max: tc.prof})
		var own []string
		for _, r := range rows {
			if z := text(r["zone_name"]); slices.Contains(homeZones[tc.faction], z) && !slices.Contains(own, z) {
				own = append(own, z)
			}
		}
		slices.Sort(own)
		line := firstWhere(words, "Trainer to visit")
		inOwn := false
		for _, z := range own {
			if strings.HasSuffix(strings.TrimSuffix(line, "."), " in "+z) {
				inOwn = true
			}
		}
		label := fmt.Sprintf("%s (%s, skill %d, level %d)", tc.prof, strings.ToLower(tc.faction), tc.skill, tc.level)
		out = append(out, benchRow{label + ": trainer town", line, "trainer zones on the side: " + strings.Join(own, ", "), secTrainSrc, resultIf(inOwn, "a town that is not one of the side's trainer towns")})

		tiers, _ := ds.Filter("profession_tiers", store.Range{Column: "profession", Min: tc.prof, Max: tc.prof})
		next := firstWhere(words, "Next tier")
		m := regexp.MustCompile(`Next tier: (\w+), cap \d+, opens at level (\d+)`).FindStringSubmatch(next)
		res, ref := "differ: no next rank was stated", "a next rank"
		if m != nil {
			ref = "no row for " + m[1] + " with a level"
			for _, r := range tiers {
				if text(r["tier"]) == m[1] && num(r["min_level"]) > 0 {
					ref = fmt.Sprintf("%s opens at level %d (skill %d)", m[1], num(r["min_level"]), num(r["skill_required"]))
					res = resultIf(m[2] == fmt.Sprint(num(r["min_level"])), "the level the rank opens at")
				}
			}
			if strings.HasPrefix(ref, "no row") {
				res = "differ (data): the tier's min_level is empty in the dataset; the advisor uses the game's gate"
			}
		}
		out = append(out, benchRow{label + ": next rank gate", next, ref, secTrainSrc, res})
	}
	finish(t, "advice-secondary-trainer-"+part, "Offline Cooking, First Aid and Fishing: trainer town and next rank ("+part+")", out)
}

// TestOfflineBenchmarkSecondaryRecipeSource: where a Cooking or First Aid recipe is learned, and the skill it needs, beside the
// recipe's row, and the item that teaches it.
func TestOfflineBenchmarkSecondaryRecipeSource(t *testing.T) {
	cases := []struct {
		name string
		ds   store.DataStore
	}{
		{"Charred Wolf Meat", consStore()}, {"Spiced Wolf Meat", consStore()}, {"Boiled Clams", consStore()}, {"Crab Cake", consStore()},
		{"Goblin Deviled Clams", consStore()}, {"Smoked Sagefish", consStore()}, {"Scorpid Surprise", consStore()},
		{"Heavy Linen Bandage", qbStore()}, {"Linen Bandage", qbStore()},
	}
	var out []benchRow
	for _, tc := range cases {
		rows, err := tc.ds.Filter("recipes", store.Range{Column: "name", Min: tc.name, Max: tc.name})
		if err != nil || len(rows) == 0 {
			t.Fatalf("%s: %v", tc.name, err)
		}
		r := rows[0]
		prof := text(r["skill_line_name"])
		res, err := learn.Find(horde(40), tc.ds, nil, tc.name)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		skill := num(r["learn_skill"])
		from := text(r["learned_from"])
		ok := true
		ref := fmt.Sprintf("%s: learn skill %d, learned from %s", prof, skill, from)
		switch from {
		case "trainer":
			ok = strings.Contains(res.Text, "Ask a "+prof+" trainer")
		case "item":
			items, _ := tc.ds.Filter("recipe_items", store.Range{Column: "teaches_spell_id", Min: num(r["spell_id"]), Max: num(r["spell_id"])})
			places := 0
			for _, it := range items {
				v, _ := tc.ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: num(it["item_id"]), Max: num(it["item_id"])})
				l, _ := tc.ds.Filter("loot_sources", store.Range{Column: "item_id", Min: num(it["item_id"]), Max: num(it["item_id"])})
				places += len(v) + len(l)
			}
			ref += fmt.Sprintf("; %d vendor and drop rows for the recipe item", places)
			if places > 0 {
				ok = strings.Contains(res.Text, "Buy the recipe") || strings.Contains(res.Text, "drops")
			} else {
				ok = strings.Contains(res.Text, "names no place")
			}
		}
		if skill > 1 {
			about := ""
			if text(r["learn_skill_source"]) == "estimate" {
				about = "about " // an estimated skill is worded as one
			}
			ok = ok && strings.Contains(res.Text, fmt.Sprintf("It needs %s%s %d.", about, prof, skill))
		}
		out = append(out, benchRow{tc.name + ": where it is learned and the skill it needs", strings.ReplaceAll(res.Text, "\n", " "), ref, secLearnSrc, resultIf(ok, "the source or the skill needed")})
	}
	finish(t, "advice-secondary-recipe-source", "Offline Cooking and First Aid: where a recipe is learned", out)
}
