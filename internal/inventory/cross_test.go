package inventory

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/store"
)

type xItem struct {
	ID            int64   `parquet:"id"`
	Name          string  `parquet:"name"`
	ItemLevel     int32   `parquet:"item_level"`
	RequiredLevel int32   `parquet:"required_level"`
	Quality       int32   `parquet:"quality"`
	ClassID       int32   `parquet:"class_id"`
	SubclassID    int32   `parquet:"subclass_id"`
	Slot          int32   `parquet:"slot"`
	ClassMask     int32   `parquet:"allowable_class_mask"`
	Bonding       int32   `parquet:"bonding"`
	StatTypes     []int16 `parquet:"stat_types,list"`
	StatsStatus   string  `parquet:"stats_status"`
	SellPrice     *int64  `parquet:"sell_price,optional"`
}

func xi(id int64, name string, q, class, sub int32, price *int64) xItem {
	return xItem{ID: id, Name: name, ItemLevel: 20, RequiredLevel: 5, Quality: q, ClassID: class, SubclassID: sub,
		ClassMask: -1, StatTypes: []int16{5}, StatsStatus: "known", SellPrice: price}
}

// crossDS: Linen Cloth is tailoring cloth, Iron Ore only used by Blacksmithing, Copper Bar by
// Blacksmithing, Troll Tooth a grey with no price, Wolf Pelt a quest item, Mystery Herb unknown.
func crossDS(t *testing.T) store.DataStore {
	p := int64(7)
	return store.New(store.ReaderSource{
		"items": table(t, []xItem{
			xi(2, "Linen Cloth", 1, 7, 5, nil),
			xi(20, "Copper Bar", 1, 7, 1, nil),
			xi(21, "Elemental Fire", 1, 7, 2, &p), // used only by Alchemy, which no one has
			xi(30, "Troll Tooth", 0, 15, 0, nil),
			xi(31, "Wolf Pelt", 1, 12, 0, nil),
			xi(32, "Fang Of Doom", 0, 15, 0, &p),
		}),
		"recipes": table(t, []recipeRow{
			{100, "Tailoring", i16(1), i16(10), i16(30)},
			{200, "Blacksmithing", i16(1), i16(10), i16(30)},
			{300, "Alchemy", i16(1), i16(10), i16(30)},
		}),
		"recipe_reagents": table(t, []reagentRow{{100, 2, 2}, {200, 20, 1}, {300, 21, 1}}),
	})
}

func chars() []character.Character {
	smith := character.Character{Name: "Smithy", Class: "WARRIOR", Level: ip(30), BagsCaptured: true, BankCaptured: true,
		Professions: []character.Skill{{Name: "Blacksmithing", Rank: ip(20), Max: ip(75)}},
		Bags:        []character.ItemStack{{ItemID: 2, Count: 12, Name: "Linen Cloth"}, {ItemID: 21, Count: 2, Name: "Elemental Fire"}, {ItemID: 30, Count: 4, Name: "Troll Tooth"}, {ItemID: 31, Count: 3, Name: "Wolf Pelt"}, {ItemID: 99, Count: 1, Name: "Mystery Herb"}}}
	tailor := character.Character{Name: "Taila", Class: "PRIEST", Level: ip(20), BagsCaptured: true, BankCaptured: false,
		Professions: []character.Skill{{Name: "Tailoring", Rank: ip(20), Max: ip(75)}},
		Bags:        []character.ItemStack{{ItemID: 2, Count: 5, Name: "Linen Cloth"}, {ItemID: 20, Count: 6, Name: "Copper Bar"}, {ItemID: 32, Count: 1, Name: "Fang Of Doom"}}}
	smith.Bank = []character.ItemStack{{ItemID: 2, Count: 40, Name: "Linen Cloth"}}
	return []character.Character{smith, tailor}
}

func find(l []Line, who, item string) Line {
	for _, x := range l {
		if x.Character == who && x.Item == item {
			return x
		}
	}
	return Line{}
}

