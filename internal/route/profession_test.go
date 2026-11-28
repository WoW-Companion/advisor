package route

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

type pfNote struct {
	Section  string  `parquet:"section"`
	Subject  string  `parquet:"subject"`
	Related  *string `parquet:"related,optional"`
	SkillMin *int16  `parquet:"skill_min,optional"`
	SkillCap *int16  `parquet:"skill_cap,optional"`
	MinLevel *int16  `parquet:"min_level,optional"`
	Text     string  `parquet:"text"`
}

type pfRecipe struct {
	SpellID       int32   `parquet:"spell_id"`
	SkillLineName string  `parquet:"skill_line_name"`
	Name          *string `parquet:"name,optional"`
	LearnSkill    *int16  `parquet:"learn_skill,optional"`
	YellowSkill   *int16  `parquet:"yellow_skill,optional"`
	GreySkill     *int16  `parquet:"grey_skill,optional"`
	LearnedFrom   string  `parquet:"learned_from"`
}

type pfReagent struct {
	SpellID int32 `parquet:"spell_id"`
	Item    int32 `parquet:"reagent_item_id"`
	Count   int32 `parquet:"count"`
}

type pfItem struct {
	ID    int64  `parquet:"id"`
	Name  string `parquet:"name"`
	Price int64  `parquet:"buy_price"`
}

type pfNode struct {
	NodeID int32  `parquet:"node_id"`
	Name   string `parquet:"name"`
	Prof   string `parquet:"profession"`
	Skill  *int16 `parquet:"skill_required,optional"`
	Zone   string `parquet:"zone_name"`
}

