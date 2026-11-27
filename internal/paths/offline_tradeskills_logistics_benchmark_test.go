package paths_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/session"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline bag-space, repair, death, riding and profession-tool benchmark: how the advisor says to get a bag or quiver and what it does with
// one in the bags, which NPCs it says repair, how it answers a question about dying, a graveyard or a corpse run, the levels it assumes a mount
// from, and which tool a profession needs and what it does with the tool in the bags, beside the dataset's rows and the game's own rules, with no
// network. It reads testdata/tradeskills_snapshot (bags, quivers, tools, recipes, vendors, notes, routes), testdata/consumables_snapshot (mounts)
// and testdata/levelling_snapshot (dungeon entrances). The dataset holds no bag size, durability, repair cost, graveyard, spirit healer, riding
// trainer or flying mount, so those cases check that the advisor says so and states none. A difference the dataset's own rows cause is marked
// "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	lgBagSrc    = "dataset items (class 1 and 11, slot 18, buy_price), recipes (skill_line_name, learn_skill), recipe_reagents; game: Warcraft Wiki Bag, Quiver"
	lgBagInvSrc = "dataset items (class_id 1 bag, 11 quiver or ammo pouch, slot 18); game: a bag worn in a bag slot is a container, not gear"
	lgRepairSrc = "game npcFlags (UNIT_NPC_FLAG_REPAIR 0x1000, VENDOR 0x80, TRAINER 0x10) as the player's QuestieDB holds them; the dataset has no repair or vendor flag"
	lgDeathSrc  = "dataset instances (entrance_zone_name); game: a ghost runs from the graveyard to the corpse, usually at the dungeon entrance (Warcraft Wiki: Spirit Healer, Graveyard)"
	lgRideSrc   = "dataset items (class 15, subclass 5: required_level); game: Warcraft Wiki, Riding: Apprentice Riding level 40 (60% mount), Journeyman level 60 (100%)"
	lgToolSrc   = "dataset items (class 2, subclass 14 tools, class 7 rods), vendor_sources, profession_notes special cases; game: Warcraft Wiki Profession tools"
	lgNoneSrc   = "the dataset has no bag size, durability, repair cost, graveyard, spirit healer, riding trainer, riding cost or flying mount row, column or table"
)

// lgItemsOnly serves items by id and no table by a filter: the inventory review then places an item by its own row alone, as it does when the other tables
// have no row for the character, and the benchmark does not weigh the whole snapshot for a bag.
type lgItemsOnly struct{ store.DataStore }

func (l lgItemsOnly) Filter(table string, r ...store.Range) ([]store.Row, error) {
	return nil, fmt.Errorf("no table %s in the bag benchmark", table)
}

func lgItemID(t *testing.T, ds store.DataStore, name string) int64 {
	t.Helper()
	return num(rowByName(t, ds, "items", name)["id"])
}

func lgCraftRow(t *testing.T, ds store.DataStore, item int64) store.Row {
	t.Helper()
	rows, err := ds.Filter("recipes", store.Range{Column: "creates_item_id", Min: item, Max: item})
	if err != nil || len(rows) == 0 {
		t.Fatalf("item %d: no recipe row (%v)", item, err)
	}
	return rows[0]
}

func lgReagents(t *testing.T, ds store.DataStore, spell int64) []string {
	t.Helper()
	rows, _ := ds.Filter("recipe_reagents", store.Range{Column: "spell_id", Min: spell, Max: spell})
	var out []string
	for _, r := range rows {
		out = append(out, text(rowOf(t, ds, "items", num(r["reagent_item_id"]))["name"]))
	}
	return out
}

func rowOf(t *testing.T, ds store.DataStore, table string, id int64) store.Row {
	t.Helper()
	r, err := ds.Get(table, id)
	if err != nil {
		t.Fatalf("%s %d: %v", table, id, err)
	}
	return r
}

// ---- bags and quivers: how to get one ----