func TestFindAcrossCharactersAndUnseenBank(t *testing.T) {
	r := Find(chars(), "linen cloths", nil)
	if len(r.Items) != 1 || r.Items[0].Total != 57 || len(r.Items[0].Holdings) != 3 {
		t.Fatalf("find = %+v", r.Items)
	}
	text := RenderFind(r)
	for _, want := range []string{"Smithy: 40 in the bank", "Taila: 5 in the bags", "Taila: bank not seen yet"} {
		if !strings.Contains(text, want) {
			t.Errorf("lacks %q:\n%s", want, text)
		}
	}
	if got := RenderFind(Find(chars(), "unobtainium", nil)); !strings.Contains(got, "No character has") || !strings.Contains(got, "bank not seen yet") {
		t.Errorf("no match = %q", got)
	}
}

func TestReviewAll(t *testing.T) {
	r := ReviewAll(chars(), crossDS(t))
	cases := []struct {
		who, item string
		action    Action
		to, why   string
	}{
		{"Smithy", "Linen Cloth", Mail, "Taila", "Tailoring"},      // reagent for an alt's profession
		{"Taila", "Copper Bar", Mail, "Smithy", "Blacksmithing"},   // and the other way
		{"Smithy", "Elemental Fire", Sell, "", "no character has"}, // no one has Alchemy
		{"Smithy", "Wolf Pelt", Toss, "", "safe to destroy"},       // quest item, no quest anywhere
		{"Smithy", "Troll Tooth", Sell, "", "grey"},                // grey
		{"Smithy", "Mystery Herb", Unsure, "", "does not know"},    // never guessed
	}
	for _, c := range cases {
		l := find(r.Lines, c.who, c.item)
		if l.Action != c.action || l.To != c.to || !strings.Contains(l.Reason, c.why) {
			t.Errorf("%s/%s = %+v, want %s %s %q", c.who, c.item, l, c.action, c.to, c.why)
		}
	}
	// Elemental Fire is not grey and has no bonding column value 0 => Auction House advice
	if l := find(r.Lines, "Smithy", "Elemental Fire"); !l.AH || !strings.Contains(l.Reason, "Auction House") {
		t.Errorf("tradeable reagent: %+v", l)
	}
	if len(r.Notes) == 0 || !strings.Contains(strings.Join(r.Notes, " "), "Taila: bank not seen yet") {
		t.Errorf("notes = %v", r.Notes)
	}
}

func TestQuestItemMailedToQuestHolder(t *testing.T) {
	cs := chars()
	cs[1].Quests = []character.Quest{{QuestID: 9, Title: "Pelts", Objectives: []character.Objective{{Text: "Wolf Pelt: 0/3", Type: "item", Done: bp(false)}}}}
	r := ReviewAll(cs, crossDS(t))
	// Taila's own quest keeps no stack of hers, but Smithy's pelts go to her
	if l := find(r.Lines, "Smithy", "Wolf Pelt"); l.Action != Mail || l.To != "Taila" {
		t.Errorf("pelt = %+v", l)
	}
}

func TestUnknownPricesNeverGuessed(t *testing.T) {
	r := ReviewAll(chars(), crossDS(t))
	// Troll Tooth (grey) has no price, Fang of Doom 7; Elemental Fire is Auction House advice: no vendor value
	if r.SellValue != 7 || r.SellUnpriced != 1 {
		t.Errorf("value %d unpriced %d", r.SellValue, r.SellUnpriced)
	}
	text := RenderAll(r, 2)
	if !strings.Contains(text, "Sell (3): 0g 0s 7c (1 stack(s) unpriced, left out)") || !strings.Contains(text, "sell price unknown") {
		t.Errorf("render:\n%s", text)
	}
	// Smithy's three Fangs add 21 to Taila's one: 28
	cs := chars()
	cs[0].Bags = []character.ItemStack{{ItemID: 32, Count: 3, Name: "Fang Of Doom"}}
	r = ReviewAll(cs, crossDS(t))
	if r.SellValue != 28 || !strings.Contains(RenderAll(r, 2), "0g 0s 28c") {
		t.Errorf("priced value = %d", r.SellValue)
	}
}

