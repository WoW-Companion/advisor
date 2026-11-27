package paths_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// The offline spell-rank, reagent-vendor and recipe-source benchmark: the rank of an ability a class has trained by a level, which
// vendor sells a crafting material and at what price, and where a recipe is learned (a trainer, a vendor, a drop), beside the dataset's
// rows, with no network. It reads testdata/usability_snapshot (class_spells), testdata/tradeskills_snapshot (recipes, recipe_reagents,
// vendor_sources, items) and testdata/sources_snapshot with the places and advice snapshots (recipe items, loot, trainers). The dataset
// names no vendor of a crafting material and holds no vendor place for one (npcs has no row for them), so a vendor is named only
// where a creature has a name, and a case whose place the dataset lacks says so and states none. A difference the dataset's own rows
// cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	rvRankSrc   = "dataset class_spells (name, rank, level, superseded_spell_id); Wowhead Classic class trainer lists"
	rvVendorSrc = "dataset vendor_sources (npc_id, price_copper, limited), items (buy_price), recipe_reagents; game: reagents sold by profession vendors"
	rvRecipeSrc = "dataset recipes (learned_from, learn_skill), recipe_items, trainer_spells, vendor_sources, loot_sources, npcs"
	rvNoneSrc   = "the dataset has no such row, column or table"
)

// ---- the rank of an ability a class has trained by a level ----

// rvTrained is the highest rank of an ability the dataset's class_spells holds at or below a level, and the level that rank is trained at.
func rvTrained(t *testing.T, class, name string, level int) (rank, at int64) {
	t.Helper()
	for _, r := range classRows(t, classID(class), 1, level) {
		if text(r["name"]) == name && num(r["rank"]) > rank {
			rank, at = num(r["rank"]), num(r["level"])
		}
	}
	return rank, at
}

var rvRankCases = []struct {
	class, name string
	level       int
}{
	{"ROGUE", "Sinister Strike", 20}, {"ROGUE", "Eviscerate", 33}, {"HUNTER", "Arcane Shot", 30}, {"HUNTER", "Serpent Sting", 45},
	{"DRUID", "Wrath", 40}, {"DRUID", "Healing Touch", 27}, {"SHAMAN", "Lightning Bolt", 26}, {"SHAMAN", "Healing Wave", 38},
	{"PALADIN", "Holy Light", 35}, {"PALADIN", "Consecration", 52}, {"PRIEST", "Lesser Heal", 14}, {"PRIEST", "Renew", 44},
	{"WARLOCK", "Immolate", 50}, {"WARLOCK", "Corruption", 29}, {"MAGE", "Arcane Missiles", 45}, {"MAGE", "Frost Armor", 31},
	{"WARRIOR", "Mortal Strike", 60}, {"WARRIOR", "Battle Shout", 25},
}

func TestOfflineBenchmarkRankByLevelA(t *testing.T) { rankByLevel(t, "a", rvRankCases[:3]) }
func TestOfflineBenchmarkRankByLevelB(t *testing.T) { rankByLevel(t, "b", rvRankCases[3:6]) }
func TestOfflineBenchmarkRankByLevelC(t *testing.T) { rankByLevel(t, "c", rvRankCases[6:9]) }
func TestOfflineBenchmarkRankByLevelD(t *testing.T) { rankByLevel(t, "d", rvRankCases[9:12]) }
func TestOfflineBenchmarkRankByLevelE(t *testing.T) { rankByLevel(t, "e", rvRankCases[12:15]) }
func TestOfflineBenchmarkRankByLevelF(t *testing.T) { rankByLevel(t, "f", rvRankCases[15:]) }