// TestOfflineBenchmarkBagCrafts: a bag or quiver a profession makes, asked by a character who can make it, is a craft the character can make now (or
// is a few points short of), with the skill and the materials of the dataset's recipe row.
func TestOfflineBenchmarkBagCrafts(t *testing.T) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range []struct {
		item, prof string
		rank       int
	}{
		{"Linen Bag", "Tailoring", 60}, {"Woolen Bag", "Tailoring", 60}, {"Mageweave Bag", "Tailoring", 200}, {"Small Silk Pack", "Tailoring", 130},
		{"Kodo Hide Bag", "Leatherworking", 60}, {"Heavy Quiver", "Leatherworking", 130}, {"Quickdraw Quiver", "Leatherworking", 210}, {"Woolen Bag", "Tailoring", 30},
	} {
		id := lgItemID(t, ds, tc.item)
		rec := lgCraftRow(t, ds, id)
		need := int(num(rec["learn_skill"]))
		res := paths.For(tsChar("Horde", 40, tsSkill{tc.prof, tc.rank, 300}), ds, id, paths.Options{})
		var best paths.Path
		found := len(res.Paths) > 0
		if found {
			best = res.Paths[0]
		}
		ok := found && best.Kind == paths.Craft && best.Skill != nil && best.Skill.Need == need && best.Skill.Profession == tc.prof
		mats := lgReagents(t, ds, num(rec["spell_id"]))
		for _, m := range mats {
			ok = ok && strings.Contains(best.Line+lgReagentNames(best), m)
		}
		status := "can make it now"
		if tc.rank < need {
			status = "a few points short: " + fmt.Sprint(need-tc.rank) + " more"
		}
		out = append(out, benchRow{fmt.Sprintf("%s for %s %d", tc.item, tc.prof, tc.rank), firstLine(best.Line), fmt.Sprintf("%s needs %s %d (%s source); materials %s; %s", tc.item, tc.prof, need, text(rec["learn_skill_source"]),
			strings.Join(mats, ", "), status), lgBagSrc, resultIf(ok, "another skill, profession or materials than the recipe row, or no craft way")})
	}
	finish(t, "advice-bag-crafts", "Offline bags and quivers: how a crafter makes one", out)
}

func lgReagentNames(p paths.Path) string {
	var out []string
	for _, g := range p.Reagents {
		out = append(out, g.Name)
	}
	return strings.Join(out, " ")
}

// TestOfflineBenchmarkBagNotMakeable: a bag or quiver the character cannot make says it is made by a profession and names the skill; it is never called
// a thing the game data has no source for, nor not worth chasing.
func TestOfflineBenchmarkBagNotMakeable(t *testing.T) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, tc := range []struct {
		item string
		char character.Character
		who  string
	}{
		{"Linen Bag", tsChar("Horde", 30), "no profession"}, {"Mageweave Bag", tsChar("Horde", 40, tsSkill{"Mining", 60, 75}), "a miner"},
		{"Small Silk Pack", tsChar("Alliance", 30, tsSkill{"Tailoring", 20, 75}), "a tailor 105 points short"}, {"Heavy Quiver", tsChar("Horde", 40, tsSkill{"Tailoring", 100, 150}), "a tailor"},
		{"Kodo Hide Bag", tsChar("Horde", 30, tsSkill{"Skinning", 40, 75}), "a skinner"}, {"Runed Copper Rod", tsChar("Horde", 30, tsSkill{"Mining", 40, 75}), "a miner (a tool)"},
		{"Arclight Spanner", tsChar("Horde", 30, tsSkill{"Mining", 40, 75}), "a miner (a tool)"},
	} {
		id := lgItemID(t, ds, tc.item)
		rec := lgCraftRow(t, ds, id)
		res := paths.For(tc.char, ds, id, paths.Options{})
		txt := res.Text()
		prof := text(rec["skill_line_name"])
		ok := !res.Dependable() && !strings.Contains(txt, "not worth chasing") && !strings.Contains(txt, "lists no source") && strings.Contains(txt, "made by a profession") && strings.Contains(txt, prof)
		out = append(out, benchRow{fmt.Sprintf("%s for %s", tc.item, tc.who), strings.ReplaceAll(txt, "\n", " | "), fmt.Sprintf("a %s recipe, skill %d; a crafted item is no random drop and the data does have a source: the recipe", prof, num(rec["learn_skill"])), lgBagSrc,
			resultIf(ok, "called sourceless or not worth chasing, or the profession is not named")})
	}
	finish(t, "advice-bag-not-makeable", "Offline bags, quivers and tools: a craft the character cannot make", out)
}

