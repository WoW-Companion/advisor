package digest

import (
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

func bagsStore(t *testing.T) store.DataStore {
	it := func(id int64, name string, ilvl, req, q, slot int32) upItem {
		return upItem{ID: id, Name: name, ItemLevel: ilvl, RequiredLevel: req, Quality: q, ClassID: 4, SubclassID: 3, Slot: slot, ClassMask: -1, StatsStatus: "none"}
	}
	world := []upLoot{{20, "creature", 701, 100, 2}, {21, "creature", 701, 100, 2}, {22, "creature", 701, 100, 2}}
	return store.New(store.ReaderSource{
		"items": upTbl(t, []upItem{
			it(1, "Plain Boots", 8, 1, 1, 8), it(2, "Test Mail Boots", 10, 4, 2, 8),
			it(3, "Test Mail Gauntlets", 12, 7, 2, 10), it(4, "Plain Gauntlets", 10, 1, 1, 10),
			it(20, "Test Mail Helm", 14, 8, 2, 1), it(21, "Test Mail Shoulders", 14, 8, 2, 3),
			it(22, "Test Mail Boots Too", 14, 8, 2, 8),
		}),
		"loot_sources": upTbl(t, world),
	})
}

func bagsWarrior(bags ...character.ItemStack) character.Character {
	return character.Character{Name: "Testwarrior", Class: "WARRIOR", Level: ip(10),
		Equipment: map[string]character.EquippedItem{
			"feet": {Slot: "feet", ItemID: ip(1), Name: "Plain Boots"}, "hands": {Slot: "hands", ItemID: ip(4), Name: "Plain Gauntlets"}},
		Bags: bags}
}

func sectionText(d Digest, name string) []string {
	var out []string
	for _, s := range d.Sections {
		if s.ID == name {
			for _, l := range s.Lines {
				out = append(out, l.Slot+" | "+l.Text+" | "+l.How)
			}
			out = append(out, "summary: "+s.Summary)
		}
	}
	return out
}

func TestBagUpgradesLeadPanelAndNext(t *testing.T) {
	ds := bagsStore(t)
	opt := Options{Now: time.Unix(1, 0)}
	before := Build(bagsWarrior(), ds, opt)
	after := Build(bagsWarrior(character.ItemStack{ItemID: 2, Count: 1}, character.ItemStack{ItemID: 3, Count: 1}), ds, opt)
	t.Log("before upgrades:\n" + strings.Join(sectionText(before, Upgrades), "\n"))
	t.Log("before next:\n" + strings.Join(sectionText(before, NextQuests), "\n"))
	t.Log("after upgrades:\n" + strings.Join(sectionText(after, Upgrades), "\n"))
	t.Log("after next:\n" + strings.Join(sectionText(after, NextQuests), "\n"))
	up := sectionText(after, Upgrades)
	if len(up) < 3 || !strings.Contains(up[0], "Test Mail Gauntlets") || !strings.Contains(up[1], "Test Mail Boots") || !strings.Contains(up[0], "In your bags") {
		t.Fatalf("bag items should lead the upgrades: %v", up)
	}
	for _, l := range up[2:] {
		if strings.HasPrefix(l, "Feet |") || strings.HasPrefix(l, "Hands |") {
			t.Errorf("a slot a bag item covers is listed again: %s", l)
		}
	}
	if next := sectionText(after, NextQuests); !strings.Contains(next[0], "Equip from your bags: Test Mail Gauntlets (Hands), Test Mail Boots (Feet)") {
		t.Fatalf("NEXT should lead with the bag count: %v", next)
	}
	one := Build(bagsWarrior(character.ItemStack{ItemID: 2, Count: 1}), ds, opt)
	if next := sectionText(one, NextQuests); !strings.Contains(next[0], "Equip Test Mail Boots (Feet) from your bags") {
		t.Fatalf("one bag upgrade should be named: %v", next)
	}
	if next := sectionText(before, NextQuests); len(next) > 0 && strings.Contains(next[0], "in your bags") {
		t.Fatalf("no bag line without bag upgrades: %v", next)
	}
}
