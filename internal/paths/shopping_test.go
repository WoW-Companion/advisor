package paths

import (
	"regexp"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/store"
)

func stats(it itemRow, known ...int16) itemRow {
	it.StatsStatus, it.StatTypes = "known", known
	return it
}

func chain(id int64, name string, ilvl, req, quality, bonding int32) itemRow {
	return item(id, name, ilvl, req, quality, mail, 5, bonding)
}

// manyCreatures makes an item a random world drop: seven creatures, each under 5%.
func manyCreatures(id int64) []lootRow {
	var l []lootRow
	for i := int64(0); i < 7; i++ {
		l = append(l, lootRow{id, "creature", 700 + i, 100, 2, nil})
	}
	return l
}

var priceWords = regexp.MustCompile(`(?i)\b\d+\s?[gsc]\b|\bgold\b|\bsilver\b|\bcopper\b|\bprice|\bcosts?\b`)

// shopStore is chest gear for a level 10 warrior: a world-drop base item with suffix variants (one
// whose stats contradict its suffix), a Bind on Equip tunic, and a Bind on Pickup boss chest.
func shopStore(t *testing.T) store.DataStore {
	var loot []lootRow
	for _, id := range []int64{201, 202, 203, 204, 205} {
		loot = append(loot, manyCreatures(id)...)
	}
	loot = append(loot, lootRow{206, "creature", 600, 100, 20, tier("verified")})
	return store.New(store.ReaderSource{
		"items": tbl(t, []itemRow{
			chain(201, "Ridged Chain Vest", 14, 8, 2, 0),
			stats(chain(202, "Ridged Chain Vest of the Bear", 14, 8, 2, 0), 4, 7),
			stats(chain(203, "Ridged Chain Vest of the Monkey", 14, 8, 2, 0), 3, 7),
			stats(chain(204, "Ridged Chain Vest of the Tiger", 14, 8, 2, 0), 3), // Agility only: not Tiger's stats
			chain(205, "Gnarled Mail Tunic", 16, 9, 2, 2),
			chain(206, "Boss Chestplate", 22, 10, 3, 1),
			chain(207, "Far Vest", 60, 30, 2, 0),
			chain(208, "Grey Vest", 15, 8, 0, 0), // poor quality
		}),
		"loot_sources": tbl(t, append(loot, manyCreatures(207)...)),
		"npcs":         tbl(t, []npcRow{{600, "Boss", 11, 11, "Dunemark"}}),
	})
}

func shopWarrior(role string) character.Character {
	c := warrior()
	c.Professions, c.Bags, c.Bank, c.Quests = nil, nil, nil, nil
	lvl := 5
	c.Equipment = map[string]character.EquippedItem{"chest": {Slot: "chest", ItemLevel: &lvl}}
	if role != "" {
		c.Profile = &character.Profile{Role: role, Focus: "leveling"}
	}
	return c
}

func TestShoppingListForAWarrior(t *testing.T) {
	ds := shopStore(t)
	cases := []struct {
		role string
		want []string
	}{
		{"", []string{"of the Bear (Strength + Stamina)"}}, // Strength is not in the data, Tiger's stats contradict
		{"damage", []string{"of the Bear (Strength + Stamina)"}},
		{"tank", []string{"of the Bear (Strength + Stamina)", "of the Monkey (Agility + Stamina)"}},
	}
	for _, tc := range cases {
		sh, ok := ShoppingList(shopWarrior(tc.role), ds, "", "chest", Options{})
		if !ok {
			t.Fatalf("role %q: no shopping list", tc.role)
		}
		if strings.Join(sh.Suffixes, "|") != strings.Join(tc.want, "|") {
			t.Errorf("role %q: suffixes %q, want %q", tc.role, sh.Suffixes, tc.want)
		}
		if sh.Type != "mail chest" || sh.MinLevel != 8 || sh.MaxLevel != 10 || sh.Quality != "green or better" {
			t.Errorf("role %q: %+v", tc.role, sh)
		}
		if len(sh.Examples) == 0 || len(sh.Examples) > 3 {
			t.Errorf("role %q: examples %q", tc.role, sh.Examples)
		}
		for _, e := range sh.Examples {
			if e != "Ridged Chain Vest" && e != "Gnarled Mail Tunic" {
				t.Errorf("role %q: example %q is not a base item in range", tc.role, e)
			}
		}
	}
	sh, _ := ShoppingList(shopWarrior(""), ds, "", "chest", Options{})
	if want := "Chest: any green mail chest, level 8–10, 'of the Bear' — cheap on the Auction House"; sh.Line != want {
		t.Errorf("line = %q", sh.Line)
	}
	if r := sh.Result(); r.Paths[0].Kind != Auction || r.Shopping == nil {
		t.Errorf("result: %+v", r)
	}
}

func TestShoppingListNeverHasAPriceOrAnId(t *testing.T) {
	sh, ok := ShoppingList(shopWarrior("tank"), shopStore(t), "", "chest", Options{})
	if !ok {
		t.Fatal("no list")
	}
	text := sh.Result().Text()
	if m := priceWords.FindString(text); m != "" {
		t.Errorf("price words %q in %s", m, text)
	}
	if !strings.Contains(text, "usually cheap") || !strings.Contains(text, "of the Bear") || !strings.Contains(text, "level range") {
		t.Errorf("search steps lack the search: %s", text)
	}
	for _, id := range []int64{201, 205, 206} {
		if m := priceWords.FindString(For(shopWarrior(""), shopStore(t), id, Options{}).Text()); m != "" {
			t.Errorf("item %d: price words %q", id, m)
		}
	}
}