// ---- bags in the bags ----

// TestOfflineBenchmarkBagInventory: a bag or quiver in the bags is no gear to bank or sell: the review leaves it unplaced with the reason the dataset
// holds no bag size, frees no slot for it, and the cross-character review does not sell it.
func TestOfflineBenchmarkBagInventory(t *testing.T) {
	all := tsStore()
	ds := lgItemsOnly{all}
	var out []benchRow
	for _, name := range []string{"Linen Bag", "Woolen Bag", "Mageweave Bag", "Small Silk Pack", "Kodo Hide Bag", "Heavy Leather Ammo Pouch", "Heavy Quiver", "Thick Leather Ammo Pouch"} {
		id := lgItemID(t, all, name)
		c := tsChar("Horde", 40)
		c.BagsCaptured = true
		c.Bags = []character.ItemStack{{ItemID: int(id), Count: 1}}
		r := inventory.Review(c, ds)
		var g inventory.Group
		var why string
		for gr, l := range r.Groups {
			for _, e := range l {
				g, why = gr, e.Reason
			}
		}
		cross := inventory.ReviewAll([]character.Character{c}, ds)
		action := ""
		for _, l := range cross.Lines {
			action = string(l.Action)
		}
		row := rowOf(t, ds, "items", id)
		res := resultIf(g == inventory.Unknown && r.SlotsFreed == 0 && why == inventory.BagReason && action == string(inventory.Unsure) && inventory.IsBag(row), "banked, sold or counted as a freed slot")
		out = append(out, benchRow{name + " in the bags: the review", fmt.Sprintf("group %s, %d slots freed; across characters: %s", g, r.SlotsFreed, action),
			fmt.Sprintf("class %d slot %d: a container, left unplaced (no bag size in the dataset)", num(row["class_id"]), num(row["slot"])), lgBagInvSrc, res})
	}
	finish(t, "advice-bag-inventory", "Offline bags: a bag in the bags is not gear to bank or sell", out)
}

// ---- repair vendors ----

// lgRepairIsle is a village with a boar and one NPC for each mix of vendor, repair and trainer flags, each giving a quest.
func lgRepairIsle(flags map[int]int) *questie.Index {
	ix := &questie.Index{
		Quests:  map[int]*questie.Quest{},
		Npcs:    map[int]questie.Entity{},
		Objects: map[int]questie.Entity{},
		Items:   map[int]questie.Item{},
		Areas:   map[int]questie.Area{901: {UIMap: 7001, Zone: "Testisle", Sub: "Test Village"}},
	}
	ix.Npcs[900] = questie.Entity{Name: "Test Boar", Spawns: []questie.Spawn{{Area: 901, X: 70, Y: 60}}}
	i := 0
	for id, fl := range flags {
		ix.Quests[id] = &questie.Quest{ID: id, Name: fmt.Sprintf("Errand %d", id), Level: 9, Starters: []questie.Ref{{ID: id}}, Finishers: []questie.Ref{{ID: id}}, Creatures: []questie.Ref{{ID: 900}}}
		ix.Npcs[id] = questie.Entity{Name: fmt.Sprintf("Giver %d", id), Faction: "AH", Flags: fl, Spawns: []questie.Spawn{{Area: 901, X: float64(20 + 6*i), Y: 50}}}
		i++
	}
	return ix
}

