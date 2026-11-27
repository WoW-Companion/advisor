package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/quests"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline key, key ring, quiver, ammo pouch, ammo, class quest chain and mailbox benchmark: what the advisor does with a key in the bags, which
// ammo it keeps for the ranged weapon a character wields, what it says of a quiver's size, of a class's quests and of the mail, beside the dataset's rows and the
// game's own rules, with no network. It reads testdata/gating_snapshot (the key items, class 13), testdata/usability_snapshot (ammo, quivers, ammo pouches and the
// bows, guns and crossbows) and testdata/places_snapshot (quests_observed with the class a player saw a quest under, quest_lines). The dataset holds no key to
// door link, no key ring, quiver or pouch size, no ammo effect, no class quest line, prerequisite or reward and no mail rule, so the cases of those check that the
// answer is framed with the line that says so (ask.LimitFacts) and that nothing is stated from memory. A difference the dataset's own rows cause is marked
// "(data)". make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	kaKeySrc    = "dataset items (class_id 13 key; bonding, quality; no row links a key to a door or dungeon); game: keys sit on the key ring and are never destroyed for space (Warcraft Wiki, Key ring)"
	kaKeyBindSr = "dataset items (class_id 13, bonding 1 Bind on Pickup or 4 quest); game: a soulbound or quest item cannot be traded or mailed (Warcraft Wiki, Bind on Pickup)"
	kaAmmoSrc   = "dataset items (ranged weapons class_id 2 subclass_id 2 bow, 3 gun, 18 crossbow; ammo class_id 6 subclass_id 2 arrow, 3 bullet); game: bows and crossbows fire arrows, guns fire bullets (Warcraft Wiki, Ammunition)"
	kaSizeSrc   = "dataset items (class_id 11 quiver and ammo pouch, class_id 1 bag; no slot count, ammo speed or capacity column); game: size and ammo speed are in item tooltips, not in the data"
	kaClassSrc  = "dataset quests_observed (title, level, faction, category = the class a player saw the quest under; no giver, turn-in or follow-up for these), quest_lines (three lines, none a class's); game: class quests are listed by class on Warcraft Wiki"
	kaMailSrc   = "dataset items (bonding); no mail table or column (items per mail, postage, cash on delivery, time kept); game: mail is kept for thirty days, soulbound items cannot be mailed (Warcraft Wiki, Mail)"
)

// kaKeyChar is a character in the bags of whom are the named keys, from the gating snapshot.
func kaKeyChar(t *testing.T, name, class string, keys ...string) character.Character {
	t.Helper()
	c := usabChar(class, 60, "Horde")
	c.Name, c.BagsCaptured, c.BankCaptured, c.Equipment = name, true, true, map[string]character.EquippedItem{}
	for _, k := range keys {
		c.Bags = append(c.Bags, character.ItemStack{ItemID: int(num(rowByName(t, gateStore(), "items", k)["id"])), Count: 1, Name: k})
	}
	return c
}

func kaLine(res inventory.CrossResult, who, item string) (inventory.Line, bool) {
	for _, l := range res.Lines {
		if l.Character == who && l.Item == item {
			return l, true
		}
	}
	return inventory.Line{}, false
}

// ---- keys and the key ring ----

var kaKeys = []string{"Shadowforge Key", "Skeleton Key", "Key to the City", "Prison Cell Key", "Crescent Key", "Viewing Room Key", "Gordok Courtyard Key"}

// TestOfflineBenchmarkKeyReview: a key in the bags is kept, whatever its binding: a dungeon key is no quest item to destroy and no vendor trash, and a key bound to its holder is not
// mailed to an alt. The review before the fix tossed the keys the dataset binds as quest items and left the rest "not sure".
func TestOfflineBenchmarkKeyReview(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, k := range kaKeys {
		row := rowByName(t, ds, "items", k)
		single := inventory.Review(kaKeyChar(t, "Magea", "MAGE", k), ds)
		var g inventory.Group
		var why string
		for gr, es := range single.Groups {
			for _, e := range es {
				g, why = gr, e.Reason
			}
		}
		out = append(out, benchRow{k + " in a mage's bags: the bag review", fmt.Sprintf("%s: %s", g, why), fmt.Sprintf("item class %d (key), bonding %d: kept", num(row["class_id"]), num(row["bonding"])), kaKeySrc,
			resultIf(g == inventory.Keep && why == inventory.KeyReason, "a key was not kept")})
		res := inventory.ReviewAll([]character.Character{kaKeyChar(t, "Magea", "MAGE", k), kaKeyChar(t, "Rogb", "ROGUE")}, ds)
		l, ok := kaLine(res, "Magea", k)
		out = append(out, benchRow{k + ": the review across a mage and an alt rogue", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "keep: a key is neither destroyed, sold nor mailed", kaKeySrc,
			resultIf(ok && l.Action == inventory.Keep_ && l.To == "" && res.SlotsFreed == 0, "a key was tossed, sold, mailed or left unsure")})
	}
	finish(t, "advice-key-review", "Offline keys: a key in the bags is kept", out)
}

