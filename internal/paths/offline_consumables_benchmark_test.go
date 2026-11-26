package paths_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/WoW-Companion/advisor/internal/benchsummary"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline enchant, gem, consumable and mount benchmark: what the advisor answers about an enchanting recipe, a
// gem, a flask, elixir, potion or food, and a mount, beside the dataset's rows, with no network.
// testdata/consumables_snapshot (testdata/consumables_snapshot/build.sh) holds the real rows: every consumable and
// mount item, the Enchanting, Alchemy and Cooking recipes with reagents, recipe items and trainers, and who sells or
// drops the items. The dataset has no gem item, no enchant slot or stat, no flask duration or stat, and no mount
// source (vendor, drop or quest), faction or riding skill, so the cases it cannot answer check that the advisor says
// so and states none of them. The snapshot names no vendor for most rows, so a creature with no name is given a
// synthetic one; the rows stay the dataset's. make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	consDir    = "../../testdata/consumables_snapshot"
	enchantSrc = "dataset recipes (learn_skill, learned_from), recipe_items, trainer_spells"
	consSrc    = "dataset items (bonding, buy_price), recipes, vendor_sources"
	mountSrc   = "dataset items (class 15, subclass 5: required_level), vendor_sources, loot_sources"
	noneSrc    = "the dataset has no such column or table"
)

type benchRow struct{ check, advisor, reference, source, result string }

func resultIf(ok bool, bad string) string {
	if ok {
		return "match"
	}
	return "differ: " + bad
}

func finish(t *testing.T, area, title string, rows []benchRow) {
	t.Helper()
	results := make([]string, len(rows))
	for i, r := range rows {
		results[i] = r.result
		if strings.HasPrefix(r.result, "differ") && !strings.Contains(r.result, "(data)") {
			t.Errorf("%s: advisor %q, reference %q: %s", r.check, r.advisor, r.reference, r.result)
		}
	}
	fmt.Println(benchsummary.AreaLine(area, results))
	path := os.Getenv("WOWC_BENCH_REPORT")
	if path == "" || len(rows) == 0 {
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### %s\n\n| Check | Advisor | Reference | Source | Result |\n| --- | --- | --- | --- | --- |\n", title)
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", r.check, r.advisor, r.reference, r.source, r.result)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(b.String() + "\n"); err != nil {
		t.Fatal(err)
	}
}

// consVendors gives a vendor creature the snapshot lacks a name for a synthetic one, so the price logic can run.
type consVendors struct{ store.DataStore }

func (n consVendors) Get(table string, id int64) (store.Row, error) {
	r, err := n.DataStore.Get(table, id)
	if table == "npcs" && (err != nil || fmt.Sprint(r["name"]) == "" || r["name"] == nil) {
		return store.Row{"id": id, "name": fmt.Sprintf("Synthetic Vendor %d", id), "zone_name": "Synthetic Vale"}, nil
	}
	return r, err
}

var (
	consOnce  sync.Once
	consCache store.DataStore
)

// consStore reads the snapshot once for the package: the tables are decoded on first use and the tests share them.
func consStore() store.DataStore {
	consOnce.Do(func() { consCache = consVendors{store.NewDir(consDir)} })
	return consCache
}

func num(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int32:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}

func text(v any) string { s, _ := v.(string); return s }

func rowByName(t *testing.T, ds store.DataStore, table, name string) store.Row {
	t.Helper()
	rows, err := ds.Filter(table, store.Range{Column: "name", Min: name, Max: name})
	if err != nil || len(rows) == 0 {
		t.Fatalf("%s %q: not in the snapshot (%v)", table, name, err)
	}
	return rows[0]
}

func firstLine(s string) string { return strings.SplitN(s, "\n", 2)[0] }

func horde(level int) character.Character {
	return character.Character{Faction: "Horde", Class: "MAGE", Level: &level}
}

// enchantCases are Enchanting recipes the snapshot holds: learned from a trainer, from a formula, or both.
var enchantCases = []string{
	"Enchant Bracer - Inferior Stamina", "Enchant Chest - Minor Absorption", "Enchant Cloak - Minor Resistance", "Enchant Bracer - Minor Stamina",
	"Enchant Cloak - Minor Protection", "Enchant Chest - Minor Intellect", "Enchant Bracer - Minor Spirit", "Enchant 2H Weapon - Lesser Intellect",
	"Enchant Boots - Minor Stamina", "Enchant Boots - Minor Agility", "Enchant Weapon - Insight", "Enchant Chest - Greater Intellect",
	"Enchant Shield - Lesser Block", "Enchant Gloves - Skinning",
}

