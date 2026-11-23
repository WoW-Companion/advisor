package digest

import (
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type upItem struct {
	ID            int64   `parquet:"id"`
	Name          string  `parquet:"name"`
	ItemLevel     int32   `parquet:"item_level"`
	RequiredLevel int32   `parquet:"required_level"`
	Quality       int32   `parquet:"quality"`
	ClassID       int32   `parquet:"class_id"`
	SubclassID    int32   `parquet:"subclass_id"`
	Slot          int32   `parquet:"slot"`
	ClassMask     int32   `parquet:"allowable_class_mask"`
	StatTypes     []int16 `parquet:"stat_types,list"`
	StatsStatus   string  `parquet:"stats_status"`
	Bonding       int32   `parquet:"bonding"`
}

type upLoot struct {
	ItemID int64  `parquet:"item_id"`
	Kind   string `parquet:"source_kind"`
	ID     int64  `parquet:"source_id"`
	Kills  int32  `parquet:"kills"`
	Drops  int32  `parquet:"drops"`
}

type upQuest struct {
	ID    int64  `parquet:"id"`
	Name  string `parquet:"name"`
	Level int32  `parquet:"level"`
	Zone  string `parquet:"zone"`
}

type upRecipe struct {
	SpellID int64  `parquet:"spell_id"`
	Name    string `parquet:"name"`
	Line    string `parquet:"skill_line_name"`
	Learn   int16  `parquet:"learn_skill"`
	Creates int64  `parquet:"creates_item_id"`
}

type upReagent struct {
	SpellID int64 `parquet:"spell_id"`
	Item    int64 `parquet:"reagent_item_id"`
	Count   int16 `parquet:"count"`
}

func upTbl[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	return tblAny(t, rows)
}

// pathsStore: a cloak from a quest, a chest to craft, bracers on the Auction House, a belt that is
// only a random drop (Bind on Pickup, many creatures, all rare).
func pathsStore(t *testing.T) store.DataStore {
	it := func(id int64, name string, ilvl, slot, subclass, bonding int32) upItem {
		class := int32(4)
		return upItem{ID: id, Name: name, ItemLevel: ilvl, RequiredLevel: 8, Quality: 2, ClassID: class, SubclassID: subclass, Slot: slot, StatsStatus: "none", Bonding: bonding}
	}
	var world []upLoot
	for i := int64(0); i < 7; i++ {
		world = append(world, upLoot{4, "creature", 700 + i, 100, 3})
	}
	return store.New(store.ReaderSource{
		"items": upTbl(t, []upItem{
			it(1, "Traveler's Shawl", 23, 16, 0, 0), it(2, "Copper Chain Vest", 17, 5, 3, 0),
			it(3, "Tough Bracers", 20, 9, 3, 2), it(4, "Wolf Fang Belt", 19, 6, 3, 1),
			it(10, "Copper Bar", 1, 0, 0, 0),
		}),
		"loot_sources":    upTbl(t, append(world, upLoot{1, "quest", 500, 0, 0}, upLoot{3, "creature", 701, 100, 2}, upLoot{10, "vendor", 1, 0, 0})),
		"quests":          upTbl(t, []upQuest{{500, "Lost Shawl", 9, "Dunemark"}}),
		"recipes":         upTbl(t, []upRecipe{{800, "Copper Chain Vest", "Blacksmithing", 5, 2}}),
		"recipe_reagents": upTbl(t, []upReagent{{800, 10, 6}}),
	})
}

func pathsWarrior() character.Character {
	smith := 45
	return character.Character{
		Name: "Testwarrior", Class: "WARRIOR", Level: ip(10),
		Equipment:   map[string]character.EquippedItem{"back": {Slot: "back", ItemLevel: ip(12), Name: "Worn Cloak"}},
		Professions: []character.Skill{{Name: "Blacksmithing", Rank: &smith}, {Name: "Mining", Rank: &smith}},
		Quests:      []character.Quest{{QuestID: 500, Title: "Lost Shawl", Zone: "Dunemark"}},
		Bags:        []character.ItemStack{{ItemID: 10, Count: 2, Name: "Copper Bar"}},
	}
}

var idDigits = regexp.MustCompile(`\d{3,}|\bilvl\b|item level|recipe \d`)

func TestUpgradeEntriesArePlainAndDependable(t *testing.T) {
	d := Build(pathsWarrior(), pathsStore(t), synthOpt)
	var ups, mats, quests *Section
	for i := range d.Sections {
		switch d.Sections[i].ID {
		case Upgrades:
			ups = &d.Sections[i]
		case KeepMaterials:
			mats = &d.Sections[i]
		case NextQuests:
			quests = &d.Sections[i]
		}
	}
	if ups == nil || ups.Summary != "3 upgrades you can get" {
		t.Fatalf("upgrades section: %+v", ups)
	}
	slots := map[string]Line{}
	for _, l := range ups.Lines {
		slots[l.Slot] = l
		for _, s := range append([]string{l.Text, l.Wear, l.Gain, l.Replaces, l.How, l.Why}, append(l.Details, l.Asks...)...) {
			if m := idDigits.FindString(s); m != "" {
				t.Errorf("%s: player-facing text %q holds %q", l.Slot, s, m)
			}
		}
		if strings.Contains(strings.ToLower(l.Text), "ilvl") {
			t.Errorf("text %q has ilvl", l.Text)
		}
	}
	if _, bad := slots["Waist"]; bad {
		t.Error("the world-drop belt is listed")
	}
	cloak := slots["Cloak"]
	if cloak.Wear != "Wear now" || cloak.Replaces != "instead of your Worn Cloak" || cloak.PathKind != "quest" || cloak.Gain != "Big upgrade" ||
		cloak.How != "Finish Lost Shawl for it (it's in your quest log, in Dunemark)" {
		t.Errorf("cloak: %+v", cloak)
	}
	if c := slots["Chest"]; c.PathKind != "craft" || c.Replaces != "fills an empty slot" || !strings.HasPrefix(c.How, "Make it with your Blacksmithing") {
		t.Errorf("chest: %+v", c)
	}
	w := slots["Wrists"]
	if w.PathKind != "auction" || w.Text != "Wrists: any green mail bracers, level 8–10 — cheap on the Auction House" ||
		w.How != "Cheap on the Auction House: mail bracers, level 8–10" || !strings.Contains(strings.Join(w.Details, " "), "Set the level range to 8 to 10") || !strings.Contains(strings.Join(w.Asks, "|"), "What should I search for on the Auction House?") {
		t.Errorf("wrists: %+v", w)
	}
	if m := regexp.MustCompile(`(?i)\bgold\b|\bsilver\b|\bprice|\d+[gsc]\b`).FindString(w.Text + w.How + strings.Join(w.Details, " ")); m != "" {
		t.Errorf("wrists shows a price: %q", m)
	}
	if got := cloak.Asks; len(got) == 0 || got[0] != "How do I get Traveler's Shawl?" && got[1] != "How do I get Traveler's Shawl?" {
		t.Errorf("asks: %v", got)
	}
	// The quest that rewards the cloak says so.
	found := false
	for _, l := range quests.Lines {
		if l.Quest == 500 {
			found = true
			if l.Why != "Rewards Traveler's Shawl, a big cloak upgrade" {
				t.Errorf("quest why = %q", l.Why)
			}
		}
	}
	if !found {
		t.Error("quest 500 is not a next quest")
	}
	// The bar held for the vest says what it is for.
	if mats == nil || len(mats.Lines) != 1 || mats.Lines[0].Why != "For Copper Chain Vest" || !strings.Contains(mats.Lines[0].Text, "for Copper Chain Vest") {
		t.Errorf("materials: %+v", mats)
	}
}

func TestGainWords(t *testing.T) {
	c := pathsWarrior()
	d := Build(c, pathsStore(t), synthOpt)
	for _, s := range d.Sections {
		if s.ID == Upgrades {
			for _, l := range s.Lines {
				if l.Gain != "Big upgrade" && l.Gain != "Solid upgrade" && l.Gain != "Small upgrade" {
					t.Errorf("gain %q", l.Gain)
				}
			}
		}
	}
}
