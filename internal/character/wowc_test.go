package character

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/WoW-Companion/advisor/internal/lua"
)

const wowcLua = `WoWCompanionDB = {
	["characters"] = {
		["Realm:Mage"] = {
			["version"] = 1, ["name"] = "Mage", ["realm"] = "Realm", ["capturedAt"] = 2000,
			["level"] = 30, ["class"] = "MAGE", ["race"] = "Human", ["faction"] = "Alliance", ["money"] = 5000,
			["equipment"] = { [1] = { ["id"] = 111, ["enchant"] = 7 }, [5] = { ["id"] = 222 } },
			["bags"] = { [0] = { ["size"] = 16, ["items"] = { [1] = { ["id"] = 5, ["count"] = 3 } } } },
			["quests"] = { ["captured"] = true, ["entries"] = { { ["id"] = 77, ["title"] = "Q", ["level"] = 29,
				["objectives"] = { { ["text"] = "Kill 5", ["have"] = 2, ["need"] = 5 } } } } },
			["completedQuests"] = { 1, 2 },
			["talents"] = { { ["tree"] = "Fire", ["nodes"] = { { ["name"] = "Impact", ["rank"] = 2, ["max"] = 5 } } } },
			["skills"] = { { ["name"] = "Mining", ["rank"] = 40, ["max"] = 75, ["group"] = "Professions", ["profession"] = true } },
			["spells"] = { 133, 168 },
			["reputations"] = { { ["faction"] = "Stormwind", ["standing"] = 5, ["value"] = 10, ["min"] = 0, ["max"] = 100 } },
		},
	},
}`

const fsLua = `ForeverScribeDB = { ["chars"] = { ["Realm:Mage"] = {
	["snapshot"] = { ["identity"] = { ["name"] = "Mage", ["realm"] = "Realm", ["class"] = "MAGE" }, ["level"] = 29, ["capturedAt"] = %d,
		["bags"] = { ["items"] = { { ["itemID"] = 9, ["count"] = 1 } } } },
	["bank"] = { ["items"] = { { ["itemID"] = 8, ["count"] = 20 } } },
}, ["Realm:Other"] = { ["snapshot"] = { ["identity"] = { ["name"] = "Other", ["realm"] = "Realm" }, ["level"] = 5, ["capturedAt"] = 10 } } } }`

func sv(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for n, body := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestOurCaptureLoadsIntoTheModel(t *testing.T) {
	dir := sv(t, map[string]string{WoWCFile: wowcLua})
	f, err := LoadFile(filepath.Join(dir, FSFile)) // ForeverScribe's file is absent: the addon's alone is read
	if err != nil || !f.HasWoWC || f.HasFS || len(f.Characters) != 1 {
		t.Fatalf("%v %+v", err, f)
	}
	c := f.Characters[0]
	if c.Key != "Realm:Mage" || c.Source != SourceWoWC || deref(c.Level) != 30 || c.Faction != "Alliance" || deref(c.Money) != 5000 {
		t.Errorf("identity %+v", c)
	}
	if c.Equipment["head"].ItemID == nil || *c.Equipment["head"].ItemID != 111 || c.Equipment["chest"].Slot != "chest" {
		t.Errorf("equipment %+v", c.Equipment)
	}
	if !c.BagsCaptured || len(c.Bags) != 1 || c.Bags[0].Count != 3 || c.BankCaptured {
		t.Errorf("bags %+v bank %v", c.Bags, c.BankCaptured)
	}
	if len(c.Quests) != 1 || c.Quests[0].QuestID != 77 || deref(c.Quests[0].Objectives[0].Need) != 5 || len(c.CompletedQuests) != 2 {
		t.Errorf("quests %+v", c.Quests)
	}
	if c.Talents == nil || c.Talents.Trees[0].Name != "Fire" || c.Talents.Trees[0].Talents[0].Rank != 2 {
		t.Errorf("talents %+v", c.Talents)
	}
	if len(c.Professions) != 1 || c.Professions[0].Name != "Mining" || !c.KnownSpells[133] || len(c.Reputations) != 1 {
		t.Errorf("skills %+v spells %v reps %v", c.Professions, c.KnownSpells, c.Reputations)
	}
}

func TestNewestCaptureWinsAndFillsWhatItLacks(t *testing.T) {
	for _, tc := range []struct {
		name     string
		fsAt     int
		level    int
		bagCount int // the leading capture's bags
	}{{"addon newer", 1000, 30, 3}, {"scribe newer", 3000, 29, 1}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := sv(t, map[string]string{WoWCFile: wowcLua, FSFile: fmt.Sprintf(fsLua, tc.fsAt)})
			f, err := LoadFile(filepath.Join(dir, FSFile))
			if err != nil || !f.HasWoWC || !f.HasFS || len(f.Characters) != 2 {
				t.Fatalf("%v %+v", err, f)
			}
			var c Character
			for _, x := range f.Characters {
				if x.Name == "Mage" {
					c = x
				}
			}
			if deref(c.Level) != tc.level || c.Bags[0].Count != tc.bagCount {
				t.Errorf("level %v bags %+v", c.Level, c.Bags)
			}
			if !c.BankCaptured || len(c.Bank) != 1 || c.Bank[0].ItemID != 8 {
				t.Errorf("the bank only ForeverScribe has was not kept: %+v", c.Bank)
			}
			if c.Source != SourceWoWC+" + "+SourceFS || c.Talents == nil {
				t.Errorf("source %q talents %v", c.Source, c.Talents)
			}
		})
	}
}