// TestOfflineBenchmarkEnchantRecipes: where an enchant is learned and the skill it needs, beside the recipe row.
func TestOfflineBenchmarkEnchantRecipes(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, name := range enchantCases {
		r := rowByName(t, ds, "recipes", name)
		from, need := text(r["learned_from"]), num(r["learn_skill"])
		res, err := learn.Find(horde(60), ds, nil, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		gate := fmt.Sprintf("%s %d", res.Profession, res.Gate.Skill)
		out = append(out, benchRow{name + ": skill to learn it", gate, fmt.Sprintf("%s %d (learn_skill)", text(r["skill_line_name"]), need), enchantSrc,
			resultIf(res.Profession == text(r["skill_line_name"]) && int64(res.Gate.Skill) == need, "the profession or the skill needed")})
		saysItem := strings.Contains(res.Text, "recipe item") || strings.Contains(res.Text, "Buy the recipe") || strings.Contains(res.Text, "drops from")
		saysTrainer := strings.Contains(res.Text, "trainer") || strings.Contains(res.Text, "Learn it from")
		ok := from == "item" && saysItem && !saysTrainer || from == "trainer" && saysTrainer && !saysItem || from == "both" && (saysItem || saysTrainer)
		out = append(out, benchRow{name + ": how it is learned", firstLine(res.Text), "learned_from " + from, enchantSrc, resultIf(ok, "a trainer for a formula recipe, or a formula for a trainer one")})
	}
	finish(t, "advice-enchant-recipes", "Offline enchants: where to learn and the skill needed", out)
}

var (
	article  = regexp.MustCompile(`\b[Aa] [AEIOUaeiou]\w+ trainer\b`)
	numbered = regexp.MustCompile(`\+\d+|\b\d+ (minutes?|hours?|seconds?|sec|min)\b|\bduration\b|\blasts?\b`)
	factions = regexp.MustCompile(`(?i)\b(alliance|horde|faction)\b`)
)

// TestOfflineBenchmarkEnchantWording: the answer for a trainer recipe reads as English.
func TestOfflineBenchmarkEnchantWording(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, name := range []string{"Enchant Bracer - Inferior Stamina", "Enchant Bracer - Minor Agility"} {
		res, err := learn.Find(horde(60), ds, nil, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out = append(out, benchRow{name + ": wording", firstLine(res.Text), "an Enchanting trainer", enchantSrc, resultIf(!article.MatchString(res.Text), `"a Enchanting trainer"`)})
	}
	finish(t, "advice-enchant-wording", "Offline enchants: the sentence for a trainer recipe", out)
}

// craftedCases are flasks, elixirs, potions and cooked food the snapshot's recipes make: Alchemy and Cooking, learned
// from a trainer or from a recipe item, from skill 15 to 300.
var craftedCases = []string{
	"Elixir of Minor Strength", "Swiftness Potion", "Draught of Water Breathing", "Elixir of Lesser Defense", "Elixir of Agility",
	"Elixir of the Mongoose", "Flask of Distilled Wisdom", "Flask of Petrification", "Flask of Natural Aggression", "Fire Protection Potion",
	"Fillet of Frenzy", "Westfall Stew", "Crocolisk Steak", "Lean Wolf Steak",
}

// itemOf is the items row of a name, and the recipe that makes it.
func itemOf(t *testing.T, ds store.DataStore, name string) (store.Row, store.Row) {
	t.Helper()
	it := rowByName(t, ds, "items", name)
	rs, _ := ds.Filter("recipes", store.Range{Column: "creates_item_id", Min: num(it["id"]), Max: num(it["id"])})
	if len(rs) == 0 {
		t.Fatalf("%s: no recipe makes it", name)
	}
	return it, rs[0]
}

func craftLine(res paths.Result) string {
	for _, p := range res.Paths {
		if p.Kind == paths.Craft {
			return p.Line
		}
	}
	return ""
}

var madeBy = regexp.MustCompile(`\(skill (\d+)\)`)

// TestOfflineBenchmarkConsumableCrafts: the skill a flask, elixir, potion or food needs and what the answer says
// about binding, beside the recipe and the item row.
func TestOfflineBenchmarkConsumableCrafts(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, name := range craftedCases {
		it, rec := itemOf(t, ds, name)
		need := num(rec["learn_skill"])
		res := paths.For(horde(60), ds, num(it["id"]), paths.Options{})
		line := craftLine(res)
		got := "no craft line"
		if m := madeBy.FindStringSubmatch(line); m != nil {
			got = "skill " + m[1]
		} else if line != "" {
			got = "no skill stated"
		}
		ref := fmt.Sprintf("%s learn_skill %d", text(rec["skill_line_name"]), need)
		ok := got == fmt.Sprintf("skill %d", need) || need <= 1 && got == "no skill stated"
		out = append(out, benchRow{name + ": skill to make it", got, ref, consSrc, resultIf(ok, "the skill the recipe needs")})
		bind := strings.Contains(line, "Bind on Equip")
		out = append(out, benchRow{name + ": binding", map[bool]string{true: "Bind on Equip", false: "no binding named"}[bind],
			fmt.Sprintf("bonding %d", num(it["bonding"])), consSrc, resultIf(bind == (num(it["bonding"]) == 2), "a binding the item does not have")})
		fl, err := learn.Find(horde(60), ds, nil, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out = append(out, benchRow{name + ": where to learn it", fmt.Sprintf("%s %d", fl.Profession, fl.Gate.Skill), fmt.Sprintf("%s %d, learned from %s", text(rec["skill_line_name"]), need, text(rec["learned_from"])), enchantSrc,
			resultIf(fl.Profession == text(rec["skill_line_name"]) && int64(fl.Gate.Skill) == need, "the profession or the skill needed")})
	}
	finish(t, "advice-consumable-crafts", "Offline consumables: crafting skill and binding", out)
}

// vendorCases are consumables a vendor sells, with the dataset's buy price, and the one advisor choice made: the
// price quoted is what a player with no reputation pays (items.buy_price), not a lower price one reporter saw.
var vendorCases = []string{
	"Lesser Healing Potion", "Refreshing Spring Water", "Tough Jerky", "Ice Cold Milk", "Homemade Cherry Pie", "Moonberry Juice",
	"Sweet Nectar", "Cured Ham Steak", "Freshly Baked Bread", "Alterac Swiss",
}

// TestOfflineBenchmarkConsumableVendors: the price a vendor offer shows beside the dataset's buy price.
func TestOfflineBenchmarkConsumableVendors(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, name := range vendorCases {
		it := rowByName(t, ds, "items", name)
		id := num(it["id"])
		rows, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		o, ok := vendors.Best(ds, id)
		got := "no offer"
		if ok {
			got = o.Price
		}
		ref := fmt.Sprintf("buy price %s (%d vendor rows)", vendors.Money(num(it["buy_price"])), len(rows))
		out = append(out, benchRow{name + ": vendor price", got, ref, consSrc, resultIf(ok && o.PriceCopper == num(it["buy_price"]), "a discounted or missing price")})
	}
	finish(t, "advice-consumable-vendors", "Offline consumables: vendor price", out)
}

// TestOfflineBenchmarkConsumableWays: the way the advisor gives for a consumable a vendor sells is the vendor.
func TestOfflineBenchmarkConsumableWays(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, name := range vendorCases[:5] {
		it := rowByName(t, ds, "items", name)
		id := num(it["id"])
		rows, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		best, found := paths.For(horde(60), ds, id, paths.Options{}).Best()
		out = append(out, benchRow{name + ": way to get it", map[bool]string{true: best.Kind, false: "none"}[found], fmt.Sprintf("%d vendor rows", len(rows)), consSrc,
			resultIf(found && best.Kind == paths.Vendor, "a way other than the vendor that sells it")})
	}
	finish(t, "advice-consumable-ways", "Offline consumables: way to get it", out)
}

// mountCases are mount items of the snapshot: level 40 and level 60, Horde and Alliance mounts, bought and dropped ones
// (the dataset names no source for any).
var mountCases = []string{
	"Gray Ram", "Black Stallion Bridle", "Red Mechanostrider", "Whistle of the Emerald Raptor", "Gray Kodo", "Reins of the Striped Nightsaber",
	"Swift White Ram", "Reins of the Swift Frostsaber", "Deathcharger's Reins", "Horn of the Red Wolf", "Great White Kodo", "Swift Green Mechanostrider",
	"Reins of the Spectral Bear",
}

// TestOfflineBenchmarkMountLevel: the level the advisor assumes a mount from, beside each mount item's required level.
func TestOfflineBenchmarkMountLevel(t *testing.T) {
	ds := consStore()
	var out []benchRow
	lowest := int64(1000)
	for _, name := range mountCases {
		it := rowByName(t, ds, "items", name)
		rl := num(it["required_level"])
		lowest = min(lowest, rl)
		out = append(out, benchRow{name + ": level needed", fmt.Sprintf("mounted from level %d", session.MountLevel), fmt.Sprintf("required_level %d", rl), mountSrc,
			resultIf(rl >= int64(session.MountLevel), "a mount needs less than the level the advisor assumes")})
	}
	out = append(out, benchRow{"lowest mount level", fmt.Sprintf("level %d", session.MountLevel), fmt.Sprintf("lowest required_level %d", lowest), mountSrc,
		resultIf(lowest == int64(session.MountLevel), "the assumed mount level is not the lowest a mount needs")})
	finish(t, "advice-mount-level", "Offline mounts: level needed", out)
}

// TestOfflineBenchmarkMountSource: how to get a mount, where the dataset lists no vendor, creature or quest for it.
// The answer must say the data lists no source; it must not call the mount not worth chasing.
func TestOfflineBenchmarkMountSource(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, name := range mountCases {
		it := rowByName(t, ds, "items", name)
		id := num(it["id"])
		v, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		l, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: id, Max: id})
		res := paths.For(horde(60), ds, id, paths.Options{})
		ref := fmt.Sprintf("%d vendor rows, %d loot rows", len(v), len(l))
		bad := strings.Contains(res.Reason, "not worth chasing")
		out = append(out, benchRow{name + ": how to get it", res.Reason, ref, mountSrc, resultIf(!res.Dependable() && !bad && strings.Contains(res.Reason, "no source"), "a mount called not worth chasing, or no statement that the data has no source")})
	}
	finish(t, "advice-mount-source", "Offline mounts: how to get one", out)
}

// gemCases are gem names no dataset item carries.
var gemCases = []string{"Delicate Blood Garnet", "Brilliant Golden Draenite", "Bold Living Ruby", "Solid Star of Elune"}

// TestOfflineBenchmarkNoAnswer: what the dataset cannot answer (gem items, an enchant's slot and stat amount, a flask's
// duration and stat, a mount's source, faction and riding skill): the advisor says so and names none.
func TestOfflineBenchmarkNoAnswer(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, g := range gemCases {
		id, _, cands := farm.Resolve(ds, g)
		_, err := learn.Find(horde(60), ds, nil, g)
		out = append(out, benchRow{g + ": gem", fmt.Sprintf("item %d, %d candidates, learn: %v", id, len(cands), err), "no item of class 3 (gem) in the dataset", noneSrc,
			resultIf(id == 0 && len(cands) == 0 && err != nil, "a gem was matched")})
	}
	flask, _ := itemOf(t, ds, "Flask of Distilled Wisdom")
	elixir, _ := itemOf(t, ds, "Elixir of the Mongoose")
	fr := paths.For(horde(60), ds, num(flask["id"]), paths.Options{})
	er := paths.For(horde(60), ds, num(elixir["id"]), paths.Options{})
	out = append(out, benchRow{"Flask of Distilled Wisdom: how long it lasts", fr.Text(), "no duration in the dataset", noneSrc, resultIf(!numbered.MatchString(fr.Text()), "a duration or stat amount was stated")})
	out = append(out, benchRow{"Flask of Distilled Wisdom: what it adds", fr.Text(), "items.stats is empty for every consumable", noneSrc, resultIf(!numbered.MatchString(fr.Text()), "a stat amount was stated")})
	out = append(out, benchRow{"Elixir of the Mongoose: what it adds", er.Text(), "items.stats is empty for every consumable", noneSrc, resultIf(!numbered.MatchString(er.Text()), "a stat amount was stated")})
	for _, name := range []string{"Enchant Chest - Greater Intellect", "Enchant Weapon - Insight"} {
		res, err := learn.Find(horde(60), ds, nil, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out = append(out, benchRow{name + ": stat amount and item slot", firstLine(res.Text), "the dataset has no enchant slot or stat table", noneSrc, resultIf(!numbered.MatchString(res.Text), "a stat amount or duration was stated")})
	}
	mount := rowByName(t, ds, "items", "Gray Ram")
	for _, c := range []character.Character{horde(60), {Faction: "Alliance", Class: "MAGE", Level: func() *int { n := 60; return &n }()}} {
		mr := paths.For(c, ds, num(mount["id"]), paths.Options{})
		sp := farm.Render("Gray Ram", farm.Spots(c, ds, num(mount["id"])), farm.Crafted(ds, num(mount["id"])), 5)
		out = append(out, benchRow{"Gray Ram (" + c.Faction + "): which faction can use it", mr.Text(), "allowable_class_mask -1; no faction column", noneSrc, resultIf(!factions.MatchString(mr.Text()) && !strings.Contains(strings.ToLower(mr.Text()), "riding"), "a faction or riding skill was named")})
		out = append(out, benchRow{"Gray Ram (" + c.Faction + "): where it drops", firstLine(strings.TrimSpace(strings.TrimPrefix(sp, "Farm spots for Gray Ram"))), "0 loot rows", noneSrc, resultIf(strings.Contains(sp, "No creature or object drops it"), "a source was named")})
	}
	finish(t, "advice-consumables-no-answer", "Offline enchants, gems, consumables and mounts: no answer in the dataset", out)
}
