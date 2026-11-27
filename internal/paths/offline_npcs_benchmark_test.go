package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dungeons"
	"github.com/WoW-Companion/advisor/internal/farm"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/paths"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/vendors"
)

// The offline quest-giver, vendor, profession skill-gate and zone-level-fit benchmark: what the advisor says of who gives
// and takes in a quest, of which vendor sells an item, of the skill a recipe or a gathering node needs, and of whether a
// dungeon's zone suits the character's level, beside the dataset's rows and the guides, with no network. It reads
// testdata/places_snapshot (quests_observed, npcs, vendor_sources, instances), testdata/advice_snapshot (items, recipes)
// and testdata/tradeskills_snapshot (recipes, gathering_nodes, loot). The dataset's quests_observed rows carry a giver
// creature id for some quests and no turn-in id, and its npcs table names none of them; no vendor in vendor_sources has a
// name in npcs; and it holds no zone level range. So the quest-giver and vendor cases check that the advisor says what it
// cannot name and names none, a case that gives a giver or a vendor composes the Questie rows or the vendor name under
// synthetic names, and the cases the dataset cannot answer check that the advisor says so and states none. A difference
// the dataset's own rows cause is marked "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	npcGiverSrc  = "dataset quests_observed (giver_npc_id, turn_in_npc_id), npcs (no row for any giver); Wowhead Classic quest pages"
	npcQuestieSr = "player's QuestieDB quest starters and finishers, composed under synthetic names; dataset quests_observed (title, level)"
	npcVendorSrc = "dataset vendor_sources (npc_id, price_copper), npcs (no vendor has a name row); Wowhead Classic item pages"
	npcSkillSrc  = "dataset recipes (learn_skill, learn_skill_source); Warcraft Wiki Smelting and First Aid pages (skill required)"
	npcNodeSrc   = "dataset gathering_nodes (skill_required, zone_name), loot_sources (object); Warcraft Wiki Mining and Herbalism node tables"
	npcZoneSrc   = "dataset instances (min_level, max_level, entrance_zone_name); the dataset has no zone level range"
	npcNoneSrc   = "the dataset has no such row, column or table"
)

// ---- quest givers and turn-ins ----

// npcQuests are real quests of quests_observed whose giver creature the dataset knows by id only.
var npcQuests = []struct {
	id    int64
	guide string
}{
	{890, "Wowhead: ends with Wharfmaster Dizzywig"}, {529, "Wowhead: ends with Darthalia in Tarren Mill"}, {1489, "a guide names its giver"},
	{6563, "a guide names its giver"}, {8366, "a guide names its giver"}, {879, "a guide names its giver"}, {1109, "a guide names its giver"}, {2, "a guide names its giver; the dataset holds no giver id"},
}

// TestOfflineBenchmarkQuestGiverNamed: a quest the dataset knows a giver id for but no giver name: the advisor names no giver or turn-in
// and says the data does not name them, never a creature id.
func TestOfflineBenchmarkQuestGiverNamed(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	for _, q := range npcQuests {
		row := qbQuest(t, q.id)
		giver, turn := num(row["giver_npc_id"]), num(row["turn_in_npc_id"])
		_, npcErr := ds.Get("npcs", giver)
		f := quests.FactsOf(ds, nil, q.id)
		txt := quests.Render(quests.OfWith(ds, nil, q.id), "")
		stated := strings.Contains(txt, "doesn't name who gives it or who takes it in")
		bare := strings.Contains(txt, fmt.Sprint(giver)+")") && giver > 0 && !strings.Contains(txt, "quest "+fmt.Sprint(q.id))
		ref := fmt.Sprintf("giver_npc_id %d (npcs row: %v), turn_in_npc_id %d; %s", giver, npcErr == nil, turn, q.guide)
		ok := f != nil && f.Giver == "" && f.TurnIn == "" && stated && !bare && f.Level == int(num(row["level"]))
		res := "differ (data): the dataset holds a giver id and no npcs row, so the advisor names none"
		if giver == 0 {
			res = "differ (data): the dataset holds no giver id, so the advisor names none"
		}
		if !ok {
			res = "differ: a giver was named or invented, or the advisor did not say the data names none"
		}
		out = append(out, benchRow{fmt.Sprintf("%s (quest %d): who gives it and who takes it in", f.Name, q.id), firstLine(strings.TrimPrefix(txt, "Seen by players on Everloot (CC BY-SA): ")), ref, npcGiverSrc, res})
	}
	finish(t, "advice-quest-giver-named", "Offline quest givers: the dataset names none", out)
}