// TestOfflineBenchmarkRepairVendors: the advisor says an NPC repairs when Questie's NPC flags carry the repair bit, and only then: a vendor, a trainer or a quest
// giver with no repair bit is not named a repairer.
func TestOfflineBenchmarkRepairVendors(t *testing.T) {
	const quest, trainer, vendor, repair = 0x2, 0x10, 0x80, 0x1000
	cases := []struct {
		id    int
		label string
		flags int
		can   []string
	}{
		{101, "an armorer: quest giver, vendor and repair", quest | vendor | repair, []string{"pick up", "buy and sell", "repair"}},
		{102, "a general goods vendor: quest giver and vendor", quest | vendor, []string{"pick up", "buy and sell"}},
		{103, "a class trainer: quest giver and trainer", quest | trainer, []string{"pick up", "train"}},
		{104, "a plain quest giver", quest, []string{"pick up"}},
		{105, "a repair flag without the vendor flag", quest | repair, []string{"pick up", "repair"}},
		{106, "a trainer who is also an armorer", quest | trainer | vendor | repair, []string{"pick up", "train", "buy and sell", "repair"}},
	}
	lvl, mp := 9, 7001
	x, y := 10.0, 50.0
	c := character.Character{Name: "Hero", QuestsCaptured: true, Level: &lvl, Location: &character.Location{Zone: "Testisle", MapID: &mp, X: &x, Y: &y}}
	var out []benchRow
	for _, tc := range cases {
		p := route.Build(c, lgRepairIsle(map[int]int{tc.id: tc.flags}))
		var got []string
		for _, h := range p.Here {
			if h.NPCID == tc.id {
				got = h.Can
			}
		}
		slices.Sort(got)
		want := slices.Clone(tc.can)
		slices.Sort(want)
		out = append(out, benchRow{tc.label + fmt.Sprintf(" (npcFlags %#x)", tc.flags), strings.Join(got, ", "), strings.Join(tc.can, ", "), lgRepairSrc,
			resultIf(slices.Equal(got, want), "another set of things to do there than the flags give")})
	}
	finish(t, "advice-repair-vendors", "Offline repair vendors: what an NPC can do from its flags", out)
}

// ---- death, graveyards and corpse runs ----

// TestOfflineBenchmarkDeathQuestions: a question about dying, a corpse, a graveyard or a spirit healer is a question about how the game works, not about the
// player's saved character, and names no class or profession: the dataset holds none of these, so the answer has no fact behind it to quote.
func TestOfflineBenchmarkDeathQuestions(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"How do I get back to my corpse?", "where is the nearest graveyard", "I died and released, how do I get my body back", "what does the spirit healer do",
		"how much durability do I lose when I die", "what is resurrection sickness", "can I skip the corpse run by using the spirit healer",
	} {
		cl := ask.Classify(q, nil, nil)
		out = append(out, benchRow{"\"" + q + "\"", fmt.Sprintf("kind %d, topic %q, subject %q", cl.Kind, cl.Topic, cl.Subject), "a know-how question with no topic: the dataset holds no death, graveyard or spirit healer fact", lgNoneSrc,
			resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone, "a personal-state question, or a class, dungeon or profession topic")})
	}
	finish(t, "advice-death-questions", "Offline death, graveyards and corpse runs: the question is know-how, not state", out)
}

// TestOfflineBenchmarkCorpseRunEntrance: the dungeon a corpse run goes to is named with the entrance zone of the dataset's row, and a dungeon with no
// entrance zone in the dataset is named with none.
func TestOfflineBenchmarkCorpseRunEntrance(t *testing.T) {
	ds := levStore()
	var out []benchRow
	for _, name := range []string{"Deadmines", "Wailing Caverns", "Scarlet Monastery", "Zul'Farrak", "Scholomance", "Blackrock Depths", "Ruins of Lordaeron"} {
		rows, _ := ds.Filter("instances", store.Range{Column: "name", Min: name, Max: name})
		if len(rows) == 0 {
			t.Fatalf("%s: not in the snapshot", name)
		}
		zone := text(rows[0]["entrance_zone_name"])
		c := levChar("WARRIOR", 60)
		cl := ask.Classify(fmt.Sprintf("how do I get back to my corpse in %s?", name), ds, nil)
		subject := cl.Subject
		if zone == "" {
			subject = name // the classifier names only dungeons with a level range: ask for this one by name
		}
		p, togo, err := dungeons.Named(c, ds, nil, subject)
		got := ""
		if err == nil && p != nil {
			got = dungeons.RenderNamed(c, *p, togo)
		}
		ref := "entrance in " + zone
		ok := cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicDungeon && cl.Subject == name && err == nil && p != nil && strings.Contains(got, ref)
		if zone == "" {
			ref, ok = "no entrance zone in the dataset: the dungeon is not named, so no entrance is invented", err == nil && p == nil
		}
		out = append(out, benchRow{"corpse run into " + name, fmt.Sprintf("kind %d, topic %q, subject %q; %s", cl.Kind, cl.Topic, subject, firstMatch(got, "entrance")), ref, lgDeathSrc,
			resultIf(ok, "not a dungeon know-how question, or another entrance than the row")})
	}
	finish(t, "advice-corpse-run-entrance", "Offline corpse runs: the dungeon entrance zone", out)
}