// rankByLevel: the trainer list for a character over levels 1 to L offers each ability once, at the highest rank the dataset has at or
// below L, with the level of that rank, and the next level it rises (the rank above) is not offered a level early.
func rankByLevel(t *testing.T, part string, cases []struct {
	class, name string
	level       int
}) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range cases {
		rank, at := rvTrained(t, tc.class, tc.name, tc.level)
		if rank == 0 {
			// a class ability the dataset holds no rank for: nothing to compare, say so
			sp, has := levHas(levelup.Spells(usabChar(tc.class, tc.level, "Horde"), ds, 1, tc.level), tc.name)
			out = append(out, benchRow{fmt.Sprintf("%s %s at level %d: the rank", strings.ToLower(tc.class), tc.name, tc.level), fmt.Sprintf("listed %v (rank %d)", has, sp.Rank), "no ranked class_spells row at or below the level", rvRankSrc,
				resultIf(!has, "a rank was invented")})
			continue
		}
		sp, has := levHas(levelup.Spells(usabChar(tc.class, tc.level, "Horde"), ds, 1, tc.level), tc.name)
		// the rank above: the next rank's level, and the character one level under it
		var nextRank, nextAt int64
		for _, r := range classRows(t, classID(tc.class), tc.level+1, 60) {
			if text(r["name"]) == tc.name && (nextAt == 0 || num(r["level"]) < nextAt) && num(r["rank"]) > rank {
				nextRank, nextAt = num(r["rank"]), num(r["level"])
			}
		}
		early := false
		if nextAt > 0 {
			e, ok := levHas(levelup.Spells(usabChar(tc.class, int(nextAt)-1, "Horde"), ds, 1, int(nextAt)-1), tc.name)
			early = ok && int64(e.Rank) >= nextRank
		}
		ok := has && int64(sp.Rank) == rank && int64(sp.Level) == at && !early
		ref := fmt.Sprintf("rank %d, trained at level %d", rank, at)
		if nextAt > 0 {
			ref += fmt.Sprintf("; rank %d at level %d", nextRank, nextAt)
		}
		out = append(out, benchRow{fmt.Sprintf("%s %s at level %d: the rank to have", strings.ToLower(tc.class), tc.name, tc.level), fmt.Sprintf("rank %d at level %d (listed %v)", sp.Rank, sp.Level, has), ref, rvRankSrc,
			resultIf(ok, "another rank or level than the dataset's highest at or below the level, or the next rank a level early")})
	}
	finish(t, "advice-rank-by-level-"+part, "Offline spell rank: the rank trained by a level ("+part+")", out)
}

// ---- crafting materials a vendor sells ----

var rvVendorCases = []struct {
	reagent, prof, craft string
	rank                 int
}{
	{"Silken Thread", "Tailoring", "White Swashbuckler's Shirt", 100}, {"Leaded Vial", "Alchemy", "Potion of Poison Cleansing", 100}, {"Crystal Vial", "Alchemy", "Oil of Immolation", 200},
	{"Imbued Vial", "Alchemy", "Elixir of Sages", 250}, {"Coarse Blasting Powder", "Engineering", "Crafted Heavy Shot", 80}, {"Strong Flux", "Blacksmithing", "Deadly Bronze Poniard", 100},
	{"Heavy Stock", "Engineering", "Lovingly Crafted Boomstick", 130}, {"Wooden Stock", "Engineering", "Rough Boomstick", 60},
}

func TestOfflineBenchmarkReagentVendorA(t *testing.T) { reagentVendors(t, "a", rvVendorCases[:2]) }
func TestOfflineBenchmarkReagentVendorB(t *testing.T) { reagentVendors(t, "b", rvVendorCases[2:4]) }
func TestOfflineBenchmarkReagentVendorC(t *testing.T) { reagentVendors(t, "c", rvVendorCases[4:6]) }
func TestOfflineBenchmarkReagentVendorD(t *testing.T) { reagentVendors(t, "d", rvVendorCases[6:]) }

