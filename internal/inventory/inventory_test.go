package inventory

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type itemRow struct {
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
	SellPrice     *int64  `parquet:"sell_price,optional"`
}

type recipeRow struct {
	SpellID       int32  `parquet:"spell_id"`
	SkillLineName string `parquet:"skill_line_name"`
	LearnSkill    *int16 `parquet:"learn_skill,optional"`
	YellowSkill   *int16 `parquet:"yellow_skill,optional"`
	GreySkill     *int16 `parquet:"grey_skill,optional"`
}

type reagentRow struct {
	SpellID       int32 `parquet:"spell_id"`
	ReagentItemID int32 `parquet:"reagent_item_id"`
	Count         int16 `parquet:"count"`
}

func table[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func i16(n int16) *int16 { return &n }
func ip(n int) *int      { return &n }
func bp(b bool) *bool    { return &b }

func item(id int64, name string, q, slot int32, price *int64) itemRow {
	return itemRow{ID: id, Name: name, ItemLevel: 20, RequiredLevel: 5, Quality: q, ClassID: 4, SubclassID: 1,
		Slot: slot, ClassMask: -1, StatTypes: []int16{5}, StatsStatus: "known", SellPrice: price}
}

const (
	headSlot, feetSlot = 1, 8
)

// synth: a priest at level 15 with Tailoring 40, wearing an item level 50 pair of boots and no hat.
func synth(t *testing.T, priced bool) store.DataStore {
	var price *int64
	if priced {
		p := int64(5)
		price = &p
	}
	return store.New(store.ReaderSource{
		"items": table(t, []itemRow{
			item(1, "Cloth Hood", 2, headSlot, nil),
			item(2, "Linen Cloth", 1, 0, nil),
			item(3, "Grey Fang", 0, 0, price),
			item(4, "Grey Thread", 0, 0, price),
			item(5, "Runic Dust", 1, 0, nil),
			item(6, "Old Boots", 2, feetSlot, nil),
			item(7, "Strange Gem", 1, 0, nil),
			item(8, "Grey Totem", 0, 0, price),
			item(10, "Odd Trinket", 1, 0, nil),
			item(11, "Silk Cowl", 3, headSlot, nil),
		}),
		"recipes": table(t, []recipeRow{
			{100, "Tailoring", i16(10), i16(45), i16(70)},    // near
			{101, "Tailoring", i16(120), i16(130), i16(150)}, // far
		}),
		"recipe_reagents": table(t, []reagentRow{{100, 2, 2}, {100, 4, 1}, {100, 11, 1}, {101, 5, 3}, {101, 2, 1}}),
	})
}

func priest() character.Character {
	return character.Character{
		Name: "Testpriest", Class: "PRIEST", Level: ip(15), BagsCaptured: true, BankCaptured: true,
		Equipment:   map[string]character.EquippedItem{"feet": {ItemLevel: ip(55)}},
		Professions: []character.Skill{{Name: "Tailoring", Rank: ip(40), Max: ip(75)}},
		Quests: []character.Quest{
			{QuestID: 1, Title: "Gem Hunt", Objectives: []character.Objective{{Text: "Strange Gem: 0/3", Type: "item", Done: bp(false)}, {Text: "Grey Totem: 1/1", Type: "item", Done: bp(false)}}},
			{QuestID: 2, Title: "Failed Hunt", Failed: bp(true), Objectives: []character.Objective{{Text: "Odd Trinket: 0/1", Type: "item"}}},
		},
		Bags: []character.ItemStack{
			{ItemID: 1, Count: 1, Name: "Cloth Hood"}, {ItemID: 2, Count: 6, Name: "Linen Cloth"}, {ItemID: 3, Count: 3, Name: "Grey Fang"},
			{ItemID: 4, Count: 2, Name: "Grey Thread"}, {ItemID: 5, Count: 4, Name: "Runic Dust"}, {ItemID: 6, Count: 1, Name: "Old Boots"},
			{ItemID: 7, Count: 1, Name: "Strange Gem"}, {ItemID: 8, Count: 1, Name: "Grey Totem"}, {ItemID: 10, Count: 1, Name: "Odd Trinket"},
			{ItemID: 99, Count: 1, Name: "Mystery"}, {ItemID: 11, Count: 1, Name: "Silk Cowl"},
		},
		Bank: []character.ItemStack{{ItemID: 3, Count: 2, Name: "Grey Fang"}},
	}
}

func TestReviewGroups(t *testing.T) {
	r := Review(priest(), synth(t, true))
	got := map[string]Entry{}
	for _, l := range r.Groups {
		for _, e := range l {
			if e.Where == "bags" {
				got[e.Name] = e
			}
		}
	}
	cases := []struct {
		name   string
		group  Group
		reason string
	}{
		{"Cloth Hood", Equip, "empty head slot"},
		{"Silk Cowl", Equip, "empty head slot"}, // also a reagent: Equip wins over Keep
		{"Linen Cloth", Keep, "Tailoring"},
		{"Grey Thread", Keep, "Tailoring"}, // grey and a reagent: Keep wins over Vendor
		{"Strange Gem", Quest, "Gem Hunt"},
		{"Grey Totem", Quest, "Gem Hunt"}, // grey and a quest item: Quest wins over Vendor
		{"Grey Fang", Vendor, "grey"},
		{"Runic Dust", Bank, "more than 50 skill points"},
		{"Old Boots", Bank, "not an upgrade"},
		{"Odd Trinket", Unknown, "no rule"}, // its only quest has failed
		{"Mystery", Unknown, "not in the dataset"},
	}
	for _, c := range cases {
		e, ok := got[c.name]
		if !ok || e.Group != c.group || !strings.Contains(e.Reason, c.reason) {
			t.Errorf("%s = %+v, want %s (%s)", c.name, e, c.group, c.reason)
		}
	}
	if len(got) != len(cases) {
		t.Errorf("%d bag stacks placed, want %d", len(got), len(cases))
	}
}

func TestVendorValueAndSlots(t *testing.T) {
	for _, c := range []struct {
		name     string
		priced   bool
		value    int64
		unpriced int
		freed    int
	}{
		{"priced", true, 25, 0, 2},   // Grey Fang 3 in bags + 2 in the bank
		{"unpriced", false, 0, 2, 2}, // never guessed
	} {
		r := Review(priest(), synth(t, c.priced))
		// vendor: Grey Fang in bags and bank; bank: Runic Dust and Old Boots in the bags
		if r.VendorValue != c.value || r.VendorUnpriced != c.unpriced || r.SlotsFreed != 3 {
			t.Errorf("%s: value %d unpriced %d freed %d", c.name, r.VendorValue, r.VendorUnpriced, r.SlotsFreed)
		}
		text := Render(r)
		want := "Vendor value: 0g 0s 25c"
		if !c.priced {
			want = "Vendor value: unknown"
		}
		if !strings.Contains(text, want) || !strings.Contains(text, "Bag slots freed: 3") {
			t.Errorf("%s render:\n%s", c.name, text)
		}
	}
}

func TestNoDatasetAndNoBank(t *testing.T) {
	c := priest()
	c.BankCaptured = false
	r := Review(c, nil)
	if len(r.Groups[Quest]) != 2 || len(r.Groups[Unknown]) != len(c.Bags)-2+1 || r.SlotsFreed != 0 {
		t.Errorf("groups = %v freed %d", r.Groups, r.SlotsFreed)
	}
	text := Render(r)
	for _, want := range []string{"No dataset configured", "Bank not captured", "Unknown (10)"} {
		if !strings.Contains(text, want) {
			t.Errorf("render lacks %q:\n%s", want, text)
		}
	}
	if got := Render(Review(character.Character{BagsCaptured: true, BankCaptured: true}, nil)); !strings.Contains(got, "No bag or bank items") {
		t.Errorf("empty render = %q", got)
	}
}

func TestMoney(t *testing.T) {
	for in, want := range map[int64]string{0: "0g 0s 0c", 25: "0g 0s 25c", 12345: "1g 23s 45c"} {
		if got := Money(in); got != want {
			t.Errorf("Money(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestToolsAndBagsAreNotBankedGear(t *testing.T) {
	ds := store.New(store.ReaderSource{"items": table(t, []itemRow{
		{ID: 20, Name: "Mining Pick", ItemLevel: 1, Quality: 1, ClassID: 2, SubclassID: 14, Slot: 21, ClassMask: -1, StatsStatus: "none"},
		{ID: 21, Name: "Linen Bag", ItemLevel: 1, Quality: 1, ClassID: 1, Slot: 18, ClassMask: -1, StatsStatus: "none"},
	})})
	c := priest()
	c.Professions = append(c.Professions, character.Skill{Name: "Mining", Rank: ip(10), Max: ip(75)})
	c.Bags = []character.ItemStack{{ItemID: 20, Count: 1, Name: "Mining Pick"}, {ItemID: 21, Count: 1, Name: "Linen Bag"}}
	c.Quests, c.Bank = nil, nil
	r := Review(c, ds)
	if len(r.Groups[Keep]) != 1 || r.Groups[Keep][0].Name != "Mining Pick" || len(r.Groups[Unknown]) != 1 || r.Groups[Unknown][0].Reason != BagReason || r.SlotsFreed != 0 {
		t.Errorf("the tool is kept and the bag unplaced, none freeing a slot: %+v", r)
	}
	alt := priest()
	alt.Name, alt.Bags, alt.Professions = "Alt", nil, nil
	c.Name = "Miner"
	for _, l := range ReviewAll([]character.Character{c, alt}, ds).Lines {
		if l.Item == "Mining Pick" && l.Action != Keep_ || l.Item == "Linen Bag" && l.Action != Unsure {
			t.Errorf("%s: %s (%s)", l.Item, l.Action, l.Reason)
		}
	}
}