// ---- repair cost ----

// TestOfflineBenchmarkRepairCostQuestions: how much repairs cost is a know-how question (a game rule), and the dataset holds no durability or repair cost, so
// no question about it takes a topic from a name in it.
func TestOfflineBenchmarkRepairCostQuestions(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"how much does it cost to repair my gear", "how much will repairs cost at level 40", "where can I repair my armor", "which vendor repairs in Orgrimmar",
		"does repairing in a capital cost less", "how do I repair without a vendor",
	} {
		cl := ask.Classify(q, nil, nil)
		out = append(out, benchRow{"\"" + q + "\"", fmt.Sprintf("kind %d, topic %q", cl.Kind, cl.Topic), "a know-how question: the dataset has no durability or repair cost to state", lgNoneSrc,
			resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone, "a personal-state question, or a topic taken from a word in it")})
	}
	finish(t, "advice-repair-cost-questions", "Offline repair cost: the question is know-how and the dataset has no cost", out)
}

// ---- riding and mounts ----

// TestOfflineBenchmarkMountTiers: the level a mount is bought from is the required level of its item, 40 for the 60% mounts the advisor assumes from level 40 and
// 60 for the 100% mounts it does not assume.
func TestOfflineBenchmarkMountTiers(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, tc := range []struct {
		name    string
		tier    int64
		percent int
	}{
		{"Whistle of the Emerald Raptor", 40, 60}, {"Reins of the Striped Nightsaber", 40, 60}, {"Horn of the Brown Wolf", 40, 60}, {"Red Skeletal Horse", 40, 60},
		{"Reins of the Frostsaber", 60, 100}, {"Horn of the Arctic Wolf", 60, 100}, {"Whistle of the Mottled Red Raptor", 60, 100}, {"White Stallion Bridle", 60, 100},
	} {
		rl := num(rowByName(t, ds, "items", tc.name)["required_level"])
		assumed := int64(session.MountLevel) <= rl && rl == 40
		want := tc.percent == 60
		out = append(out, benchRow{fmt.Sprintf("%s: the level it is bought from, and whether a character is assumed mounted at it", tc.name), fmt.Sprintf("required_level %d; assumed mounted from level %d: %v", rl, session.MountLevel, assumed),
			fmt.Sprintf("a %d%% mount: level %d; assumed mounted: %v", tc.percent, tc.tier, want), lgRideSrc, resultIf(rl == tc.tier && assumed == want, "another required level than the riding tier, or the 100% mount assumed")})
	}
	finish(t, "advice-mount-tiers", "Offline riding: the level a mount is bought from", out)
}

// TestOfflineBenchmarkRidingTrainer: where to learn riding, what it costs and what a flying mount needs are not answered from a recipe or a name in the dataset: the
// advisor says the data lists none, for each way of asking, and names no trainer, price or level.
func TestOfflineBenchmarkRidingTrainer(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, q := range []string{"riding trainer", "Artisan Riding", "how much is riding", "Riding", "Expert Riding"} {
		res, err := learn.Find(horde(60), ds, nil, q)
		ok := err == nil && len(res.Ways) == 0 && strings.Contains(res.Text, "lists no riding trainer, level or cost") && !numbered.MatchString(res.Text)
		out = append(out, benchRow{fmt.Sprintf("%q: where to learn it, and what it costs", q), firstLine(res.Text), "no riding trainer, skill or cost row in the dataset", lgRideSrc, resultIf(ok, "a trainer, price or level was given")})
	}
	for _, q := range []string{"Cold Weather Flying", "Flight Master's License"} {
		res, err := learn.Find(horde(60), ds, nil, q)
		out = append(out, benchRow{fmt.Sprintf("%q: a flying skill", q), fmt.Sprintf("%q, %v", firstLine(res.Text), err), "no flying skill, recipe or item in the dataset: an error, no answer", lgNoneSrc, resultIf(err != nil && res.Text == "", "a flying skill was found or invented")})
	}
	finish(t, "advice-riding-trainer", "Offline riding and flying: no trainer, cost or flying skill in the dataset", out)
}