// npcIndex is a QuestieDB of six quest shapes: one giver and one taker, two givers, the same NPC for both, an object that starts it,
// an object that ends it, and a taker in the giver's zone.
func npcIndex() *questie.Index {
	at := func(name string, area int) questie.Entity {
		return questie.Entity{Name: name, Spawns: []questie.Spawn{{Area: area, X: 50, Y: 50}}}
	}
	npc, obj := func(id int) questie.Ref { return questie.Ref{ID: id} }, func(id int) questie.Ref { return questie.Ref{Object: true, ID: id} }
	q := func(id int, name string, start, end []questie.Ref) *questie.Quest {
		return &questie.Quest{ID: id, Name: name, Starters: start, Finishers: end}
	}
	return &questie.Index{
		Areas:   map[int]questie.Area{1: {Zone: "Synthetic Vale"}, 2: {Zone: "Synthetic Cove"}},
		Npcs:    map[int]questie.Entity{1: at("Synthetic Giver", 1), 2: at("Synthetic Taker", 2), 3: at("Synthetic Second Giver", 2)},
		Objects: map[int]questie.Entity{1: at("Synthetic Notice Board", 1)},
		Quests: map[int]*questie.Quest{
			890:  q(890, "The Missing Shipment", []questie.Ref{npc(1)}, []questie.Ref{npc(2)}),
			529:  q(529, "Battle of Hillsbrad", []questie.Ref{npc(1), npc(3)}, []questie.Ref{npc(3)}),
			1489: q(1489, "Hamuul Runetotem", []questie.Ref{npc(1)}, []questie.Ref{npc(1)}),
			6563: q(6563, "The Essence of Aku'Mai", []questie.Ref{obj(1)}, []questie.Ref{npc(2)}),
			8366: q(8366, "Southsea Shakedown", []questie.Ref{npc(3)}, []questie.Ref{obj(1)}),
			879:  q(879, "Betrayal from Within", []questie.Ref{npc(2)}, []questie.Ref{npc(1)}),
		},
	}
}

// TestOfflineBenchmarkQuestGiverQuestie: where the player's QuestieDB names a giver and a taker, the advisor names both with their zone, keeps the
// dataset's level, and words a giver that is also the taker once for each.
func TestOfflineBenchmarkQuestGiverQuestie(t *testing.T) {
	ds, ix := qbStore(), npcIndex()
	want := map[int64][2]string{
		890:  {"Synthetic Giver (Synthetic Vale)", "Synthetic Taker (Synthetic Cove)"},
		529:  {"Synthetic Giver (Synthetic Vale) or Synthetic Second Giver (Synthetic Cove)", "Synthetic Second Giver (Synthetic Cove)"},
		1489: {"Synthetic Giver (Synthetic Vale)", "Synthetic Giver (Synthetic Vale)"},
		6563: {"Synthetic Notice Board (Synthetic Vale)", "Synthetic Taker (Synthetic Cove)"},
		8366: {"Synthetic Second Giver (Synthetic Cove)", "Synthetic Notice Board (Synthetic Vale)"},
		879:  {"Synthetic Taker (Synthetic Cove)", "Synthetic Giver (Synthetic Vale)"},
	}
	var out []benchRow
	for _, id := range []int64{890, 529, 1489, 6563, 8366, 879} {
		f := quests.FactsOf(ds, ix, id)
		txt := quests.Render(quests.OfWith(ds, ix, id), "")
		w := want[id]
		lvl := int(num(qbQuest(t, id)["level"]))
		ok := f != nil && f.Giver == w[0] && f.TurnIn == w[1] && f.Level == lvl && strings.Contains(txt, "starts with "+w[0]) && strings.Contains(txt, "turn in to "+w[1]) && !strings.Contains(txt, "doesn't name")
		out = append(out, benchRow{fmt.Sprintf("%s (quest %d): giver and turn-in", f.Name, id), firstLine(strings.TrimPrefix(txt, "Seen by players on Everloot (CC BY-SA): ")), fmt.Sprintf("starters: %s; finishers: %s; level %d", w[0], w[1], lvl), npcQuestieSr,
			resultIf(ok, "another giver, turn-in or level")})
	}
	finish(t, "advice-quest-giver-questie", "Offline quest givers: named from the player's QuestieDB", out)
}