func TestOnlyForeverScribeStillLoads(t *testing.T) {
	dir := sv(t, map[string]string{FSFile: fmt.Sprintf(fsLua, 5)})
	f, err := LoadFile(filepath.Join(dir, FSFile))
	if err != nil || f.HasWoWC || !f.HasFS || f.Characters[0].Source != SourceFS {
		t.Fatalf("%v %+v", err, f)
	}
	if _, err := LoadFile(filepath.Join(t.TempDir(), FSFile)); !os.IsNotExist(err) {
		t.Errorf("no file at all: %v", err)
	}
}

const observedLua = `WoWCompanionDB = { ["observed"] = {
	["trainers"] = { { ["npc"] = 5, ["name"] = "Smith", ["map"] = 84, ["x"] = 40.1, ["y"] = 60, ["faction"] = "Alliance", ["t"] = 9,
		["offers"] = { { ["name"] = "Runed Copper Gauntlets", ["cost"] = 1250, ["skill"] = "Blacksmithing", ["skillLevel"] = 75, ["spell"] = 500 } } } },
	["vendors"] = { { ["npc"] = 6, ["name"] = "Seller", ["map"] = 84, ["x"] = 10, ["y"] = 20, ["items"] = { { ["id"] = 70, ["price"] = 9000 } } } },
	["gathering"] = { { ["kind"] = "herb", ["map"] = 84, ["cx"] = 4, ["cy"] = 6, ["n"] = 7, ["skillMin"] = 1, ["skillMax"] = 30, ["items"] = { [2447] = { ["count"] = 9, ["n"] = 7 } } } },
	["events"] = { { ["t"] = 5, ["ev"] = "accepted", ["questID"] = 77, ["npc"] = "Old Giver", ["npcId"] = 3, ["npcMap"] = 84, ["npcX"] = 30, ["npcY"] = 31 } },
} }`