func TestMailForNamedCharacter(t *testing.T) {
	r, name, ok := MailFor(chars(), "smithy", crossDS(t))
	if !ok || name != "Smithy" || len(r.Lines) != 1 || r.Lines[0].Item != "Copper Bar" {
		t.Fatalf("mail = %+v %q %v", r.Lines, name, ok)
	}
	if text := RenderMail(r, name); !strings.Contains(text, "Copper Bar x6 from Taila") {
		t.Errorf("render = %q", text)
	}
	if _, _, ok := MailFor(chars(), "nobody", crossDS(t)); ok {
		t.Error("unknown character found")
	}
}

func TestBestFoodKeptRestSold(t *testing.T) {
	ds := store.New(store.ReaderSource{
		"items": table(t, []xItem{
			{ID: 50, Name: "Bread", ItemLevel: 5, RequiredLevel: 1, Quality: 1, ClassID: 0, SubclassID: 5, ClassMask: -1, StatsStatus: "known"},
			{ID: 51, Name: "Stew", ItemLevel: 15, RequiredLevel: 15, Quality: 1, ClassID: 0, SubclassID: 5, ClassMask: -1, StatsStatus: "known"},
			{ID: 52, Name: "Feast", ItemLevel: 45, RequiredLevel: 45, Quality: 1, ClassID: 0, SubclassID: 5, ClassMask: -1, StatsStatus: "known"},
		}),
		"recipes":         table(t, []recipeRow{{1, "Tailoring", nil, nil, nil}}),
		"recipe_reagents": table(t, []reagentRow{{1, 2, 1}}),
	})
	c := character.Character{Name: "Eater", Class: "MAGE", Level: ip(20), BagsCaptured: true, BankCaptured: true,
		Bags: []character.ItemStack{{ItemID: 50, Count: 5, Name: "Bread"}, {ItemID: 51, Count: 5, Name: "Stew"}, {ItemID: 52, Count: 5, Name: "Feast"}}}
	r := ReviewAll([]character.Character{c}, ds)
	for item, want := range map[string]Action{"Bread": Sell, "Stew": Keep_, "Feast": Keep_} {
		if l := find(r.Lines, "Eater", item); l.Action != want {
			t.Errorf("%s = %+v, want %s", item, l, want)
		}
	}
}

// A Bind on Pickup piece is bound to its holder: an alt who could wear it is never sent it by mail, a Bind on Equip one is.
func TestBoundOnPickupGearNotMailed(t *testing.T) {
	boe, bop := xi(40, "Silk Mantle", 2, 4, 1, nil), xi(41, "Silk Cowl", 3, 4, 1, nil)
	boe.Slot, bop.Slot, boe.Bonding, bop.Bonding = 3, 1, 2, 1
	boe.RequiredLevel, bop.RequiredLevel = 28, 28
	price := int64(90)
	boe.SellPrice, bop.SellPrice = &price, &price
	ds := store.New(store.ReaderSource{"items": table(t, []xItem{boe, bop}), "recipes": table(t, []recipeRow{}), "recipe_reagents": table(t, []reagentRow{})})
	holder := character.Character{Name: "Wara", Class: "WARRIOR", Level: ip(5), BagsCaptured: true, BankCaptured: true,
		Bags: []character.ItemStack{{ItemID: 40, Count: 1, Name: "Silk Mantle"}, {ItemID: 41, Count: 1, Name: "Silk Cowl"}}}
	alt := character.Character{Name: "Magea", Class: "MAGE", Level: ip(30), BagsCaptured: true, BankCaptured: true}
	r := ReviewAll([]character.Character{holder, alt}, ds)
	if l := find(r.Lines, "Wara", "Silk Mantle"); l.Action != Mail || l.To != "Magea" {
		t.Errorf("Bind on Equip piece = %+v, want mail to Magea", l)
	}
	l := find(r.Lines, "Wara", "Silk Cowl")
	if l.Action != Sell || l.To != "" || l.AH || !strings.Contains(l.Reason, "cannot be mailed") {
		t.Errorf("Bind on Pickup piece = %+v, want a vendor sale that says it cannot be mailed", l)
	}
}
