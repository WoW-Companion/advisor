package digest

import (
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/wording"
)

// Synthetic weapons: a one-hander and a shield are worn; a one-hander (fits the main hand and, once
// Dual Wield is learned, the off hand), a better shield and a two-hander are candidates.
const (
	idSword, idBuckler, idAxe, idShield, idMaul = 50, 51, 60, 61, 62
)

func weaponStore(t *testing.T, dualWieldLevel int32) store.DataStore {
	it := func(id int64, name string, ilvl, class, subclass, slot int32) upItem {
		return upItem{ID: id, Name: name, ItemLevel: ilvl, RequiredLevel: 8, Quality: 2, ClassID: class, SubclassID: subclass, Slot: slot, StatsStatus: "none"}
	}
	src := store.ReaderSource{
		"items": upTbl(t, []upItem{
			it(idSword, "Worn Blade", 8, 2, 7, 13), it(idBuckler, "Tribal Buckler", 8, 4, 6, 14),
			it(idAxe, "Thick War Axe", 16, 2, 0, 13), it(idShield, "Oak Barrier", 14, 4, 6, 14), it(idMaul, "Heavy Maul", 30, 2, 5, 17),
		}),
		"loot_sources": upTbl(t, []upLoot{{idAxe, "quest", 501, 0, 0}, {idShield, "quest", 502, 0, 0}, {idMaul, "quest", 503, 0, 0}}),
		"quests":       upTbl(t, []upQuest{{501, "Axe Quest", 9, "Dunemark"}, {502, "Shield Quest", 9, "Dunemark"}, {503, "Maul Quest", 9, "Dunemark"}}),
	}
	if dualWieldLevel > 0 {
		src["class_spells"] = tbl(t, []spellRow{{1, 674, "Dual Wield", nil, dualWieldLevel, nil}})
	}
	return store.New(src)
}

func weaponWarrior(level int, p *character.Profile) character.Character {
	worn := func(slot string, id, ilvl int, name string) character.EquippedItem {
		return character.EquippedItem{Slot: slot, ItemID: ip(id), ItemLevel: ip(ilvl), Name: name}
	}
	return character.Character{
		Name: "Testwarrior", Class: "WARRIOR", Level: ip(level), Profile: p,
		Equipment: map[string]character.EquippedItem{"mainhand": worn("mainhand", idSword, 8, "Worn Blade"), "offhand": worn("offhand", idBuckler, 8, "Tribal Buckler")},
		Quests:    []character.Quest{{QuestID: 501, Title: "Axe Quest"}, {QuestID: 502, Title: "Shield Quest"}, {QuestID: 503, Title: "Maul Quest"}},
	}
}

func invTypes(c gear.Result, slot string) map[int]bool {
	m := map[int]bool{}
	for _, cd := range c.Slots[slot] {
		m[cd.Item.Slot] = true
	}
	return m
}

func TestWeaponAndOffHandChoices(t *testing.T) {
	fury := &character.Profile{Role: "damage", Focus: "leveling", Spec: "Fury"}
	tank := &character.Profile{Role: "tank", Focus: "dungeons", Spec: "Protection"}
	cases := []struct {
		name      string
		level     int
		dw        int32
		p         *character.Profile
		offWeapon bool // an off-hand weapon is offered
		offShield bool
		twoHand   bool
	}{
		{"level 10 with a shield, no play style", 10, 20, nil, false, true, false},
		{"level 10, damage style, Dual Wield not learned", 10, 20, fury, false, false, true},
		{"level 20 with a shield, no play style", 20, 20, nil, false, true, false},
		{"level 20 tank", 20, 20, tank, false, true, false},
		{"level 20 damage knows Dual Wield", 20, 20, fury, true, false, true},
		{"level 20 damage, dataset says Dual Wield at 30", 20, 30, fury, false, false, true},
		{"level 10, no class spell table", 10, 0, fury, false, false, true},
	}
	for _, tc := range cases {
		res, err := gear.Find(weaponWarrior(tc.level, tc.p), weaponStore(t, tc.dw), "", gear.Options{NoSources: true})
		if err != nil {
			t.Fatal(err)
		}
		off, main := invTypes(res, "offhand"), invTypes(res, "mainhand")
		if got := off[13] || off[22]; got != tc.offWeapon {
			t.Errorf("%s: off-hand weapon offered = %v", tc.name, got)
		}
		if off[14] != tc.offShield {
			t.Errorf("%s: shield offered = %v", tc.name, off[14])
		}
		if main[17] != tc.twoHand {
			t.Errorf("%s: two-hander offered = %v", tc.name, main[17])
		}
	}
}

