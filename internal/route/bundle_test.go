package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

const flagPoison = 0x400

// bundleWorld is constructIsle with the windshaper's shop and repair flags off, so each test adds the NPCs it wants.
func bundleWorld() (*questie.Index, character.Character) {
	ix := constructIsle()
	e := ix.Npcs[3]
	e.Flags = flagQuestGiver
	ix.Npcs[3] = e
	ix.Objects[900] = questie.Entity{Name: "Mailbox", Spawns: []questie.Spawn{{Area: 901, X: 51, Y: 50.5}}}
	return ix, constructChar()
}

func attached(p Plan, kind string) *Errand {
	for i, e := range p.Errands {
		if e.Kind == kind {
			return &p.Errands[i]
		}
	}
	return nil
}

func TestRestockAttachesToTheStopNearTheVendor(t *testing.T) {
	ix, c := bundleWorld()
	vial := 3371
	c.Bags = []character.ItemStack{{ItemID: vial, Count: 1, Name: "Empty Vial"}}
	c.BagsCaptured = true
	c.Recipes = map[string]character.SeenRecipes{"poisons": {Entries: []character.SeenRecipe{
		{Name: "Instant Poison", Difficulty: "yellow", Reagents: []character.ItemStack{{ItemID: vial, Count: 1}}},
		{Name: "Old Poison", Difficulty: "grey", Reagents: []character.ItemStack{{ItemID: 9, Count: 1}}}}}}
	c.Seen = &character.Observed{Vendors: []character.SeenVendor{{NPC: 7, Name: "Sly Garrett", Map: 7001, X: 52, Y: 51,
		Items: []character.SeenStock{{ItemID: int64(vial), Price: 50}, {ItemID: 9, Price: 5}}}}}
	e := attached(Build(c, ix), ErrandRestock)
	if e == nil || e.Attach == 0 || e.Label != "buy 4 Empty Vial from Sly Garrett" || e.N != 0 {
		t.Fatalf("restock errand %+v", e)
	}
}

func TestRepairNeedsDurabilityAndANearbyNPC(t *testing.T) {
	ix, c := bundleWorld()
	d, m := 40, 100
	c.Equipment = map[string]character.EquippedItem{"Head": {Durability: &d, DurabilityMax: &m}, "Chest": {Durability: &d, DurabilityMax: &m}}
	if attached(Build(c, ix), ErrandRepair) != nil {
		t.Errorf("no repair NPC, no errand")
	}
	ix.Npcs[20] = questie.Entity{Name: "Anvil Smith", Flags: flagVendor | flagRepair, Spawns: []questie.Spawn{{Area: 901, X: 52, Y: 50}}}
	e := attached(Build(c, ix), ErrandRepair)
	if e == nil || e.Attach == 0 || e.NPC != "Anvil Smith" {
		t.Fatalf("repair errand %+v", e)
	}
	d = 90
	if attached(Build(c, ix), ErrandRepair) != nil {
		t.Errorf("healthy gear needs no repair")
	}
}

func TestFarNPCIsAttachedOnlyWhenUrgent(t *testing.T) {
	ix, c := bundleWorld()
	ix.Npcs[20] = questie.Entity{Name: "Far Smith", Flags: flagVendor | flagRepair, Spawns: []questie.Spawn{{Area: 901, X: 30, Y: 30}}}
	d, m := 40, 100
	c.Equipment = map[string]character.EquippedItem{"Head": {Durability: &d, DurabilityMax: &m}}
	if p := Build(c, ix); len(p.Errands) != 0 && attached(p, ErrandRepair) != nil {
		t.Errorf("a far NPC and no urgency attach nothing: %+v", p.Errands)
	}
	d = 10
	p := Build(c, ix)
	e := attached(p, ErrandRepair)
	if e == nil || e.Attach != 0 || e.N == 0 || p.Steps[e.N-1].Errand != ErrandRepair || p.Total != len(p.Steps) {
		t.Fatalf("urgent repair is its own step: %+v", e)
	}
}

func TestSellJunkWhenBagsAreFull(t *testing.T) {
	shops := []Spot{{Name: "Shopkeep", Map: 7001, X: 50, Y: 51}}
	if _, ok := junkChore(56, 66, 3, shops); !ok {
		t.Errorf("85%% full with greys is a sell errand")
	}
	for _, c := range [][3]int{{40, 66, 3}, {56, 66, 0}, {56, 0, 3}} {
		if _, ok := junkChore(c[0], c[1], c[2], shops); ok {
			t.Errorf("%v is no errand", c)
		}
	}
	ix, c := bundleWorld()
	p := Build(c, ix)
	ch, _ := junkChore(56, 66, 3, shops)
	bundleChores(&p, []Chore{ch})
	if e := attached(p, ErrandSellJunk); e == nil || e.Attach == 0 || e.Label != "sell junk to Shopkeep" {
		t.Errorf("sell errand %+v", e)
	}
}