// TestOfflineBenchmarkKeyBankAndQuest: a key in the bank is kept too, and a key a quest in the log asks for is kept for that quest, not for its key class.
func TestOfflineBenchmarkKeyBankAndQuest(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	c := kaKeyChar(t, "Magea", "MAGE")
	id := int(num(rowByName(t, ds, "items", "Shadowforge Key")["id"]))
	c.Bank = []character.ItemStack{{ItemID: id, Count: 1, Name: "Shadowforge Key"}}
	res := inventory.ReviewAll([]character.Character{c}, ds)
	l, ok := kaLine(res, "Magea", "Shadowforge Key")
	out = append(out, benchRow{"Shadowforge Key in the bank: the review", fmt.Sprintf("%s: %s", l.Action, l.Reason), "keep (the bank holds it already)", kaKeySrc, resultIf(ok && l.Action == inventory.Keep_ && l.Where == "bank", "a banked key was tossed")})
	q := kaKeyChar(t, "Magea", "MAGE", "Crescent Key")
	q.Quests = []character.Quest{{QuestID: 1, Title: "A Door Unlocked", Objectives: []character.Objective{{Text: "Crescent Key: 0/1", Type: "item"}}}}
	res = inventory.ReviewAll([]character.Character{q}, ds)
	l, ok = kaLine(res, "Magea", "Crescent Key")
	out = append(out, benchRow{"Crescent Key a quest in the log asks for: the review", fmt.Sprintf("%s: %s", l.Action, l.Reason), "keep, for the quest", kaKeySrc, resultIf(ok && l.Action == inventory.Keep_ && strings.Contains(l.Reason, "A Door Unlocked"), "a quest's key was not kept for the quest")})
	finish(t, "advice-key-bank-quest", "Offline keys: a banked key and a quest's key are kept", out)
}

// TestOfflineBenchmarkKeyTooltip: a key's tooltip is its binding: a key the dataset binds on pickup or to a quest is said to be untradeable, so it is not offered to an alt.
func TestOfflineBenchmarkKeyTooltip(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	for _, k := range kaKeys {
		row := rowByName(t, ds, "items", k)
		f := ask.ItemTooltipFacts(ds, "can I mail ["+k+"] to my alt?")
		can := strings.Contains(f, "cannot be traded, mailed")
		out = append(out, benchRow{k + ": can it be mailed", strings.TrimPrefix(strings.ReplaceAll(f, "\n", " "), "Item tooltips (from the item rows; answer from these, not from memory): "), fmt.Sprintf("bonding %d: cannot be traded or mailed", num(row["bonding"])), kaKeyBindSr,
			resultIf(can && strings.Contains(f, k), "a bound key was said to be mailable")})
	}
	finish(t, "advice-key-binding", "Offline keys: a bound key cannot be mailed", out)
}

// TestOfflineBenchmarkKeyDatasetNotes: what the dataset files under keys, and what it leaves out: the key class holds seven items of the snapshot and none is linked to a door, a
// dungeon or a source; the four lockpicking Skeleton Keys are filed as class 0.
func TestOfflineBenchmarkKeyDatasetNotes(t *testing.T) {
	ds := gateStore()
	var out []benchRow
	rows, _ := ds.Filter("items", store.Range{Column: "class_id", Min: int64(13), Max: int64(13)})
	linked := 0
	for _, r := range rows {
		loot, _ := ds.Filter("loot_sources", store.Range{Column: "item_id", Min: num(r["id"]), Max: num(r["id"])})
		vend, _ := ds.Filter("vendor_sources", store.Range{Column: "item_id", Min: num(r["id"]), Max: num(r["id"])})
		linked += len(loot) + len(vend)
	}
	out = append(out, benchRow{"class 13 key items: a loot or vendor source", fmt.Sprintf("%d keys, %d source rows", len(rows), linked), "no key has a source row", kaKeySrc, resultIf(len(rows) >= 7 && linked == 0, "a key has a source row the advisor never reads")})
	cols := brColumns(ds, "items", "id")
	bad := brHasColumn(cols, "opens", "lock", "door", "unlocks")
	out = append(out, benchRow{"items: a column that says what a key opens", fmt.Sprintf("%d columns: none names a door", len(cols)), "no such column", kaKeySrc, resultIf(bad == "", "a column names "+bad)})
	for _, k := range []string{"Silver Skeleton Key", "Golden Skeleton Key"} {
		trade := newTradeDS(t)
		r := rowByName(t, trade, "items", k)
		out = append(out, benchRow{k + ": filed as a key", fmt.Sprintf("class %d, subclass %d", num(r["class_id"]), num(r["subclass_id"])), "class 13 (Key), as Skeleton Key is", kaKeySrc,
			"differ (data): the dataset files the lockpicking skeleton keys as class 0 subclass 8, not as keys; the advisor places them as it places any consumable"})
	}
	finish(t, "advice-key-dataset-notes", "Offline keys: what the dataset links and files", out)
}