func TestShieldUsersKeepShields(t *testing.T) {
	// A dual wielder (no shield worn) keeps weapons: no shield for them without a style change.
	c := weaponWarrior(20, nil)
	c.Equipment["offhand"] = character.EquippedItem{Slot: "offhand", ItemID: ip(idAxe), ItemLevel: ip(8), Name: "Worn Axe"}
	res, _ := gear.Find(c, weaponStore(t, 20), "", gear.Options{NoSources: true})
	if invTypes(res, "offhand")[14] {
		t.Error("a shield is offered to a dual wielder")
	}
	// A shield user's shield is replaced only by a shield.
	res, _ = gear.Find(weaponWarrior(20, nil), weaponStore(t, 20), "", gear.Options{NoSources: true})
	for _, cd := range res.Slots["offhand"] {
		if cd.Item.Slot != 14 {
			t.Errorf("off hand offers %s (inventory type %d) to a shield user", cd.Item.Name, cd.Item.Slot)
		}
	}
}

func TestSlotLabelsMatchInventoryTypes(t *testing.T) {
	for _, tc := range []struct {
		slot string
		inv  int
		want string
	}{
		{"mainhand", 13, "One-hand"}, {"mainhand", 21, "Main hand"}, {"mainhand", 17, "Two-hand"},
		{"offhand", 14, "Shield"}, {"offhand", 22, "Off hand"}, {"offhand", 23, "Held in off hand"}, {"offhand", 13, "One-hand"},
		{"mainhand", 0, "Main hand"}, {"offhand", 0, "Off hand"},
	} {
		if got := wording.SlotName(tc.slot, tc.inv); got != tc.want {
			t.Errorf("SlotName(%s, %d) = %q, want %q", tc.slot, tc.inv, got, tc.want)
		}
	}
	// In a built digest each weapon entry's slot is its item's own type.
	fury := &character.Profile{Role: "damage", Focus: "leveling", Spec: "Fury"}
	label := map[int]string{idAxe: "One-hand", idShield: "Shield", idMaul: "Two-hand"}
	for _, s := range Build(weaponWarrior(20, fury), weaponStore(t, 20), synthOpt).Sections {
		for _, l := range s.Lines {
			if want, ok := label[l.Item]; s.ID == Upgrades && ok && l.Slot != want {
				t.Errorf("%s: slot %q, want %q", l.Text, l.Slot, want)
			}
		}
	}
}

var forbidden = regexp.MustCompile(`(?i)upgrade|instead of|wear now|fills an empty`)

func TestUpgradeFieldsCarryEachFactOnce(t *testing.T) {
	fury := &character.Profile{Role: "damage", Focus: "leveling", Spec: "Fury"}
	for name, d := range map[string]Digest{
		"paths":  Build(pathsWarrior(), pathsStore(t), synthOpt),
		"weapon": Build(weaponWarrior(20, fury), weaponStore(t, 20), synthOpt),
		"shield": Build(weaponWarrior(10, nil), weaponStore(t, 20), synthOpt),
	} {
		n := 0
		for _, s := range d.Sections {
			if s.ID != Upgrades {
				continue
			}
			for _, l := range s.Lines {
				n++
				if strings.HasPrefix(l.Text, l.Slot+":") || forbidden.MatchString(l.Text) || strings.Contains(strings.ToLower(l.Text), strings.ToLower(l.Slot)+":") {
					t.Errorf("%s: text %q repeats a field", name, l.Text)
				}
				if strings.Contains(l.Replaces, "instead of") || strings.Contains(l.Replaces, "empty") {
					t.Errorf("%s: replaces %q is more than a name", name, l.Replaces)
				}
				for _, det := range append(l.Details, l.Why) {
					if forbidden.MatchString(det) || strings.HasPrefix(det, l.Slot+":") {
						t.Errorf("%s: %q repeats a field", name, det)
					}
				}
				if len([]rune(l.How)) > MaxHow {
					t.Errorf("%s: how %q is long", name, l.How)
				}
			}
		}
		if n == 0 {
			t.Errorf("%s: no upgrades to check", name)
		}
	}
}

func TestCraftWording(t *testing.T) {
	chest := func(c character.Character) Line {
		for _, s := range Build(c, pathsStore(t), synthOpt).Sections {
			for _, l := range s.Lines {
				if s.ID == Upgrades && l.PathKind == "craft" {
					return l
				}
			}
		}
		t.Fatal("no craft entry")
		return Line{}
	}
	learn := chest(pathsWarrior())
	if learn.How != "You can make it now (your Blacksmithing 45, needs 5)" || !strings.Contains(strings.Join(learn.Details, " "), "Learn it from a Blacksmithing trainer (if you haven't)") {
		t.Errorf("learnable: %q", learn.How)
	}
	known := pathsWarrior()
	known.KnownSpells = map[int64]bool{800: true}
	if got := chest(known).How; got != "You can make it now (your Blacksmithing 45, needs 5)" {
		t.Errorf("known: %q", got)
	}
}