func TestMailNamesTheAltAndKeepsQuestItems(t *testing.T) {
	ix, c := bundleWorld()
	p := Build(c, ix)
	box := spotsWhere(ix, c, map[int]bool{7001: true}, true, func(e questie.Entity) bool { return strings.Contains(strings.ToLower(e.Name), "mailbox") })
	protected := func(id int64, name string) bool { return name == "Test Core" }
	ch := mailChores([]Mailing{{Item: "Linen Cloth", Count: 20, To: "Bob"}, {Item: "Test Core", Count: 1, To: "Bob"}}, protected, box)
	if len(ch) != 1 {
		t.Fatalf("chores %+v", ch)
	}
	bundleChores(&p, ch)
	e := attached(p, ErrandMail)
	if e == nil || e.Attach == 0 || !strings.Contains(e.Label, "Bob") || strings.Contains(e.Label, "Test Core") || e.NPC != "Mailbox" {
		t.Errorf("mail errand %+v", e)
	}
}

func TestErrandGapsNameWhatIsNotCaptured(t *testing.T) {
	c := character.Character{Equipment: map[string]character.EquippedItem{"Head": {}}}
	if g := strings.Join(ErrandGaps(c), ";"); !strings.Contains(g, "durability") || !strings.Contains(g, "bag size") {
		t.Errorf("gaps %q", g)
	}
}

func TestProtectedQuestItemsAreNeverSold(t *testing.T) {
	guard := func(id int64, name string) bool { return name == "Test Core" }
	got := safeJunk([]string{"Gray Fang", "Test Core"}, guard)
	if len(got) != 1 || got[0] != "Gray Fang" {
		t.Errorf("safe junk %v", got)
	}
}

func TestRestockSkipsKnownRecipesOutsideAPurpose(t *testing.T) {
	ix, c := bundleWorld()
	vial, water, thistle := 3371, 1179, 2447
	c.Bags = []character.ItemStack{{ItemID: vial, Count: 1, Name: "Empty Vial"}}
	c.BagsCaptured = true
	c.Recipes = map[string]character.SeenRecipes{
		"cooking": {Entries: []character.SeenRecipe{{Name: "Thistle Tea", Difficulty: "green", Reagents: []character.ItemStack{
			{ItemID: water, Count: 1, Name: "Refreshing Spring Water"}, {ItemID: thistle, Count: 1, Name: "Swiftthistle"}}}}},
		"poisons": {Entries: []character.SeenRecipe{{Name: "Instant Poison", Difficulty: "yellow", Reagents: []character.ItemStack{{ItemID: vial, Count: 1, Name: "Empty Vial"}}}}}}
	c.Seen = &character.Observed{Vendors: []character.SeenVendor{{NPC: 7, Name: "Sly Garrett", Map: 7001, X: 52, Y: 51,
		Items: []character.SeenStock{{ItemID: int64(vial), Price: 50}, {ItemID: int64(water), Price: 5}}}}}
	p := Build(c, ix)
	n := 0
	for _, e := range p.Errands {
		if e.Kind == ErrandRestock {
			n++
			if e.Label != "buy 4 Empty Vial from Sly Garrett" || !strings.HasPrefix(e.Why, "for Instant Poison") {
				t.Errorf("restock %+v", e)
			}
		}
	}
	if n != 1 {
		t.Fatalf("want only the vial line: %+v", p.Errands)
	}
}

func TestRestockDropsACraftWithAnUnsoldReagent(t *testing.T) {
	ix, c := bundleWorld()
	vial, dust := 3371, 3372
	c.BagsCaptured = true
	c.Recipes = map[string]character.SeenRecipes{"poisons": {Entries: []character.SeenRecipe{{Name: "Deadly Poison", Difficulty: "orange", Reagents: []character.ItemStack{
		{ItemID: vial, Count: 1, Name: "Empty Vial"}, {ItemID: dust, Count: 1, Name: "Dust of Decay"}}}}}}
	c.Seen = &character.Observed{Vendors: []character.SeenVendor{{NPC: 7, Name: "Sly Garrett", Map: 7001, X: 52, Y: 51,
		Items: []character.SeenStock{{ItemID: int64(vial), Price: 50}}}}}
	if e := attached(Build(c, ix), ErrandRestock); e != nil {
		t.Fatalf("half a craft: %+v", e)
	}
}