// ---- profession tools ----

// TestOfflineBenchmarkProfessionTools: a character with a profession and no tool for it is told which tool it needs and, where vendors sell it, its dataset price;
// with the tool in the bags, bank or worn the advice does not ask for it.
func TestOfflineBenchmarkProfessionTools(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range []struct{ prof, tool string }{
		{"Mining", "Mining Pick"}, {"Skinning", "Skinning Knife"}, {"Blacksmithing", "Blacksmith Hammer"}, {"Enchanting", "Runed Copper Rod"}, {"Engineering", "Arclight Spanner"},
		{"Fishing", "Fishing Pole"},
	} {
		it := rowByName(t, ds, "items", tc.tool)
		id := num(it["id"])
		vend, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		c := tsChar("Horde", 30, tsSkill{tc.prof, 20, 75})
		without := professions.RouteFacts(c, ds, nil, tc.prof)
		c.Bags = []character.ItemStack{{ItemID: int(id), Count: 1, Name: tc.tool}}
		with := professions.RouteFacts(c, ds, nil, tc.prof)
		line := firstMatch(without, "They have no")
		ok := strings.Contains(line, tc.tool) && !strings.Contains(with, "They have no") && without != ""
		ref := fmt.Sprintf("%s needs a %s; %d vendor rows", tc.prof, tc.tool, len(vend))
		if len(vend) > 0 {
			ok = ok && strings.Contains(line, "buy "+tc.tool+" from a vendor ("+vendors.Money(num(it["buy_price"]))+")")
			ref += ", buy_price " + vendors.Money(num(it["buy_price"]))
		}
		out = append(out, benchRow{tc.prof + " at skill 20 with no " + tc.tool, line, ref, lgToolSrc, resultIf(ok, "the tool is not named, or still asked for with it in the bags")})
	}
	finish(t, "advice-profession-tools", "Offline profession tools: the tool a profession needs", out)
}

// TestOfflineBenchmarkProfessionToolKeep: a profession's tool in the bags is kept by a character that has the profession, passed on to an alt that has it, and only
// sold when no character has the profession; it is never banked or sold as gear nobody wears.
func TestOfflineBenchmarkProfessionToolKeep(t *testing.T) {
	all := tsStore()
	ds := lgItemsOnly{all}
	var out []benchRow
	for _, tc := range []struct{ tool, prof string }{
		{"Mining Pick", "Mining"}, {"Skinning Knife", "Skinning"}, {"Blacksmith Hammer", "Blacksmithing"}, {"Arclight Spanner", "Engineering"}, {"Fishing Pole", "Fishing"}, {"Runed Copper Rod", "Enchanting"},
	} {
		id := int(lgItemID(t, all, tc.tool))
		owner := tsChar("Horde", 40, tsSkill{tc.prof, 50, 75})
		owner.Name, owner.BagsCaptured = "Owner", true
		owner.Bags = []character.ItemStack{{ItemID: id, Count: 1, Name: tc.tool}}
		alt := tsChar("Horde", 40)
		alt.Name = "Alt"
		r := inventory.Review(owner, ds)
		var g inventory.Group
		for gr, l := range r.Groups {
			if len(l) > 0 {
				g = gr
			}
		}
		// the same tool held by a character without the profession, an alt that has it, then an alt that has not
		holder := tsChar("Horde", 40)
		holder.Name, holder.BagsCaptured = "Holder", true
		holder.Bags = owner.Bags
		withProf := tsChar("Horde", 40, tsSkill{tc.prof, 50, 75})
		withProf.Name = "Crafter"
		given := action(inventory.ReviewAll([]character.Character{holder, withProf}, ds))
		spare := action(inventory.ReviewAll([]character.Character{holder, alt}, ds))
		kept := action(inventory.ReviewAll([]character.Character{owner, alt}, ds))
		bound := text(rowOf(t, ds, "items", int64(id))["name"]) == "Runed Copper Rod"
		wantGiven := "mail"
		if bound {
			wantGiven = "sell" // bound on pickup: it cannot be mailed
		}
		ok := g == inventory.Keep && kept == "keep" && given == wantGiven && spare == "sell"
		out = append(out, benchRow{tc.tool + " in the bags of a " + tc.prof + " character, an alt that has it, one that has not", fmt.Sprintf("owner: %s/%s; to an alt with %s: %s; no alt has it: %s", g, kept, tc.prof, given, spare),
			fmt.Sprintf("owner keeps it; an alt with %s gets it by mail (%s); sold when no character has %s", tc.prof, wantGiven, tc.prof), lgToolSrc, resultIf(ok, "the tool was banked or sold from the character that needs it")})
	}
	finish(t, "advice-profession-tool-keep", "Offline profession tools: keep, mail or sell the tool in the bags", out)
}