func pfTable[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func pi16(n int16) *int16   { return &n }
func pstr(s string) *string { return &s }

// profData is a small dataset: three ranks, a First Aid recipe, a Herbalism node and a few crafting professions.
func profData(t *testing.T) store.DataStore { return profDataWith(t, []pfReagent{{500, 7, 1}}) }

func profDataWith(t *testing.T, reagents []pfReagent) store.DataStore {
	return store.New(profTables(t, reagents))
}

// profTables are profData's tables, for a test that adds more.
func profTables(t *testing.T, reagents []pfReagent) store.ReaderSource {
	rank := func(name string, lo, hi, lv int16) pfNote {
		return pfNote{"ranks", name, nil, pi16(lo), pi16(hi), pi16(lv), name}
	}
	recipes := []pfRecipe{{500, "First Aid", pstr("Heavy Linen Bandage"), pi16(40), pi16(80), pi16(110), "trainer"}}
	for i, p := range []string{"Tailoring", "Blacksmithing", "Engineering", "Leatherworking"} {
		recipes = append(recipes, pfRecipe{int32(600 + i), p, pstr(p + " Thing"), pi16(1), pi16(40), pi16(60), "trainer"})
	}
	return store.ReaderSource{
		"profession_notes": pfTable(t, []pfNote{rank("Apprentice", 1, 75, 5), rank("Journeyman", 75, 150, 10), rank("Expert", 150, 225, 20)}),
		"recipes":          pfTable(t, recipes),
		"recipe_reagents":  pfTable(t, reagents),
		"items":            pfTable(t, []pfItem{{7, "Heavy Linen Cloth", 100}, {8, "Rune Thread", 100}}),
		"gathering_nodes":  pfTable(t, []pfNode{{1, "Bruiseweed", "Herbalism", pi16(1), "Durotar"}, {2, "Peacebloom", "Herbalism", pi16(1), "Mulgore"}}),
	}
}

// profWorld is Orgrimmar (map 7001) with the town's quest givers and a Tailoring trainer.
func profWorld() (*questie.Index, character.Character, Options) {
	ix := townIndex()
	ix.Areas[901] = questie.Area{UIMap: 7001, Zone: "Orgrimmar"}
	ix.Npcs[50] = questie.Entity{Name: "Tess the Tailor", Sub: "Tailoring Trainer", Flags: questie.FlagTrainer | questie.FlagProfTrainer, Spawns: []questie.Spawn{{Area: 901, X: 40, Y: 40}}}
	c := townHero(0)
	c.Faction, c.Class, c.BankCaptured = "Horde", "ROGUE", true
	c.Location.Zone = "Orgrimmar"
	lvl := 25
	c.Level = &lvl
	return ix, c, townOpt()
}

func profErrands(p Plan) []Errand {
	var out []Errand
	for _, e := range p.Errands {
		if e.Kind == ErrandProfession {
			out = append(out, e)
		}
	}
	return out
}

func skillOf(name string, rank, max int) character.Skill {
	return character.Skill{Name: name, Rank: &rank, Max: &max}
}

func TestProfessionTrainerDueAndPlaceable(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profData(t)
	c.Professions = []character.Skill{skillOf("Tailoring", 150, 150)}
	es := profErrands(BuildWith(c, ix, opt))
	if len(es) != 1 || es[0].Label != "Train Tailoring (Expert) with Tess the Tailor" || es[0].N == 0 || es[0].Attach != 0 {
		t.Fatalf("errands %+v", es)
	}
}

func TestProfessionTrainerDueNoneInHub(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profData(t)
	c.Professions = []character.Skill{skillOf("Tailoring", 150, 150)}
	delete(ix.Npcs, 50)
	if es := profErrands(BuildWith(c, ix, opt)); len(es) != 0 {
		t.Fatalf("errands %+v", es)
	}
	ix.Npcs[50] = questie.Entity{Name: "Far Tailor", Sub: "Tailoring Trainer", Flags: questie.FlagTrainer, Spawns: []questie.Spawn{{Area: 902, X: 40, Y: 40}}}
	ix.Areas[902] = questie.Area{UIMap: 7002, Zone: "Durotar"}
	if es := profErrands(BuildWith(c, ix, opt)); len(es) != 0 {
		t.Fatalf("a trainer outside the hub: %+v", es)
	}
}

func TestProfessionVendorReagentOnTheWay(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profData(t)
	c.Professions = []character.Skill{skillOf("First Aid", 60, 75)}
	c.Seen = &character.Observed{Vendors: []character.SeenVendor{{NPC: 9, Name: "Cloth Clara", Map: 7001, X: 71, Y: 70,
		Items: []character.SeenStock{{ItemID: 7, Price: 100}}}}}
	e := attached(BuildWith(c, ix, opt), ErrandRestock)
	if e == nil || e.Attach == 0 || !strings.HasPrefix(e.Label, "buy ") || !strings.HasSuffix(e.Label, " Heavy Linen Cloth from Cloth Clara") || !strings.HasPrefix(e.Why, "for your First Aid") {
		t.Fatalf("restock %+v", e)
	}
}

func TestProfessionGatheringInATripZone(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profData(t)
	c.Professions = []character.Skill{skillOf("Herbalism", 60, 75)}
	ix.Areas[902] = questie.Area{UIMap: 7002, Zone: "Durotar"}
	// A trip into Durotar, and a run that has a stop there; a second trip with no node of the skill gets nothing.
	p := &Plan{Steps: []Stop{{N: 1, Kind: KindTurnIn, Map: 7001, X: 10, Y: 10}, {N: 2, Kind: KindTurnIn, Map: 7002, X: 50, Y: 50}},
		Trips: []Trip{{Order: 1, Map: 7002, Zone: "Durotar"}, {Order: 2, Map: 7001, Zone: "Orgrimmar"}}}
	addProfessionChores(p, c, &questieSource{qx: ix, opt: opt}, opt)
	es := profErrands(*p)
	if len(es) != 1 || es[0].Label != "Gather Bruiseweed here (skill-ups to 75)" || es[0].Attach != 2 {
		t.Fatalf("errands %+v", p.Errands)
	}
}

func TestProfessionCraftBatchAtTheFirstCityStop(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profData(t)
	c.Professions = []character.Skill{skillOf("First Aid", 60, 75)}
	c.Bags = []character.ItemStack{{ItemID: 7, Count: 200, Name: "Heavy Linen Cloth"}}
	c.BagsCaptured = true
	p := BuildWith(c, ix, opt)
	es := profErrands(p)
	if len(es) != 1 || !strings.HasPrefix(es[0].Label, "Craft ") || !strings.Contains(es[0].Label, " Heavy Linen Bandage (to ") || es[0].Attach != 1 {
		t.Fatalf("errands %+v", es)
	}
	if attached(p, ErrandRestock) != nil {
		t.Errorf("nothing is missing, so nothing to buy")
	}
}

func TestProfessionNoProfessionsNoLines(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profData(t)
	p := BuildWith(c, ix, opt)
	if es := profErrands(p); len(es) != 0 || attached(p, ErrandRestock) != nil {
		t.Fatalf("errands %+v", p.Errands)
	}
}

func TestProfessionLinesCapAtThree(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profData(t)
	for i, p := range []string{"Blacksmithing", "Engineering", "Leatherworking"} {
		ix.Npcs[70+i] = questie.Entity{Name: p + " Trainer", Sub: p + " Trainer", Flags: questie.FlagTrainer, Spawns: []questie.Spawn{{Area: 901, X: 30 + float64(i), Y: 30}}}
	}
	c.Professions = []character.Skill{skillOf("Tailoring", 150, 150), skillOf("Blacksmithing", 150, 150), skillOf("Engineering", 150, 150), skillOf("Leatherworking", 150, 150)}
	if es := profErrands(BuildWith(c, ix, opt)); len(es) != 3 {
		t.Fatalf("want 3 profession errands, got %+v", es)
	}
}

func restocks(p Plan) []Errand {
	var out []Errand
	for _, e := range p.Errands {
		if e.Kind == ErrandRestock {
			out = append(out, e)
		}
	}
	return out
}

func TestRestockPathBandRecipeWithAllReagentsSoldIsOneLine(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profData(t)
	c.Professions = []character.Skill{skillOf("First Aid", 60, 75)}
	c.Seen = &character.Observed{Vendors: []character.SeenVendor{{NPC: 9, Name: "Cloth Clara", Map: 7001, X: 71, Y: 70,
		Items: []character.SeenStock{{ItemID: 7, Price: 100}}}}}
	es := restocks(BuildWith(c, ix, opt))
	if len(es) != 1 || !strings.Contains(es[0].Why, "Heavy Linen Bandage") {
		t.Fatalf("restock lines %+v", es)
	}
}

func TestRestockPathRecipeWithAnUnsoldReagentIsNoLine(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profDataWith(t, []pfReagent{{500, 7, 1}, {500, 8, 1}})
	c.Professions = []character.Skill{skillOf("First Aid", 60, 75)}
	c.Seen = &character.Observed{Vendors: []character.SeenVendor{{NPC: 9, Name: "Cloth Clara", Map: 7001, X: 71, Y: 70,
		Items: []character.SeenStock{{ItemID: 7, Price: 100}}}}}
	if es := restocks(BuildWith(c, ix, opt)); len(es) != 0 {
		t.Fatalf("half a craft was listed: %+v", es)
	}
}

func TestRestockNoLineTwiceAcrossRestockAndPath(t *testing.T) {
	ix, c, opt := profWorld()
	opt.Data = profData(t)
	c.Professions = []character.Skill{skillOf("First Aid", 60, 75)}
	c.Recipes = map[string]character.SeenRecipes{"poisons": {Entries: []character.SeenRecipe{
		{Name: "Instant Poison", Difficulty: "yellow", Reagents: []character.ItemStack{{ItemID: 7, Count: 1, Name: "Heavy Linen Cloth"}}}}}}
	c.Seen = &character.Observed{Vendors: []character.SeenVendor{{NPC: 9, Name: "Cloth Clara", Map: 7001, X: 71, Y: 70,
		Items: []character.SeenStock{{ItemID: 7, Price: 100}}}}}
	es := restocks(BuildWith(c, ix, opt))
	if len(es) != 1 || !strings.HasPrefix(es[0].Why, "for Instant Poison") {
		t.Fatalf("restock lines %+v", es)
	}
}

type pfTier struct {
	Profession string  `parquet:"profession"`
	Tier       string  `parquet:"tier"`
	How        string  `parquet:"how"`
	Faction    *string `parquet:"faction,optional"`
	Trainer    *string `parquet:"trainer_name,optional"`
	Place      *string `parquet:"place,optional"`
}

type pfStep struct {
	Profession  string `parquet:"profession"`
	Step        int16  `parquet:"step"`
	Option      int8   `parquet:"option"`
	Kind        string `parquet:"kind"`
	From        int16  `parquet:"skill_from"`
	To          int16  `parquet:"skill_to"`
	Spell       int32  `parquet:"recipe_spell_id"`
	Recipe      string `parquet:"recipe_name"`
	Crafts      int16  `parquet:"crafts"`
	Materials   string `parquet:"materials"`
	LearnedFrom string `parquet:"learned_from"`
}

type pfOffer struct {
	Npc  int32 `parquet:"npc_id"`
	Item int32 `parquet:"item_id"`
}

func TestProfessionTrainerRankTaughtInOtherCitiesOnly(t *testing.T) {
	ix, c, opt := profWorld()
	tables := profTables(t, []pfReagent{{500, 7, 1}})
	tables["profession_tiers"] = pfTable(t, []pfTier{
		{"Tailoring", "Expert", "trainer", pstr("Horde"), pstr("Sera of Undercity"), pstr("Undercity")},
		{"Tailoring", "Expert", "trainer", pstr("Alliance"), pstr("Aldo"), pstr("Ironforge")},
	})
	opt.Data = store.New(tables)
	c.Professions = []character.Skill{skillOf("Tailoring", 150, 150)}
	// Tess teaches the lower ranks in Orgrimmar: the Expert rank is trained in Undercity, so no line here
	if es := profErrands(BuildWith(c, ix, opt)); len(es) != 0 {
		t.Fatalf("a trainer of another rank was named: %+v", es)
	}
	// an Alliance row does not count for a Horde character, and a rank the table lists for Orgrimmar does
	tables["profession_tiers"] = pfTable(t, []pfTier{{"Tailoring", "Expert", "trainer", pstr("Horde"), pstr("Tess the Tailor"), pstr("Orgrimmar")}})
	opt.Data = store.New(tables)
	es := profErrands(BuildWith(c, ix, opt))
	if len(es) != 1 || es[0].Label != "Train Tailoring (Expert) with Tess the Tailor" {
		t.Fatalf("errands %+v", es)
	}
}

func TestProfessionTrainerNamedByTheDatasetComesFirst(t *testing.T) {
	ix, c, opt := profWorld()
	ix.Npcs[51] = questie.Entity{Name: "Nearby Tailor", Sub: "Tailoring Trainer", Flags: questie.FlagTrainer | questie.FlagProfTrainer, Spawns: []questie.Spawn{{Area: 901, X: 48, Y: 49}}}
	tables := profTables(t, []pfReagent{{500, 7, 1}})
	tables["profession_tiers"] = pfTable(t, []pfTier{{"Tailoring", "Expert", "trainer", pstr("Horde"), pstr("Tess the Tailor"), pstr("Orgrimmar")}})
	opt.Data = store.New(tables)
	c.Professions = []character.Skill{skillOf("Tailoring", 150, 150)}
	es := profErrands(BuildWith(c, ix, opt))
	if len(es) != 1 || es[0].Label != "Train Tailoring (Expert) with Tess the Tailor" {
		t.Fatalf("the nearer trainer was named: %+v", es)
	}
}

func TestProfessionTrainRecipeIsTheGuideStepsRecipe(t *testing.T) {
	ix, c, opt := profWorld()
	tables := profTables(t, []pfReagent{{500, 7, 1}})
	tables["profession_routes"] = pfTable(t, []pfStep{
		{"Tailoring", 1, 0, "craft", 1, 40, 700, "Bolt of Guide", 10, "[]", "trainer"},
		{"Tailoring", 2, 0, "craft", 40, 75, 701, "Vendor Recipe", 10, "[]", "vendor"},
	})
	opt.Data = store.New(tables)
	c.Professions = []character.Skill{skillOf("Tailoring", 20, 225)}
	c.KnownSpells = map[int64]bool{1: true}
	es := profErrands(BuildWith(c, ix, opt))
	if len(es) != 1 || es[0].Label != "Train Tailoring (Bolt of Guide) with Tess the Tailor" {
		t.Fatalf("errands %+v", es)
	}
	c.KnownSpells[700] = true
	if es := profErrands(BuildWith(c, ix, opt)); len(es) != 0 {
		t.Fatalf("a known recipe is trained again: %+v", es)
	}
	delete(c.KnownSpells, 700)
	c.Professions = []character.Skill{skillOf("Tailoring", 50, 225)}
	if es := profErrands(BuildWith(c, ix, opt)); len(es) != 0 {
		t.Fatalf("a vendor's recipe is sent to the trainer: %+v", es)
	}
}

func TestRestockVendorInTownAmongManyElsewhere(t *testing.T) {
	ix, c, opt := profWorld()
	tables := profTables(t, []pfReagent{{500, 7, 1}})
	var offers []pfOffer
	ix.Areas[902] = questie.Area{UIMap: 7002, Zone: "Durotar"}
	for id := 100; id < 108; id++ { // eight vendors sell the cloth; only the last stands in town
		offers = append(offers, pfOffer{int32(id), 7})
		ix.Npcs[id] = questie.Entity{Name: fmt.Sprintf("Cloth Seller %d", id), Flags: questie.FlagVendor, Spawns: []questie.Spawn{{Area: 902, X: 50, Y: 50}}}
	}
	ix.Npcs[107] = questie.Entity{Name: "Cloth Seller 107", Flags: questie.FlagVendor, Spawns: []questie.Spawn{{Area: 901, X: 52, Y: 50}}}
	offers = append(offers, pfOffer{108, 7})
	ix.Npcs[108] = questie.Entity{Name: "Alliance Seller", Faction: "A", Flags: questie.FlagVendor, Spawns: []questie.Spawn{{Area: 901, X: 50, Y: 51}}}
	tables["vendor_sources"] = pfTable(t, offers)
	opt.Data = store.New(tables)
	c.Professions = []character.Skill{skillOf("First Aid", 60, 75)}
	es := restocks(BuildWith(c, ix, opt))
	if len(es) != 1 || !strings.HasSuffix(es[0].Label, " Heavy Linen Cloth from Cloth Seller 107") {
		t.Fatalf("restock lines %+v", es)
	}
}