// ---- vendors ----

var npcVendorItems = []string{"Refreshing Spring Water", "Simple Wood", "Coarse Thread", "Empty Vial", "Weak Flux", "Salt", "Mild Spices", "Soothing Spices"}

// TestOfflineBenchmarkVendorUnnamed: an item the dataset lists vendors for and names none of: the advisor says a vendor sells it and that it cannot say
// which, never "not worth chasing" and never a vendor id.
func TestOfflineBenchmarkVendorUnnamedA(t *testing.T) { vendorUnnamed(t, "a", npcVendorItems[:4]) }
func TestOfflineBenchmarkVendorUnnamedB(t *testing.T) { vendorUnnamed(t, "b", npcVendorItems[4:]) }

func vendorUnnamed(t *testing.T, part string, items []string) {
	ds := qbStore()
	var out []benchRow
	for _, n := range items {
		it := rowByName(t, ds, "items", n)
		id := num(it["id"])
		rows, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		named := 0
		for _, r := range rows {
			if npc, err := ds.Get("npcs", num(r["npc_id"])); err == nil && text(npc["name"]) != "" {
				named++
			}
		}
		res := paths.For(horde(40), ds, id, paths.Options{})
		txt := res.Text()
		ok := len(rows) > 0 && named == 0 && !strings.Contains(txt, "not worth chasing") && strings.Contains(res.Reason+txt, "vendors sell it") && !strings.Contains(txt, "Buy from")
		out = append(out, benchRow{n + ": a vendor that sells it", txt, fmt.Sprintf("%d vendor_sources rows, %d vendors with a name", len(rows), named), npcVendorSrc, resultIf(ok, "called not worth chasing, or no statement that vendors sell it")})
		out = append(out, benchRow{n + ": which vendor", "none named", "a vendor in a town (Wowhead)", npcVendorSrc, "differ (data): no vendor of vendor_sources has a name row in npcs"})
	}
	finish(t, "advice-vendor-unnamed-"+part, "Offline vendors: sold by vendors the dataset does not name ("+part+")", out)
}

// TestOfflineBenchmarkVendorNamed: a vendor that has a name is named with its zone and the price a player with no standing pays (the item's buy price: a lower row
// is a reporter's reputation discount), the lowest creature id breaking a tie.
func TestOfflineBenchmarkVendorNamed(t *testing.T) {
	ds := mbNamed()
	var out []benchRow
	for _, n := range []string{"Refreshing Spring Water", "Simple Wood", "Salt", "Weak Flux", "Coarse Thread"} {
		id := num(rowByName(t, ds, "items", n)["id"])
		rows, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: id, Max: id})
		it := rowByName(t, ds, "items", n)
		low, npc := int64(1<<62), int64(1<<62)
		for _, r := range rows {
			if p := num(r["price_copper"]); p < low {
				low = p
			}
		}
		base := max(num(it["buy_price"]), low) // a row below the buy price is a discount
		for _, r := range rows {
			if p := max(num(r["price_copper"]), base); p == base && num(r["npc_id"]) < npc {
				npc = num(r["npc_id"])
			}
		}
		o, ok := vendors.Best(ds, id)
		out = append(out, benchRow{n + ": the vendor and its price", o.Line(), fmt.Sprintf("buy price %s (lowest row %s): Synthetic Vendor %d", vendors.Money(base), vendors.Money(low), npc), npcVendorSrc,
			resultIf(ok && o.NPCID == npc && o.PriceCopper == base && o.Zone == "Synthetic Vale", "another vendor or price")})
	}
	finish(t, "advice-vendor-named", "Offline vendors: a named vendor and its lowest price", out)
}

// ---- profession skill gates ----