func action(r inventory.CrossResult) string {
	for _, l := range r.Lines {
		return string(l.Action)
	}
	return ""
}

// ---- no answer ----

// TestOfflineBenchmarkLogisticsNoAnswer: the dataset has no bag size, durability, repair cost, graveyard, spirit healer or flying mount; no table or column holds one, so the
// advisor states none, and a bag's name does not turn a bag question into a question about a class.
func TestOfflineBenchmarkLogisticsNoAnswer(t *testing.T) {
	ds := consVendors{tsStore()}
	var out []benchRow
	for _, table := range []string{"graveyards", "spirit_healers", "repair_costs", "bag_slots", "riding_trainers", "flying_mounts"} {
		_, err := ds.Filter(table, store.Range{Column: "id", Min: int64(0), Max: int64(1)})
		out = append(out, benchRow{"a table of " + strings.ReplaceAll(table, "_", " "), fmt.Sprint(err != nil), "no such table in the dataset", lgNoneSrc, resultIf(err != nil, "a table was invented")})
	}
	rows, _ := ds.Filter("items", store.Range{Column: "name", Min: "Mageweave Bag", Max: "Mageweave Bag"})
	for _, col := range []string{"container_slots", "durability", "repair_cost"} {
		_, has := rows[0][col]
		out = append(out, benchRow{"the items column " + col, fmt.Sprint(has), "no such column in items", lgNoneSrc, resultIf(!has, "a column was invented")})
	}
	for _, q := range []string{"how many slots does a Mageweave Bag have", "how many slots does a Linen Bag give me", "what is the biggest bag I can buy", "what level do I need for a flying mount", "can I fly in Azeroth"} {
		cl := ask.Classify(q, ds, nil)
		out = append(out, benchRow{"\"" + q + "\"", fmt.Sprintf("kind %d, topic %q, subject %q", cl.Kind, cl.Topic, cl.Subject), "a know-how question with no topic: no bag size or flying fact is held; \"Mageweave\" is no mage", lgNoneSrc,
			resultIf(cl.Kind == ask.KindKnowHow && cl.Topic == ask.TopicNone, "a class topic was taken from the name of a bag, or a personal-state question")})
	}
	bag := paths.For(tsChar("Horde", 40, tsSkill{"Tailoring", 200, 300}), ds, lgItemID(t, ds, "Mageweave Bag"), paths.Options{}).Text()
	out = append(out, benchRow{"Mageweave Bag: how many slots", firstLine(bag), "no slot count in the dataset: none is stated", lgNoneSrc, resultIf(!strings.Contains(strings.ToLower(bag), "slots"), "a slot count was stated")})
	finish(t, "advice-logistics-no-answer", "Offline bags, repair, death, riding and flying: no answer in the dataset", out)
}