func newTradeDS(t *testing.T) store.DataStore {
	t.Helper()
	return tsStore()
}

// ---- ammo and the ranged weapon ----

// kaRanged are the ranged weapons the snapshot holds, by the ammo each fires.
var kaRanged = map[string]string{"Worn Shortbow": "arrows", "Light Crossbow": "arrows", "Solid Blunderbuss": "bullets"}

var kaAmmoCases = []struct {
	class, weapon, ammo string
	level               int
	want                inventory.Action
	says                string
}{
	{"HUNTER", "Worn Shortbow", "Razor Arrow", 30, inventory.Keep_, "the best arrows"},
	{"HUNTER", "Light Crossbow", "Razor Arrow", 30, inventory.Keep_, "the best arrows"},
	{"HUNTER", "Solid Blunderbuss", "Crafted Heavy Shot", 30, inventory.Keep_, "the best bullets"},
	{"HUNTER", "Solid Blunderbuss", "Razor Arrow", 30, inventory.Unsure, "fires bullets, not arrows"},
	{"HUNTER", "Worn Shortbow", "Crafted Heavy Shot", 30, inventory.Unsure, "fires arrows, not bullets"},
	{"HUNTER", "Light Crossbow", "Crafted Heavy Shot", 30, inventory.Unsure, "fires arrows, not bullets"},
	{"WARRIOR", "Solid Blunderbuss", "Explosive Arrow", 40, inventory.Unsure, "fires bullets, not arrows"},
	{"ROGUE", "Light Crossbow", "Explosive Arrow", 40, inventory.Keep_, "the best arrows"},
	{"HUNTER", "", "Razor Arrow", 30, inventory.Keep_, "the best arrows"},
	{"HUNTER", "", "Crafted Heavy Shot", 30, inventory.Keep_, "the best bullets"},
}

// TestOfflineBenchmarkAmmoRangedWeapon: ammo is kept for the ranged weapon the character wields: arrows for a bow or a crossbow, bullets for a gun; the other kind is "not sure" (the
// player may change weapon) with the reason, never kept as the best ammo to use. A character with no ranged weapon in the capture keeps both kinds as before.
func TestOfflineBenchmarkAmmoRangedWeapon(t *testing.T) {
	var out []benchRow
	for _, tc := range kaAmmoCases {
		ds := usabStore()
		c := usabChar(tc.class, tc.level, "Horde")
		c.Name, c.BagsCaptured, c.BankCaptured, c.Equipment = "Benchtest", true, true, map[string]character.EquippedItem{}
		ref := "no ranged weapon in the capture: both kinds kept"
		if tc.weapon != "" {
			w := usabItem(t, tc.weapon)
			id := int(num(w["id"]))
			c.Equipment["ranged"] = character.EquippedItem{Slot: "ranged", ItemID: &id, Name: tc.weapon}
			ref = fmt.Sprintf("%s (subclass %d) fires %s", tc.weapon, num(w["subclass_id"]), kaRanged[tc.weapon])
		}
		a := usabItem(t, tc.ammo)
		c.Bags = []character.ItemStack{{ItemID: int(num(a["id"])), Count: 200, Name: tc.ammo}}
		res := inventory.ReviewAll([]character.Character{c}, lgItemsOnly{ds})
		got := "no line"
		ok := false
		if len(res.Lines) == 1 {
			l := res.Lines[0]
			got = fmt.Sprintf("%s: %s", l.Action, l.Reason)
			ok = l.Action == tc.want && strings.Contains(l.Reason, tc.says)
		}
		out = append(out, benchRow{fmt.Sprintf("%s level %d, %s: %s (ammo subclass %d)", strings.ToLower(tc.class), tc.level, map[bool]string{true: tc.weapon, false: "no ranged weapon"}[tc.weapon != ""], tc.ammo, num(a["subclass_id"])), got,
			fmt.Sprintf("%s: %s", tc.want, ref), kaAmmoSrc, resultIf(ok, "the action or the reason: ammo the weapon cannot fire")})
	}
	finish(t, "advice-ammo-ranged-weapon", "Offline ammo: kept for the ranged weapon wielded", out)
}