// reagentVendors: a crafting material the dataset says vendors sell is answered as a vendor buy, at the base price of the item (what a
// neutral buyer pays, items.buy_price; else the lowest row), with limited stock said when every vendor row is limited, and the vendor
// is named only when the dataset names the creature. A vendor the dataset gives no name for is "a vendor", never a made-up place.
func reagentVendors(t *testing.T, part string, cases []struct {
	reagent, prof, craft string
	rank                 int
}) {
	plain := tsStore()
	named := consVendors{plain}
	var out []benchRow
	for _, tc := range cases {
		item := rowByName(t, plain, "items", tc.reagent)
		id := num(item["id"])
		rows := tsRows(t, "vendor_sources", "item_id", id)
		limited := len(rows) > 0
		low := int64(0)
		for _, r := range rows {
			limited = limited && r["limited"] == true
			if p := num(r["price_copper"]); p > 0 && (low == 0 || p < low) {
				low = p
			}
		}
		base := num(item["buy_price"])
		if base == 0 {
			base = low
		}
		craft := num(rowByName(t, plain, "items", tc.craft)["id"])
		c := tsChar("Horde", 40, tsSkill{tc.prof, tc.rank, 300})
		how := func(ds store.DataStore) paths.Reagent {
			for _, p := range paths.For(c, ds, craft, paths.Options{}).Paths {
				for _, g := range p.Reagents {
					if g.Name == tc.reagent {
						return g
					}
				}
			}
			t.Fatalf("%s: no reagent line in the path to %s", tc.reagent, tc.craft)
			return paths.Reagent{}
		}
		g := how(named)
		noName := how(plain)
		ok := g.Offer != nil && g.Offer.PriceCopper == base && g.Offer.Limited == limited && strings.HasPrefix(g.How, "buy from ") && strings.Contains(g.How, " for "+vendors.Money(base)) &&
			noName.Offer == nil && noName.How == "buy it from a vendor"
		got := g.How
		if g.Offer == nil {
			got = "no offer: " + g.How
		}
		out = append(out, benchRow{fmt.Sprintf("%s for %s (%s %d): the vendor and the price", tc.reagent, tc.craft, tc.prof, tc.rank), got + "; vendor unnamed: " + noName.How,
			fmt.Sprintf("%d vendor rows, lowest %s, base price %s, all limited %v", len(rows), vendors.Money(low), vendors.Money(base), limited), rvVendorSrc,
			resultIf(ok, "a price other than the base price, a stock note, or a vendor named when the dataset names none")})
		out = append(out, benchRow{fmt.Sprintf("%s: the vendor's name and place", tc.reagent), noName.How, fmt.Sprintf("%d vendors sell it", len(rows)), rvVendorSrc,
			"differ (data): npcs holds no name or zone for any vendor that sells it"})
	}
	finish(t, "advice-reagent-vendor-"+part, "Offline crafting materials: the vendor and its price ("+part+")", out)
}

// ---- where a recipe is learned ----

type rvRecipe struct {
	recipe, faction string
}

var rvTrainerRecipes = []rvRecipe{{"Brown Linen Pants", "Alliance"}, {"Cerulean Dye", "Horde"}, {"Small Leather Ammo Pouch", "Horde"}, {"Reagent Bot", "Alliance"}, {"Medium Armor Kit", "Horde"}}
var rvVendorRecipes = []rvRecipe{{"Woolen Reagent Bag", "Horde"}, {"Linen Reagent Bag", "Alliance"}, {"Longjaw Mud Snapper", "Horde"}, {"Murloc Scale Breastplate", "Alliance"}, {"Red Linen Bag", "Horde"}, {"Strider Stew", "Alliance"}}

func TestOfflineBenchmarkRecipeSourceTrainers(t *testing.T) {
	recipeSources(t, "trainers", rvTrainerRecipes)
}
func TestOfflineBenchmarkRecipeSourceVendors(t *testing.T) {
	recipeSources(t, "vendors", rvVendorRecipes)
}