type npcRecipe struct {
	name, prof string
	guide      int64
}

var npcRecipes = []npcRecipe{
	{"Heavy Linen Bandage", "First Aid", 40}, {"Smelt Tin", "Mining", 50}, {"Smelt Bronze", "Mining", 50}, {"Smelt Silver", "Mining", 65}, {"Smelt Iron", "Mining", 100}, {"Smelt Gold", "Mining", 115},
	{"Wool Bandage", "First Aid", 80}, {"Heavy Wool Bandage", "First Aid", 115}, {"Silk Bandage", "First Aid", 150}, {"Heavy Silk Bandage", "First Aid", 180},
	{"Heavy Mageweave Bandage", "First Aid", 240}, {"Runecloth Bandage", "First Aid", 260},
}

// TestOfflineBenchmarkRecipeSkillGate: the skill a recipe needs is the dataset's learn skill, worded "about" when the dataset only estimates it; a difference from
// the guides is the dataset's.
func TestOfflineBenchmarkRecipeSkillGateA(t *testing.T) { recipeSkillGate(t, "a", npcRecipes[:6]) }
func TestOfflineBenchmarkRecipeSkillGateB(t *testing.T) { recipeSkillGate(t, "b", npcRecipes[6:]) }

func recipeSkillGate(t *testing.T, part string, cases []npcRecipe) {
	ds := tsStore()
	var out []benchRow
	for _, r := range cases {
		rec := recipeNamed(t, ds, r.name, r.prof)
		res, err := learn.Find(horde(40), ds, nil, r.name)
		need, src := num(rec["learn_skill"]), text(rec["learn_skill_source"])
		about := strings.Contains(res.Gate.Line, "about")
		ok := err == nil && res.Profession == r.prof && int64(res.Gate.Skill) == need && about == (src == "estimate")
		out = append(out, benchRow{r.name + ": the skill it needs", res.Gate.Line, fmt.Sprintf("dataset %s %d (%s); guide %d", r.prof, need, src, r.guide), npcSkillSrc, resultIf(ok, "another skill, or an estimate worded as exact")})
		diff := "match"
		if need != r.guide {
			diff = fmt.Sprintf("differ (data): the dataset's learn skill (%s) is %d, the guide's %d", src, need, r.guide)
		}
		out = append(out, benchRow{r.name + ": the skill in the guides", fmt.Sprintf("%s %d", r.prof, int64(res.Gate.Skill)), fmt.Sprintf("Warcraft Wiki: %s %d", r.prof, r.guide), npcSkillSrc, diff})
	}
	finish(t, "advice-recipe-skill-gate-"+part, "Offline profession skill gates: the skill a recipe needs ("+part+")", out)
}

type npcNode struct {
	label, reagent, prof string
	craft                int64
	skill                int
	nodeNeeds            int
	node                 string
}

var npcNodes = []npcNode{
	{"Tin Ore at Mining 40", "Tin Ore", "Mining", 3576, 40, 65, "Tin Vein"},
	{"Tin Ore at Mining 70", "Tin Ore", "Mining", 3576, 70, 65, "Tin Vein"},
	{"Copper Ore at Mining 1", "Copper Ore", "Mining", 2840, 1, 1, "Copper Vein"},
	{"Coarse Stone at Mining 50", "Coarse Stone", "Mining", 3478, 50, 65, "Tin Vein"},
	{"Earthroot at Herbalism 10", "Earthroot", "Herbalism", 2454, 10, 15, "Earthroot"},
	{"Briarthorn at Herbalism 60", "Briarthorn", "Herbalism", 5631, 60, 70, "Briarthorn"},
	{"Briarthorn at Herbalism 75", "Briarthorn", "Herbalism", 2459, 75, 70, "Briarthorn"},
}

// TestOfflineBenchmarkGatherSkillGate: a gathering node whose skill the character lacks is not offered, and the answer says the skill it needs and the character's own;
// one within the skill is offered with its zone.
func TestOfflineBenchmarkGatherSkillGateA(t *testing.T) { gatherSkillGate(t, "a", npcNodes[:4]) }
func TestOfflineBenchmarkGatherSkillGateB(t *testing.T) { gatherSkillGate(t, "b", npcNodes[4:]) }