// TestOfflineBenchmarkAmmoRangedWeaponAlt: ammo a wielded gun cannot fire is still mailed to an alt who can use it, and a gun hunter's own bullets are kept.
func TestOfflineBenchmarkAmmoRangedWeaponAlt(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	w := usabItem(t, "Worn Shortbow")
	wid := int(num(w["id"]))
	a := usabItem(t, "Razor Arrow")
	mk := func(name, class string, lv int, bow bool, item store.Row) character.Character {
		c := usabChar(class, lv, "Horde")
		c.Name, c.BagsCaptured, c.BankCaptured, c.Equipment = name, true, true, map[string]character.EquippedItem{}
		if bow {
			c.Equipment["ranged"] = character.EquippedItem{Slot: "ranged", ItemID: &wid, Name: "Worn Shortbow"}
		}
		if item != nil {
			c.Bags = []character.ItemStack{{ItemID: int(num(item["id"])), Count: 100, Name: "Razor Arrow"}}
		}
		return c
	}
	res := inventory.ReviewAll([]character.Character{mk("Magea", "MAGE", 40, false, a), mk("Huntb", "HUNTER", 40, true, nil)}, lgItemsOnly{ds})
	l, ok := kaLine(res, "Magea", "Razor Arrow")
	out = append(out, benchRow{"a mage's arrows, an alt hunter wielding a bow", fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason), "mail to the hunter: a bow fires arrows", kaAmmoSrc, resultIf(ok && l.Action == inventory.Mail && l.To == "Huntb", "the arrows were not passed on")})
	res = inventory.ReviewAll([]character.Character{mk("Magea", "MAGE", 40, false, a)}, lgItemsOnly{ds})
	l, ok = kaLine(res, "Magea", "Razor Arrow")
	out = append(out, benchRow{"a mage's arrows, no alt", fmt.Sprintf("%s: %s", l.Action, l.Reason), "sell: a mage has no ranged weapon that fires ammo", kaAmmoSrc, resultIf(ok && l.Action == inventory.Sell, "a mage kept arrows")})
	finish(t, "advice-ammo-alt", "Offline ammo: passed on to an alt who can fire it", out)
}

// TestOfflineBenchmarkQuiverSizeNoAnswer: how many slots a quiver, an ammo pouch or a class bag holds, and which is the biggest, is not in the dataset: the answer is framed with the bag size
// line. Before the fix only a "bag" or "key ring" question got it, so a quiver question was answered from memory.
func TestOfflineBenchmarkQuiverSizeNoAnswer(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"how many slots does the Heavy Quiver have?", "what is the biggest quiver?", "does my quiver have more slots than my ammo pouch?", "how big is a Thick Leather Ammo Pouch?",
		"how many slots does a soul bag have?", "what is the largest herb bag?", "how many arrows fit in the Quickdraw Quiver?", "is the Heavy Leather Ammo Pouch bigger than the Small Ammo Pouch?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f), "no slot count, size or capacity column on quivers, pouches or bags", kaSizeSrc,
			resultIf(vtSays(f, "Bag and bank size:") && strings.Contains(f, "quiver"), "no line saying the data holds no quiver size, so one may be made up from memory")})
	}
	finish(t, "advice-quiver-size", "Offline quivers and ammo pouches: the data holds no size", out)
}

// TestOfflineBenchmarkQuiverSizeScope: a question about a quiver or a bag that asks for a row the data has (who can use it, its price, its level, where it is made) gets no size line.
func TestOfflineBenchmarkQuiverSizeScope(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"can a mage use a quiver?", "what level do I need for the Heavy Quiver?", "where do I buy an ammo pouch?", "which profession makes a Quickdraw Quiver?",
		"is the Heavy Quiver soulbound?", "what arrows should a hunter buy?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f) + "(no size line)", "the question asks for a class, a level, a price, a source or a binding: rows the data has", kaSizeSrc,
			resultIf(!strings.Contains(f, "Bag and bank size:"), "the size line was added to a question the data answers")})
	}
	finish(t, "advice-quiver-size-scope", "Offline quivers and ammo pouches: questions the data answers get no size line", out)
}