func TestParseObservedReadsEverySection(t *testing.T) {
	vars, err := lua.Parse(observedLua)
	if err != nil {
		t.Fatal(err)
	}
	o := ParseObserved(vars)
	if len(o.Trainers) != 1 || o.Trainers[0].Offers[0].Cost != 1250 || o.Trainers[0].Offers[0].SkillLevel != 75 {
		t.Errorf("trainers %+v", o.Trainers)
	}
	if vs, st := o.VendorsOf(70, "Horde"); len(vs) != 1 || st[0].Price != 9000 {
		t.Errorf("vendors %+v", vs)
	}
	if len(o.Gather) != 1 || o.Gather[0].Items[2447] != 9 || o.Gather[0].CX != 4 {
		t.Errorf("gather %+v", o.Gather)
	}
	if g, ok := o.GiverOf(77); !ok || g.NPC != "Old Giver" || g.X != 30 {
		t.Errorf("giver %+v", g)
	}
	if got := o.TrainersFor("Horde"); len(got) != 0 {
		t.Errorf("a Horde character was offered an Alliance trainer: %+v", got)
	}
}

func TestAttachAddonReadsObservations(t *testing.T) {
	dir := sv(t, map[string]string{WoWCFile: observedLua})
	c := AttachAddon(filepath.Join(dir, WoWCFile), Character{Key: "Realm:Mage"})
	if c.Seen.Empty() || len(c.Seen.Trainers) != 1 {
		t.Fatalf("%+v", c.Seen)
	}
	if c := AttachAddon(filepath.Join(dir, "nothing.lua"), Character{}); c.Seen != nil {
		t.Error("observations from a missing file")
	}
}

func TestRecipesAndFlightMastersAreRead(t *testing.T) {
	vars, err := lua.Parse(`WoWCompanionDB = { characters = { ["R:Smith"] = { name = "Smith", recipes = { Blacksmithing = { seenAt = 5, entries = {
	  { name = "Copper Rod", id = 9, difficulty = "Orange", makes = { item = 7, count = 1 }, reagents = { { item = 3, count = 2 } } } } } } } },
	  observed = { flightmasters = { { npc = 1, name = "Wing Wen", map = 12, x = 1, y = 2, faction = "Alliance", t = 10 } } } }`)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := LoadWoWC(vars)
	r, ok := f.Characters[0].Recipe("blacksmithing", "copper rod")
	if !ok || r.Difficulty != "orange" || r.Makes != 7 || len(r.Reagents) != 1 || r.Reagents[0].Count != 2 {
		t.Errorf("recipe %+v %v", r, ok)
	}
	if fl := ParseObserved(vars).FlightsFor("Alliance"); len(fl) != 1 || fl[0].Name != "Wing Wen" {
		t.Errorf("flights %+v", fl)
	}
	if ParseObserved(vars).FlightsFor("Horde") != nil {
		t.Error("other faction's flight master listed")
	}
	if got := ParseObserved(map[string]any{}); got.FlightsFor("") != nil {
		t.Error("absent sections must be empty")
	}
}

func TestWoWCTalentPoints(t *testing.T) {
	n := 3
	for name, tc := range map[string]struct {
		talents any
		points  *int
		want    *int
		nilT    bool
	}{
		"points with no talents": {nil, &n, &n, false},
		"absent points":          {nil, nil, nil, true},
	} {
		got := wowcTalents(tc.talents, tc.points)
		if (got == nil) != tc.nilT || (got != nil && (got.Unspent == nil) != (tc.want == nil || false)) {
			t.Errorf("%s: %+v", name, got)
		}
		if got != nil && tc.want != nil && *got.Unspent != *tc.want {
			t.Errorf("%s: unspent %d", name, *got.Unspent)
		}
	}
}

func TestWoWCTalentNodeSpellID(t *testing.T) {
	tr := wowcTalentTrees(lua.Table{int64(1): lua.Table{"tree": "Shadow", "nodes": lua.Table{int64(1): lua.Table{"spell": int64(7001), "rank": int64(3), "max": int64(5)}}}})
	if tr == nil {
		t.Fatal("no trees")
	}
	n := tr.Trees[0].Talents[0]
	if n.SpellID == nil || *n.SpellID != 7001 || n.Rank != 3 || n.Name != "" || *tr.Trees[0].Points != 3 {
		t.Errorf("node %+v", n)
	}
}