func TestShoppingOmitsUnverifiedSuffixes(t *testing.T) {
	ds := store.New(store.ReaderSource{
		"items":        tbl(t, []itemRow{chain(201, "Ridged Chain Vest", 14, 8, 2, 0)}),
		"loot_sources": tbl(t, manyCreatures(201)),
	})
	sh, ok := ShoppingList(shopWarrior("tank"), ds, "", "chest", Options{})
	if !ok || len(sh.Suffixes) != 0 || strings.Contains(sh.Line, "'of") {
		t.Errorf("no suffix item in the data: %+v %v", sh, ok)
	}
}

func TestShoppingOnlyWhereNoBetterWayExists(t *testing.T) {
	ds, c := shopStore(t), shopWarrior("")
	res, _ := gear.Find(c, ds, "", gear.Options{Slot: "chest", NoSources: true})
	cands := res.Slots["chest"]
	if _, ok := Shop(c, ds, "chest", cands, Quest); ok {
		t.Error("a quest in reach beats shopping")
	}
	if _, ok := Shop(c, ds, "chest", cands, Craft); ok {
		t.Error("a craft beats shopping")
	}
	if _, ok := Shop(c, ds, "chest", cands, Dungeon); !ok {
		t.Error("a dungeon is not better than the Auction House")
	}
}

func TestAuctionPathRules(t *testing.T) {
	ds, c := shopStore(t), shopWarrior("")
	cases := []struct {
		id   int64
		want string
	}{
		{201, "Common world drop. Usually cheap on the Auction House."},
		{205, "Common world drop. Usually cheap on the Auction House."}, // BoE and a world drop
		{206, ""}, // Bind on Pickup world drop stays out
		{208, ""}, // poor quality
	}
	for _, tc := range cases {
		r := For(c, ds, tc.id, Options{})
		got := ""
		for _, p := range r.Paths {
			if p.Kind == Auction {
				got = p.Line
			}
		}
		if got != tc.want {
			t.Errorf("item %d: auction line %q, want %q", tc.id, got, tc.want)
		}
	}
	// Bind on Equip from a single named creature is also bought, not called a world drop.
	boe := store.New(store.ReaderSource{
		"items":        tbl(t, []itemRow{chain(1, "Wolf Hauberk", 15, 8, 2, 2)}),
		"loot_sources": tbl(t, []lootRow{{1, "creature", 7, 100, 40, tier("verified")}}),
	})
	r := For(c, boe, 1, Options{})
	if len(r.Paths) == 0 || r.Paths[0].Kind != Auction || strings.Contains(r.Paths[0].Line, "world drop") || !strings.Contains(r.Paths[0].Line, "Bind on Equip") {
		t.Errorf("Bind on Equip, one creature: %+v", r.Paths)
	}
}

func TestWaysAreRankedWithTheAuctionHouseInTheMiddle(t *testing.T) {
	boe := func(id int64, name string) itemRow { return chain(id, name, 15, 8, 2, 2) }
	ds := store.New(store.ReaderSource{
		"items": tbl(t, []itemRow{
			boe(1, "Logged Quest Chest"), boe(2, "Unstarted Quest Chest"), boe(3, "Ready Craft Chest"), boe(4, "Short Craft Chest"),
			item(10, "Copper Bar", 1, 0, 1, 0, 0, 0),
		}),
		"loot_sources": tbl(t, []lootRow{
			{1, "quest", 500, 0, 0, nil}, {1, "creature", 600, 100, 20, tier("verified")},
			{2, "quest", 501, 0, 0, nil},
			{3, "creature", 800, 100, 1, nil}, {4, "creature", 801, 100, 1, nil},
			{10, "vendor", 1, 0, 0, nil},
		}),
		"npcs":            tbl(t, []npcRow{{600, "Hogger Prime", 12, 12, "Dunemark"}}),
		"quests":          tbl(t, []questRow{{500, "Lost Shawl", 9, "Dunemark"}, {501, "Fresh Errand", 9, "Dunemark"}}),
		"recipes":         tbl(t, []recipeRow{{810, "Ready Craft Chest", "Blacksmithing", 5, 3}, {811, "Short Craft Chest", "Blacksmithing", 5, 4}}),
		"recipe_reagents": tbl(t, []reagentRow{{810, 10, 3}, {811, 10, 9}}),
		"instances":       tbl(t, []instRow{{30, "Wolf Den", "dungeon", 8, 14}}),
		"instance_bosses": tbl(t, []bossRow{{30, "Hogger Prime", 600}}),
	})
	c := warrior() // quest 500 is in the log; three Copper Bars in bags and bank
	want := map[int64]string{
		1: "quest,auction,dungeon,creature", // a logged quest, then the Auction House, then the boss
		2: "auction,quest",                  // a quest not yet picked up comes after
		3: "craft,auction",                  // every material on hand
		4: "auction,craft",                  // materials missing
	}
	for id, w := range want {
		if got := kinds(For(c, ds, id, Options{})); got != w {
			t.Errorf("item %d: ways %q, want %q", id, got, w)
		}
	}
}