// ---- class quest chains ----

// kaClassQuests are class quests the dataset holds (quests_observed, category the class): warlock 1507 and 1513, the warlock dungeon quest 1740, a paladin's 98601.
var kaClassQuests = []int64{1507, 1513, 1740, 98601}

// TestOfflineBenchmarkClassQuestChainFacts: a question about a class's quests or quest chain gets the line that says the data holds no class quest line, prerequisite or reward, and names none.
func TestOfflineBenchmarkClassQuestChainFacts(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what is the warlock class quest chain?", "which class quests does a paladin do at level 4?", "what is the hunter pet quest chain?", "list the rogue quest line from level 10",
		"which quests do I do as a druid to learn Cat Form?", "what is the order of the priest class quests?", "what is the warrior quest chain for Charge?", "what do I get from the mage quests in Stormwind?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f), "no class quest line, prerequisite or reward in the data: say so, name none", kaClassSrc,
			resultIf(vtSays(f, "Class quest chains:") && strings.Contains(f, "name no class quest"), "no line saying the data holds no class quest chain, so one may be made up from memory")})
	}
	ds := qbStore()
	for _, id := range kaClassQuests {
		c := quests.Of(ds, id)
		out = append(out, benchRow{fmt.Sprintf("quest %d: a quest line", id), fmt.Sprintf("found %v, %d steps", c.Found, len(c.Steps)), "quest_lines holds no row for a class quest", kaClassSrc, resultIf(!c.Found && len(c.Steps) == 0, "a line was made up for a class quest")})
	}
	finish(t, "advice-class-quest-chain-facts", "Offline class quest chains: the data holds none, and the answer says so", out)
}

// TestOfflineBenchmarkClassQuestChainScope: a question about the quests of a zone or a named line, a class's spells or its trainer gets no class quest line.
func TestOfflineBenchmarkClassQuestChainScope(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what quests are in the Barrens?", "what comes after Murloc Gills in the Toxic Soil quest line?", "what does a shaman learn at level 10?", "where is the hunter trainer in Orgrimmar?",
		"which quest rewards a Thunderfury?", "how do I find the quest giver in Goldshire?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f) + "(no class quest line)", "the question asks for a zone, a named line, a spell or a trainer", kaClassSrc, resultIf(!strings.Contains(f, "Class quest chains:"), "the class quest line was added to another question")})
	}
	finish(t, "advice-class-quest-chain-scope", "Offline class quests: other quest questions get no class quest line", out)
}

// ---- the mailbox ----

// TestOfflineBenchmarkMailboxFacts: how long mail stays, whether gold or a bound item can be sent, how many items attach and what a returned mail does is not in the dataset: the answer is framed with the mail line.
// Before the fix a question that said "stay", "send gold" or "soulbound" got none, or only the capital services line.
func TestOfflineBenchmarkMailboxFacts(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"how long does mail stay in my mailbox?", "can I mail a soulbound item to my alt?", "can I send gold through the mail?", "how many days until my mail expires?",
		"how many items can I attach to one mail?", "what is the postage on a mail with gold?", "how long is a cash on delivery mail kept?", "is mail I did not collect returned to the sender?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f), "no mail rule in the data: say so, name no figure; the item's binding decides whether it can be mailed", kaMailSrc,
			resultIf(vtSays(f, "Mail:"), "no line saying the data holds no mail rule, so a figure may be made up from memory")})
	}
	finish(t, "advice-mailbox-facts", "Offline mailbox: the data holds no mail rules", out)
}

// TestOfflineBenchmarkMailboxScope: a question about mail armor, the mailbox's place or an item to mail with a binding in the facts gets no mail rules line (it is no question about the post).
func TestOfflineBenchmarkMailboxScope(t *testing.T) {
	var out []benchRow
	for _, q := range []string{
		"what mail armor should my shaman buy, how much does it cost?", "can a priest wear mail?", "what is the best leather or mail for a hunter at 40?", "when does a hunter get mail armor?",
		"which mail helm drops in Scholomance, what is its price?", "where is the mailbox in Orgrimmar?",
	} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{"\"" + q + "\": the facts", firstLine(f) + "(no mail rules line)", "the question is about the armor type or the mailbox's place", kaMailSrc, resultIf(!strings.Contains(f, "Mail: the game data holds no mail rules"), "the mail rules line was added to an armor question")})
	}
	finish(t, "advice-mailbox-scope", "Offline mailbox: armor and place questions get no mail rules line", out)
}