func gatherSkillGate(t *testing.T, part string, cases []npcNode) {
	ds := tsStore()
	var out []benchRow
	for _, n := range cases {
		profs := map[string]int{n.prof: n.skill, "Alchemy": 100, "Blacksmithing": 100}
		g, _ := mbReagent(t, mbChar(30, "Horde", profs), ds, n.craft, n.reagent)
		nodes, _ := ds.Filter("gathering_nodes", store.Range{Column: "name", Min: n.node, Max: n.node})
		dataSkill := int64(0)
		if len(nodes) > 0 {
			dataSkill = num(nodes[0]["skill_required"])
		}
		offered := mbKind(g.How) == "gather"
		var ok bool
		if n.skill >= n.nodeNeeds {
			ok = offered && strings.Contains(g.How, n.node) && strings.Contains(g.How, " in ")
		} else {
			ok = !offered && strings.Contains(g.How, fmt.Sprintf("gathering it needs %s %d; yours is %d", n.prof, n.nodeNeeds, n.skill))
		}
		ok = ok && dataSkill == int64(n.nodeNeeds)
		out = append(out, benchRow{n.label + ": the node and the skill", g.How, fmt.Sprintf("%s needs %s %d (gathering_nodes %d)", n.node, n.prof, n.nodeNeeds, dataSkill), npcNodeSrc, resultIf(ok, "the node, its zone or its skill")})
	}
	finish(t, "advice-gather-skill-gate-"+part, "Offline profession skill gates: the skill a gathering node needs ("+part+")", out)
}

// ---- zone level fit ----

type npcZone struct {
	name  string
	level int
}

var npcZones = []npcZone{
	{"Deadmines", 10}, {"Deadmines", 20}, {"Deadmines", 40}, {"Wailing Caverns", 45}, {"Ragefire Chasm", 30}, {"Scarlet Monastery", 60}, {"Shadowfang Keep", 24}, {"Blackfathom Deeps", 22},
}

// TestOfflineBenchmarkZoneLevelFit: a dungeon asked about by name says whether the character is under its range (how many levels to go), inside it, or far over
// it, in the zone its entrance stands in, by the dataset's row.
func TestOfflineBenchmarkZoneLevelFitA(t *testing.T) { zoneLevelFit(t, "a", npcZones[:4]) }
func TestOfflineBenchmarkZoneLevelFitB(t *testing.T) { zoneLevelFit(t, "b", npcZones[4:]) }

func zoneLevelFit(t *testing.T, part string, cases []npcZone) {
	ds := instStore()
	var out []benchRow
	for _, z := range cases {
		row := rowByName(t, ds, "instances", z.name)
		lo, hi, zone := num(row["min_level"]), num(row["max_level"]), text(row["entrance_zone_name"])
		lv := z.level
		c := character.Character{Faction: "Horde", Class: "WARRIOR", Level: &lv}
		p, togo, err := dungeons.Named(c, ds, nil, z.name)
		if err != nil || p == nil {
			t.Fatalf("%s: %v", z.name, err)
		}
		txt := dungeons.RenderNamed(c, *p, togo)
		var want, fit string
		switch {
		case int64(lv) < lo:
			want, fit = fmt.Sprintf("under the range: %d levels to go", lo-int64(lv)), fmt.Sprintf("it opens around %d", lo)
		case int64(lv) > hi+dungeons.LevelWindow:
			want, fit = fmt.Sprintf("over the range: it tops out at %d", hi), fmt.Sprintf("it tops out around %d, so it is well below your level", hi)
		default:
			want = "inside the range or within three levels over it"
		}
		got := "no fit line"
		for _, l := range strings.Split(txt, "\n") {
			if strings.HasPrefix(l, "You're") {
				got = l
			}
		}
		ok := togo == max(int(lo)-lv, 0) && strings.Contains(txt, "entrance in "+zone) && (fit == "" && got == "no fit line" || fit != "" && strings.Contains(got, fit))
		out = append(out, benchRow{fmt.Sprintf("%s at level %d: does it fit", z.name, lv), got, fmt.Sprintf("instances %d-%d, entrance %s: %s", lo, hi, zone, want), npcZoneSrc, resultIf(ok, "no or another statement of the fit, or another zone")})
	}
	finish(t, "advice-zone-level-fit-"+part, "Offline zone level fit: a dungeon's range against the character's level ("+part+")", out)
}

