package route

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

const vialID = 3371

// sourcingWorld is bundleWorld with a banker beside the mailbox, and a poisoner who is short 4 Empty Vial for a
// batch (one in the bags) that a vendor beside the route sells.
func sourcingWorld() (*questie.Index, character.Character) {
	ix, c := bundleWorld()
	ix.Npcs[21] = questie.Entity{Name: "Test Banker", Flags: questie.FlagBanker, Spawns: []questie.Spawn{{Area: 901, X: 52, Y: 50}}}
	c.Faction, c.Realm, c.Key = "Alliance", "Testrealm", "Testrealm:Hero"
	c.Bags = []character.ItemStack{{ItemID: vialID, Count: 1, Name: "Empty Vial"}}
	c.BagsCaptured = true
	c.Recipes = map[string]character.SeenRecipes{"poisons": {Entries: []character.SeenRecipe{
		{Name: "Instant Poison", Difficulty: "yellow", Reagents: []character.ItemStack{{ItemID: vialID, Count: 1}}}}}}
	c.Seen = &character.Observed{Vendors: []character.SeenVendor{{NPC: 7, Name: "Sly Garrett", Map: 7001, X: 52, Y: 51,
		Items: []character.SeenStock{{ItemID: int64(vialID), Price: 50}}}}}
	return ix, c
}

func vials(n int) []character.ItemStack {
	return []character.ItemStack{{ItemID: vialID, Count: n, Name: "Empty Vial"}}
}

func sourcingAlt(name string, n int) character.Character {
	a := hero()
	a.Name, a.Key, a.Faction, a.Realm = name, "Testrealm:"+name, "Alliance", "Testrealm"
	a.Bags, a.BagsCaptured = vials(n), true
	return a
}

func errandText(p Plan) string {
	var l []string
	for _, e := range p.Errands {
		l = append(l, e.Kind+": "+e.Label+" | "+e.Why)
	}
	return strings.Join(l, "\n")
}

func buildWith(c character.Character, ix *questie.Index, others ...character.Character) Plan {
	return BuildWith(c, ix, Options{Others: append([]character.Character{c}, others...)})
}

func TestOwnBankCoversTheShortage(t *testing.T) {
	ix, c := sourcingWorld()
	c.Bank, c.BankCaptured = vials(10), true
	p := buildWith(c, ix)
	b := attached(p, ErrandBank)
	if b == nil || b.Label != "take 4 Empty Vial from your bank at Test Banker" || b.Attach == 0 || attached(p, ErrandRestock) != nil {
		t.Fatalf("bank pickup, no buy expected:\n%s", errandText(p))
	}
}

func TestSplitAcrossBankAltAndVendor(t *testing.T) {
	ix, c := sourcingWorld()
	c.Bank, c.BankCaptured = vials(1), true
	old, now := int64(100), int64(200)
	c.CapturedAt = &now
	alt := sourcingAlt("Zed", 2)
	alt.CapturedAt = &old
	p := buildWith(c, ix, alt)
	b, m, r := attached(p, ErrandBank), attached(p, ErrandMail), attached(p, ErrandRestock)
	if b == nil || !strings.HasPrefix(b.Label, "take 1 Empty Vial") {
		t.Fatalf("bank:\n%s", errandText(p))
	}
	if m == nil || !strings.HasPrefix(m.Label, "Zed can mail you 2 Empty Vial") || !strings.Contains(m.Why, "as of their last login") || m.Attach == 0 {
		t.Fatalf("alt mail:\n%s", errandText(p))
	}
	if r == nil || r.Label != "buy 1 Empty Vial from Sly Garrett" {
		t.Fatalf("the rest is bought:\n%s", errandText(p))
	}
}

func TestAltKeepsWhatItsOwnQuestNeeds(t *testing.T) {
	ix, c := sourcingWorld()
	alt := sourcingAlt("Zed", 9)
	no := false
	alt.Quests = []character.Quest{{QuestID: 99, Title: "Vial Run", Complete: &no, Objectives: []character.Objective{{Type: "item", Text: "Empty Vial: 0/4", Done: &no}}}}
	p := buildWith(c, ix, alt)
	if attached(p, ErrandMail) != nil || attached(p, ErrandRestock) == nil {
		t.Fatalf("the alt's quest item is not used:\n%s", errandText(p))
	}
}

func TestAltOnTheOtherFactionIsNotUsed(t *testing.T) {
	ix, c := sourcingWorld()
	alt := sourcingAlt("Zed", 9)
	alt.Faction = "Horde"
	p := buildWith(c, ix, alt)
	if attached(p, ErrandMail) != nil || attached(p, ErrandRestock) == nil {
		t.Fatalf("mail cannot cross factions:\n%s", errandText(p))
	}
}

func TestNoBankCaptureKeepsBagsOnlyAndSaysSo(t *testing.T) {
	ix, c := sourcingWorld()
	c.Bank = vials(10) // not captured: never read
	p := buildWith(c, ix)
	r := attached(p, ErrandRestock)
	if attached(p, ErrandBank) != nil || r == nil || r.Label != "buy 4 Empty Vial from Sly Garrett" || !strings.Contains(r.Why, "your bank was not seen") {
		t.Fatalf("bags-only behavior:\n%s", errandText(p))
	}
}

func TestAltsPlanCarriesTheMatchingMailErrand(t *testing.T) {
	ix, c := sourcingWorld()
	alt := sourcingAlt("Zed", 9)
	p := buildWith(alt, ix, c)
	m := attached(p, ErrandMail)
	if m == nil || m.Label != "mail 4 Empty Vial to Hero at Mailbox" || m.Attach == 0 {
		t.Fatalf("matching mail errand:\n%s", errandText(p))
	}
}