// recipeSources: where a recipe is learned, by the way the dataset says (learned_from): a trainer recipe asks the profession's trainer
// at the skill trainer_spells needs, an item recipe is bought from a vendor the dataset names for the side or said to come from the
// recipe item, and neither is sent to the other's place.
func recipeSources(t *testing.T, part string, cases []rvRecipe) {
	ds := qbStore()
	var out []benchRow
	for _, tc := range cases {
		res, err := learn.Find(character.Character{Faction: tc.faction, Class: "MAGE"}, ds, nil, tc.recipe)
		if err != nil {
			t.Fatalf("%s: %v", tc.recipe, err)
		}
		recs, _ := ds.SearchName("recipes", tc.recipe, 5)
		rec := recs[0]
		for _, r := range recs {
			if text(r["name"]) == tc.recipe {
				rec = r
			}
		}
		spell := num(rec["spell_id"])
		from := text(rec["learned_from"])
		skill := text(rec["skill_line_name"])
		need := num(rec["learn_skill"])
		trs, _ := ds.Filter("trainer_spells", store.Range{Column: "spell_id", Min: spell, Max: spell})
		for _, r := range trs {
			if n := num(r["required_skill"]); n > 1 && (n < need || need == 0) {
				need = n
			}
		}
		ris, _ := ds.Filter("recipe_items", store.Range{Column: "teaches_spell_id", Min: spell, Max: spell})
		vend, vendNamed := 0, 0
		itemName := ""
		for _, ri := range ris {
			it, _ := ds.Get("items", num(ri["item_id"]))
			itemName = text(it["name"])
			n, rows := refVendors(ds, num(ri["item_id"]), tc.faction)
			vend += rows
			vendNamed += len(n)
		}
		ok := true
		bad := ""
		switch from {
		case "trainer":
			ok = strings.Contains(res.Text, "trainer") && !strings.Contains(res.Text, "Buy the recipe") && !strings.Contains(res.Text, "recipe item")
			if len(res.Ways) == 0 {
				ok = ok && strings.HasPrefix(res.Text, "Ask "+wording.A(skill)+" trainer")
			}
			bad = "a vendor or a recipe item for a trainer recipe"
		case "item":
			ok = !strings.Contains(res.Text, "Ask ") && (vendNamed > 0 && strings.Contains(res.Text, "Buy the recipe from") || vendNamed == 0 && strings.Contains(res.Text, "recipe item "+itemName))
			bad = "a trainer for an item recipe, or a vendor the dataset does not name"
		}
		ok = ok && res.Gate.Skill == int(need)
		ref := fmt.Sprintf("%s; %s %d; %d trainer rows; recipe item %q, %d vendor rows, %d named", from, skill, need, len(trs), itemName, vend, vendNamed)
		out = append(out, benchRow{fmt.Sprintf("%s (%s): where it is learned", tc.recipe, tc.faction), firstLine(res.Text) + " / " + res.Gate.Line, ref, rvRecipeSrc, resultIf(ok, bad+", or another skill than the dataset's")})
		if from == "trainer" && len(res.Ways) == 0 && len(trs) > 0 {
			out = append(out, benchRow{tc.recipe + ": trainer name", "no trainer named", fmt.Sprintf("%d trainers teach it", len(trs)), rvRecipeSrc, "differ (data): trainers.name is empty for every trainer of the recipe"})
		}
		if from == "item" && vend > 0 && vendNamed == 0 {
			out = append(out, benchRow{tc.recipe + ": vendor name", "no vendor named", fmt.Sprintf("%d vendors sell it", vend), rvRecipeSrc, "differ (data): npcs holds no name for any vendor of the recipe item"})
		}
	}
	finish(t, "advice-recipe-source-"+part, "Offline recipe source: "+part, out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkRanksVendorsNoAnswer: what the dataset does not hold is not guessed: a rank above the last row (the dataset ends at
// level 60), an ability another class has, a vendor for a material no vendor sells, a place for a vendor the dataset does not name, and
// a recipe the dataset lacks.
func TestOfflineBenchmarkRanksVendorsNoAnswer(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	mage := usabChar("MAGE", 60, "Horde")
	above := levelup.Spells(mage, ds, 61, 70)
	out = append(out, benchRow{"a level 61 to 70 mage: spell ranks to train", fmt.Sprintf("%d spells", len(above)), "no class_spells row above level 60", rvRankSrc, resultIf(len(above) == 0, "a rank was invented")})
	_, has := levHas(levelup.Learned(usabChar("WARRIOR", 60, "Horde"), ds, 60), "Fireball")
	out = append(out, benchRow{"a level 60 warrior: Fireball", fmt.Sprintf("listed %v", has), "no Fireball row for a warrior", rvRankSrc, resultIf(!has, "a mage spell was given to a warrior")})
	_, has = levHas(levelup.Spells(usabChar("MAGE", 60, "Horde"), ds, 1, 60), "Frostbolt of Doom")
	out = append(out, benchRow{"a level 60 mage: a spell the dataset lacks", fmt.Sprintf("listed %v", has), "no such class_spells row", rvRankSrc, resultIf(!has, "a spell was invented")})
	rs := paths.For(tsChar("Horde", 40, tsSkill{"Tailoring", 100, 300}), consVendors{tsStore()}, num(rowByName(t, tsStore(), "items", "Bolt of Woolen Cloth")["id"]), paths.Options{})
	vend := ""
	for _, p := range rs.Paths {
		for _, g := range p.Reagents {
			if g.Name == "Wool Cloth" && g.Offer != nil {
				vend = g.How
			}
		}
	}
	out = append(out, benchRow{"Wool Cloth for Bolt of Woolen Cloth: a vendor", fmt.Sprintf("%q", vend), "no vendor_sources row: a creature drop, no vendor", rvVendorSrc, resultIf(vend == "", "a vendor was invented for a dropped material")})
	g := rvFirstReagent(t, "Rune Thread", "Tailoring", 200)
	out = append(out, benchRow{"Rune Thread: the vendor's place", g.How, "vendor_sources rows but no npcs row: no name, no zone", rvVendorSrc, resultIf(g.Offer == nil && !strings.Contains(g.How, " in "), "a place was named")})
	_, err := learn.Find(character.Character{Faction: "Horde", Class: "MAGE"}, qbStore(), nil, "Pattern: Imaginary Cloak Of Tomorrow")
	out = append(out, benchRow{"a recipe the dataset does not hold", fmt.Sprint(err), "no recipes row", rvRecipeSrc, resultIf(err != nil, "a recipe was invented")})
	finish(t, "advice-ranks-vendors-no-answer", "Offline spell ranks, reagent vendors and recipe sources: no answer in the dataset", out)
}

// rvFirstReagent finds a vendor-bought reagent in the first craft path a profession's recipe gives, for a character with no named vendor.
func rvFirstReagent(t *testing.T, reagent, prof string, rank int) paths.Reagent {
	t.Helper()
	ds := tsStore()
	id := num(rowByName(t, ds, "items", reagent)["id"])
	rr, _ := ds.Filter("recipe_reagents", store.Range{Column: "reagent_item_id", Min: id, Max: id})
	for _, r := range rr {
		rec, _ := ds.Filter("recipes", store.Range{Column: "spell_id", Min: num(r["spell_id"]), Max: num(r["spell_id"])})
		if len(rec) == 0 || rec[0]["creates_item_id"] == nil || text(rec[0]["skill_line_name"]) != prof {
			continue
		}
		for _, p := range paths.For(tsChar("Horde", 40, tsSkill{prof, rank, 300}), ds, num(rec[0]["creates_item_id"]), paths.Options{}).Paths {
			for _, g := range p.Reagents {
				if g.Name == reagent {
					return g
				}
			}
		}
	}
	t.Fatalf("%s: no reagent line", reagent)
	return paths.Reagent{}
}

// refVendors are the names of the creatures that sell an item in vendor_sources, open to the side, and how many vendor rows the table holds
// for it (a vendor with no name in npcs is counted but not named).
func refVendors(ds store.DataStore, item int64, faction string) (named []string, rows int) {
	vs, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: item, Max: item})
	for _, r := range vs {
		rows++
		n, err := ds.Get("npcs", num(r["npc_id"]))
		if err != nil || text(n["name"]) == "" || !learn.FactionOK(text(n["faction"]), faction) {
			continue
		}
		named = append(named, text(n["name"]))
	}
	slices.Sort(named)
	return named, rows
}