// ---- no answer in the dataset ----

// TestOfflineBenchmarkNpcNoAnswer: what the dataset cannot answer: a zone's level range, a quest it lacks, a vendor for an item no vendor sells, an item it lacks, a
// node with no skill row and a recipe it lacks. The advisor says so and states none.
func TestOfflineBenchmarkNpcNoAnswer(t *testing.T) {
	ds := qbStore()
	var out []benchRow
	_, err := ds.Filter("zones", store.Range{Column: "id", Min: int64(0)})
	p, _, _ := dungeons.Named(horde(10), instStore(), nil, "Elwynn Forest")
	out = append(out, benchRow{"Elwynn Forest: its level range", fmt.Sprintf("table error %v; dungeon found %v", err != nil, p != nil), "levels 1-10 (Warcraft Wiki); no zone level table", npcNoneSrc,
		resultIf(err != nil && p == nil, "a zone range was invented or a zone taken for a dungeon")})
	id, name, cands := quests.Resolve(ds, "The Rise of Thunderfury")
	txt := quests.Render(quests.OfWith(ds, nil, 999999), "")
	out = append(out, benchRow{"a quest the dataset lacks", fmt.Sprintf("id %d, %q, %d candidates; %s", id, name, len(cands), firstLine(txt)), "no quests_observed row", npcNoneSrc,
		resultIf(id == 0 && len(cands) == 0 && strings.HasPrefix(txt, "No quest line or follow-up quest is listed"), "a quest was invented")})
	f := quests.FactsOf(ds, nil, 2)
	t2 := quests.Render(quests.OfWith(ds, nil, 2), "")
	out = append(out, benchRow{"Sharptalon's Claw: a quest with no giver id", firstLine(t2), "quests_observed giver_npc_id empty", npcNoneSrc,
		resultIf(f != nil && f.Giver == "" && strings.Contains(t2, "doesn't name who gives it"), "a giver was named")})
	for _, n := range []string{"Hammer of the Northern Wind", "Azure Silk Vest"} {
		it := rowByName(t, ds, "items", n)
		vend, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: num(it["id"]), Max: num(it["id"])})
		res := paths.For(horde(40), ds, num(it["id"]), paths.Options{})
		bad := strings.Contains(res.Text(), "Buy from") || strings.Contains(res.Text(), "vendors sell it")
		out = append(out, benchRow{n + ": a vendor that sells it", firstLine(res.Text()), fmt.Sprintf("%d vendor_sources rows", len(vend)), npcNoneSrc, resultIf(len(vend) == 0 && !bad, "a vendor was named for an item no vendor sells")})
	}
	fid, fname, fc := farm.Resolve(ds, "Thunderfury, Blessed Blade of the Windseeker")
	out = append(out, benchRow{"an item the dataset lacks", fmt.Sprintf("id %d, %q, %d candidates", fid, fname, len(fc)), "no items row", npcNoneSrc, resultIf(fid == 0 && len(fc) == 0, "an item was matched")})
	g, _ := mbReagent(t, mbChar(30, "Horde", map[string]int{"Alchemy": 100, "Herbalism": 10}), tsStore(), 2455, "Mageroyal")
	out = append(out, benchRow{"Mageroyal at Herbalism 10: the node's skill", g.How, "no gathering_nodes row for its node: no skill; the game needs Herbalism 50 (Warcraft Wiki)", npcNodeSrc,
		"differ (data): the dataset has no skill for the node, so the advisor states none"})
	_, rerr := learn.Find(horde(40), tsStore(), nil, "Gurubashi Gumbo")
	out = append(out, benchRow{"Gurubashi Gumbo: the skill it needs", fmt.Sprint(rerr), "no recipes row", npcNoneSrc, resultIf(rerr != nil, "a recipe was invented")})
	finish(t, "advice-npc-no-answer", "Offline quest giver, vendor, skill gate and zone fit: no answer in the dataset", out)
}
