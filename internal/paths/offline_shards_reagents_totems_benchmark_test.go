package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/levelup"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline soul shard, class reagent, totem and ritual benchmark: what the advisor does with a warlock's shards and stones, a druid's seeds, a rogue's powder, a priest's feather
// and a shaman's totems in the bags, which levels it says the Create, totem and ritual spells come at, and what it says when asked what a spell uses up, beside the dataset's rows and
// the game's own rules, with no network. It reads testdata/consumables_snapshot (Soulstones, Healthstones, the seeds, and the recipes that also use the seeds),
// testdata/tradeskills_snapshot (Demonic Figurine, Light Feather, Blinding Powder) and testdata/usability_snapshot (class_spells and the totem relics). The dataset holds
// no Soul Shard row, no spell reagent table (recipe_reagents lists profession recipes), no shard rule and no totem or ritual effect, so the cases of those check that the answer is
// framed with the line that says so (ask.LimitFacts) and that nothing is stated from memory. A difference the dataset's own rows cause is marked "(data)". make offline-benchmark
// runs it; WOWC_BENCH_REPORT receives the tables.

const (
	srShardSrc  = "dataset items (no Soul Shard row in any snapshot), class_spells (Drain Soul, Create Soulstone, Create Healthstone: name, rank, level; no reagent column); game: a warlock's Create and summon spells use Soul Shards up (Warcraft Wiki, Soul Shard)"
	srStoneSrc  = "dataset items (class_id 0 subclass_id 8; Soulstone bonding 1, Healthstone bonding 0; no sell or buy price); game: a warlock's Create spells make them (Warcraft Wiki, Soulstone, Healthstone)"
	srSeedSrc   = "dataset items (class_id 15 subclass_id 1 reagent, bonding 0), recipe_reagents (Minor Wizard Oil, Minor Mana Oil, Lesser Wizard Oil use the seeds); game: Maple and Stranglethorn Seeds are the druid's Rebirth reagents (Warcraft Wiki, Rebirth)"
	srReagSrc   = "dataset items (class_id 15 subclass_id 1 reagent; Demonic Figurine, Light Feather bonding 0, Blinding Powder bonding 1); game: Ritual of Doom, Levitate and Blind use them up (Warcraft Wiki, class reagents)"
	srSpellSrc  = "dataset class_spells (name, rank, level); Warcraft Wiki class spell pages"
	srRelicSrc  = "dataset items (class_id 4 subclass_id 9 totem relic, bonding 1, required_level); game: only shamans wield a totem in the relic slot (Warcraft Wiki, Totem)"
	srNoneSrc   = "the dataset has no such row, column or table"
	srLimitSrc  = "dataset recipe_reagents (profession recipes only; no row for a class spell's spell id), class_spells (no reagent, duration, range or cost column); no totem or ritual table"
	srDataNoteS = "dataset items (a deprecated or test placeholder row carries no flag that says so)"
)

// srSyntheticShard is the game's Soul Shard item id: the snapshot holds no row for it, so the bags carry the id and the name the capture reports.
const srSyntheticShard = 6265

// srChar is a character of the class and level with the named stacks in the bags: each name is read from the store for its item id; "Soul Shard" is the capture's own (no row).
func srChar(t *testing.T, ds store.DataStore, name, class string, stacks ...string) character.Character {
	t.Helper()
	c := usabChar(class, 60, "Horde")
	c.Name, c.BagsCaptured, c.BankCaptured, c.Equipment = name, true, true, map[string]character.EquippedItem{}
	for _, s := range stacks {
		id := srSyntheticShard
		if s != "Soul Shard" {
			id = int(num(rowByName(t, ds, "items", s)["id"]))
		}
		c.Bags = append(c.Bags, character.ItemStack{ItemID: id, Count: 5, Name: s})
	}
	return c
}

func srProf(c character.Character, p string) character.Character {
	c.Professions = append(c.Professions, character.Skill{Name: p})
	return c
}

// srSingle is the group and reason the review of one character gives the item.
func srSingle(c character.Character, ds store.DataStore, item string) (inventory.Group, string) {
	var g inventory.Group
	var why string
	for gr, es := range inventory.Review(c, ds).Groups {
		for _, e := range es {
			if e.Name == item {
				g, why = gr, e.Reason
			}
		}
	}
	return g, why
}

// srAll is the line of the review across the characters for the item in the named holder's bags.
func srAll(chars []character.Character, ds store.DataStore, who, item string) inventory.Line {
	l, _ := kaLine(inventory.ReviewAll(chars, ds), who, item)
	return l
}

func srLine(l inventory.Line) string { return fmt.Sprintf("%s %q: %s", l.Action, l.To, l.Reason) }

// ---- soul shards, Soulstones and Healthstones in the bags ----

// TestOfflineBenchmarkShardStoneReview: a warlock keeps the shards and the stones a Create spell made; another class is not told to sell or toss what the dataset cannot place. Before the fix
// the review left a warlock's Soul Shard "not in the dataset" and its Soulstone and Healthstone "no rule places it".
func TestOfflineBenchmarkShardStoneReview(t *testing.T) {
	ds := consStore()
	var out []benchRow
	lock := srChar(t, ds, "Lockb", "WARLOCK", "Soul Shard", "Soulstone", "Major Healthstone", "Lesser Soulstone")
	for _, it := range []string{"Soul Shard", "Soulstone", "Major Healthstone", "Lesser Soulstone"} {
		g, why := srSingle(lock, ds, it)
		src, ref := srStoneSrc, "kept: a warlock's own shard or stone"
		if it == "Soul Shard" {
			src = srShardSrc
		}
		out = append(out, benchRow{it + " in a warlock's bags: the bag review", fmt.Sprintf("%s: %s", g, why), ref, src, resultIf(g == inventory.Keep && strings.Contains(why, "warlock"), "a warlock's shard or stone was not kept")})
	}
	mage := srChar(t, ds, "Magea", "MAGE", "Soul Shard", "Soulstone", "Healthstone")
	alt := srChar(t, ds, "Lockb", "WARLOCK")
	for _, it := range []string{"Soul Shard", "Soulstone", "Healthstone"} {
		l := srAll([]character.Character{mage, alt}, ds, "Magea", it)
		src := srStoneSrc
		if it == "Soul Shard" {
			src = srShardSrc
		}
		out = append(out, benchRow{it + " in a mage's bags, a warlock alt: the review across both", srLine(l), "not sold, tossed or mailed on a guess: the dataset has no price and no rule for it", src,
			resultIf(l.Action == inventory.Unsure && l.Price == nil, "a stone or shard was sold, tossed or mailed")})
	}
	out = append(out, benchRow{"Soul Shard in a warlock's bags: the review across a warlock and a mage", srLine(srAll([]character.Character{lock, mage}, ds, "Lockb", "Soul Shard")), "keep: no bag slot is freed", srShardSrc,
		resultIf(srAll([]character.Character{lock, mage}, ds, "Lockb", "Soul Shard").Action == inventory.Keep_, "a warlock's Soul Shard was not kept")})
	finish(t, "advice-shard-stone-review", "Offline soul shards: a warlock keeps its shards and stones", out)
}

// TestOfflineBenchmarkShardSpellLevels: the level the Create, Drain and Soul spells are trained at, beside class_spells.
func TestOfflineBenchmarkShardSpellLevels(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		name string
		rank int64
	}{{"Drain Soul", 1}, {"Drain Soul", 4}, {"Create Healthstone", 1}, {"Create Healthstone", 5}, {"Create Soulstone", 1}, {"Create Soulstone", 5}, {"Create Firestone", 1}, {"Create Spellstone", 1}, {"Soul Fire", 1}} {
		r := utilTrained(t, "WARLOCK", "Horde", tc.name, tc.rank)
		r.source = srSpellSrc
		out = append(out, r)
	}
	finish(t, "advice-shard-spell-levels", "Offline soul shards: the levels of the Create and Drain spells", out)
}

// TestOfflineBenchmarkShardTooltips: what a stone's binding lets a player do with it, from its row; the Soul Shard has no row, so the tooltip says so and states nothing.
func TestOfflineBenchmarkShardTooltips(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, tc := range []struct{ item, want, ref string }{
		{"Soulstone", "cannot be traded", "bonding 1: soulbound, cannot be traded or mailed"},
		{"Lesser Soulstone", "cannot be traded", "bonding 1: soulbound, cannot be traded or mailed"},
		{"Major Healthstone", "can be traded", "bonding 0: binds to no one, can be traded"},
	} {
		f := ask.ItemTooltipFacts(ds, "can I trade ["+tc.item+"] to my friend?")
		out = append(out, benchRow{tc.item + ": can it be traded", strings.TrimPrefix(strings.ReplaceAll(f, "\n", " "), "Item tooltips (from the item rows; answer from these, not from memory): "), tc.ref, srStoneSrc,
			resultIf(strings.Contains(f, tc.want) && !strings.Contains(f, "no row"), "the stone's binding was not read from its row")})
	}
	f := ask.ItemTooltipFacts(ds, "can I trade [Soul Shard] to my friend?")
	out = append(out, benchRow{"Soul Shard: can it be traded", strings.ReplaceAll(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n"), "\n", " "), "no Soul Shard row: the binding is not known",
		srShardSrc, resultIf(strings.Contains(f, "no row for this item") && !strings.Contains(f, "soulbound"), "a binding was stated for an item with no row")})
	finish(t, "advice-shard-tooltips", "Offline soul shards: a stone's binding, a shard with no row", out)
}

// ---- class reagents ----

// TestOfflineBenchmarkReagentReview: the seeds a druid casts Rebirth with are kept by the druid though Enchanting recipes use them too; the review before the fix sold them on the Auction House for a druid
// who had no enchanter alt.
func TestOfflineBenchmarkReagentReview(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, seed := range []string{"Maple Seed", "Stranglethorn Seed"} {
		druid := srChar(t, ds, "Druida", "DRUID", seed)
		g, why := srSingle(druid, ds, seed)
		out = append(out, benchRow{seed + " in a druid's bags: the bag review", fmt.Sprintf("%s: %s", g, why), "kept: the druid's Rebirth reagent", srSeedSrc, resultIf(g == inventory.Keep && strings.Contains(why, "druid"), "a druid's seed was not kept")})
		warrior := srChar(t, ds, "Warb", "WARRIOR")
		l := srAll([]character.Character{druid, warrior}, ds, "Druida", seed)
		out = append(out, benchRow{seed + " in a druid's bags, a warrior alt: the review across both", srLine(l), "keep: not sold on the Auction House for a profession no character has", srSeedSrc, resultIf(l.Action == inventory.Keep_, "a druid's seed was sold or mailed")})
	}
	ench := srProf(srChar(t, ds, "Ench", "MAGE"), "Enchanting")
	druid := srChar(t, ds, "Druida", "DRUID", "Maple Seed")
	l := srAll([]character.Character{druid, ench}, ds, "Druida", "Maple Seed")
	out = append(out, benchRow{"Maple Seed in a druid's bags, an enchanter alt: the review across both", srLine(l), "keep: Rebirth uses it, though Minor Wizard Oil and Minor Mana Oil do too", srSeedSrc, resultIf(l.Action == inventory.Keep_, "a druid's seed was mailed to the enchanter")})
	mage := srChar(t, ds, "Magea", "MAGE", "Maple Seed")
	l = srAll([]character.Character{mage, srChar(t, ds, "Druida", "DRUID")}, ds, "Magea", "Maple Seed")
	out = append(out, benchRow{"Maple Seed in a mage's bags, a druid alt: the review across both", srLine(l), "mail to the druid (bonding 0: it can be mailed)", srSeedSrc, resultIf(l.Action == inventory.Mail && l.To == "Druida", "not mailed to the druid")})
	l = srAll([]character.Character{mage, srChar(t, ds, "Warb", "WARRIOR")}, ds, "Magea", "Maple Seed")
	out = append(out, benchRow{"Maple Seed in a mage's bags, no druid or enchanter: the review across both", srLine(l), "sell: no character's spells or professions use it", srSeedSrc, resultIf(l.Action == inventory.Sell, "a seed nobody uses was kept or mailed")})
	l = srAll([]character.Character{mage, ench}, ds, "Magea", "Maple Seed")
	out = append(out, benchRow{"Maple Seed in a mage's bags, an enchanter alt: the review across both", srLine(l), "mail to the enchanter: Minor Wizard Oil uses it", srSeedSrc, resultIf(l.Action == inventory.Mail && l.To == "Ench", "not mailed to the enchanter")})

	ts := tsStore()
	for _, tc := range []struct{ item, class, alt string }{{"Demonic Figurine", "WARLOCK", "MAGE"}, {"Light Feather", "PRIEST", "MAGE"}, {"Blinding Powder", "ROGUE", "MAGE"}} {
		owner := srChar(t, ts, "Owner", tc.class, tc.item)
		g, why := srSingle(owner, ts, tc.item)
		out = append(out, benchRow{tc.item + " in a " + strings.ToLower(tc.class) + "'s bags: the bag review", fmt.Sprintf("%s: %s", g, why), "kept: a " + strings.ToLower(tc.class) + "'s reagent", srReagSrc, resultIf(g == inventory.Keep, "a class reagent was not kept")})
		l := srAll([]character.Character{srChar(t, ts, "Other", tc.alt, tc.item), srChar(t, ts, "Owner", tc.class)}, ts, "Other", tc.item)
		row := rowByName(t, ts, "items", tc.item)
		want := inventory.Mail
		ref := "mail to the " + strings.ToLower(tc.class) + " alt (bonding 0)"
		if num(row["bonding"]) == 1 {
			want, ref = inventory.Sell, "sell: bonding 1, bound to its holder, cannot be mailed to the "+strings.ToLower(tc.class)+" alt"
		}
		out = append(out, benchRow{tc.item + " in a mage's bags, a " + strings.ToLower(tc.class) + " alt: the review across both", srLine(l), ref, srReagSrc, resultIf(l.Action == want, "the reagent went to the wrong place")})
	}
	finish(t, "advice-reagent-review", "Offline class reagents: kept by the class, mailed to its alt", out)
}

// TestOfflineBenchmarkReagentTooltips: a reagent's binding from its row; the reagents the snapshot holds no row for are said to have none.
func TestOfflineBenchmarkReagentTooltips(t *testing.T) {
	ds := consStore()
	var out []benchRow
	f := ask.ItemTooltipFacts(ds, "can I sell [Maple Seed] to a friend?")
	out = append(out, benchRow{"Maple Seed: can it be traded", strings.ReplaceAll(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n"), "\n", " "), "bonding 0: binds to no one, can be traded", srSeedSrc,
		resultIf(strings.Contains(f, "can be traded") && strings.Contains(f, "vendor for"), "the seed's binding or price was not read from its row")})
	for _, it := range []string{"Symbol of Kings", "Arcane Powder", "Ashwood Seed", "Wild Thornroot"} {
		f := ask.ItemTooltipFacts(ds, "can I trade ["+it+"] to my friend?")
		out = append(out, benchRow{it + ": can it be traded", strings.ReplaceAll(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n"), "\n", " "), "no row in the snapshot: the binding is not known", srNoneSrc,
			resultIf(strings.Contains(f, "no row for this item") && !strings.Contains(f, "soulbound"), "a binding was stated for an item with no row")})
	}
	finish(t, "advice-reagent-tooltips", "Offline class reagents: a reagent's binding, a reagent with no row", out)
}

// ---- totems and rituals ----

// TestOfflineBenchmarkTotemRitualLevels: the levels the shaman's utility totems and the warlock's rituals are trained at, beside class_spells.
func TestOfflineBenchmarkTotemRitualLevels(t *testing.T) {
	var out []benchRow
	for _, tc := range []struct {
		class, name string
		rank        int64
	}{
		{"SHAMAN", "Tremor Totem", 0}, {"SHAMAN", "Sentry Totem", 0}, {"SHAMAN", "Poison Cleansing Totem", 0}, {"SHAMAN", "Disease Cleansing Totem", 0}, {"SHAMAN", "Fire Resistance Totem", 1},
		{"SHAMAN", "Grace of Air Totem", 1}, {"SHAMAN", "Totemic Recall", 0}, {"SHAMAN", "Reincarnation", 0},
		{"SHAMAN", "Windwall Totem", 1}, {"WARLOCK", "Ritual of Doom", 0}, {"WARLOCK", "Inferno", 0},
	} {
		r := utilTrained(t, tc.class, "Horde", tc.name, tc.rank)
		r.source = srSpellSrc
		out = append(out, r)
	}
	finish(t, "advice-totem-ritual-levels", "Offline totems and rituals: the levels of the utility totems and the rituals", out)
}

// TestOfflineBenchmarkTotemRelicReview: a totem is a shaman's relic: another class is not told to keep it, and a shaman at its level keeps it.
func TestOfflineBenchmarkTotemRelicReview(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	row := usabItem(t, "Totem of Flowing Water")
	war := srChar(t, ds, "Warb", "WARRIOR", "Totem of Flowing Water")
	sham := srChar(t, ds, "Shama", "SHAMAN")
	l := srAll([]character.Character{war, sham}, ds, "Warb", "Totem of Flowing Water")
	out = append(out, benchRow{"Totem of Flowing Water in a warrior's bags, a shaman alt: the review across both", srLine(l),
		fmt.Sprintf("bonding %d: bound on pickup, so it cannot be mailed to the shaman; a warrior cannot use it", num(row["bonding"])), srRelicSrc,
		resultIf(l.Action == inventory.Sell && strings.Contains(l.Reason, "cannot be mailed") || l.Action == inventory.Sell && num(row["bonding"]) != 1, "a bound totem was mailed or kept for a warrior")})
	s60 := srChar(t, ds, "Shama", "SHAMAN", "Totem of Flowing Water")
	g, why := srSingle(s60, ds, "Totem of Flowing Water")
	out = append(out, benchRow{"Totem of Flowing Water in a level 60 shaman's bags: the bag review", fmt.Sprintf("%s: %s", g, why), "equip: the relic slot is empty and a shaman holds a totem there (required level 60)", srRelicSrc,
		resultIf(g == inventory.Equip, "a shaman's totem was left for the bank")})
	pal := srChar(t, ds, "Pala", "PALADIN", "Totem of Flowing Water")
	g, why = srSingle(pal, ds, "Totem of Flowing Water")
	out = append(out, benchRow{"Totem of Flowing Water in a paladin's bags: the bag review", fmt.Sprintf("%s: %s", g, why), "not equipped or kept as an upgrade: a paladin holds a libram", srRelicSrc,
		resultIf(g != inventory.Equip && g != inventory.Keep, "a paladin was told to equip a totem")})
	low := srChar(t, ds, "Shama", "SHAMAN", "Totem of Flowing Water")
	lv := 40
	low.Level = &lv
	g, why = srSingle(low, ds, "Totem of Flowing Water")
	out = append(out, benchRow{"Totem of Flowing Water in a level 40 shaman's bags: the bag review", fmt.Sprintf("%s: %s", g, why), fmt.Sprintf("keep, from level %d (required_level)", num(row["required_level"])), srRelicSrc,
		resultIf(g == inventory.Keep && strings.Contains(why, fmt.Sprintf("level %d", num(row["required_level"]))), "a totem the shaman will use was banked")})
	worn := srChar(t, ds, "Shama", "SHAMAN", "Totem of Flowing Water")
	wid := int(num(usabItem(t, "Totem of Life")["id"]))
	worn.Equipment = map[string]character.EquippedItem{"ranged": {ItemID: &wid}}
	g, why = srSingle(worn, ds, "Totem of Flowing Water")
	out = append(out, benchRow{"Totem of Flowing Water beside a worn Totem of Life: the bag review", fmt.Sprintf("%s: %s", g, why), "keep: the dataset holds no totem effect, so one totem is not ranked above another", srNoneSrc,
		resultIf(g == inventory.Keep, "a totem was banked or called an upgrade on no effect")})
	finish(t, "advice-totem-relic-review", "Offline totems: a totem relic is a shaman's", out)
}

// ---- the dataset has no answer ----

// TestOfflineBenchmarkReagentNoAnswer: what the dataset cannot answer about a spell's reagent, a shard's rules, a totem's effect or a ritual: the question is framed with the line that says so, and
// the level facts for a warlock name no shard. Before the fix none of these questions carried that line.
func TestOfflineBenchmarkReagentNoAnswer(t *testing.T) {
	ds := usabStore()
	var out []benchRow
	for _, tc := range []struct{ q, ref string }{
		{"how many soul shards does Summon Voidwalker use?", "no spell reagent table; class_spells has no reagent column"},
		{"how do I get more soul shards?", "no Soul Shard row and no shard rule"},
		{"what reagent does Rebirth need?", "recipe_reagents holds no row for a class spell"},
		{"how many Wild Thornroot does Gift of the Wild use?", "no spell reagent table"},
		{"what does Windfury Totem do and how long does it last?", "class_spells has no effect or duration column"},
		{"what is the range of Mana Spring Totem?", "class_spells has no range column"},
		{"how many people does Ritual of Summoning need?", "no ritual row: the number of players is not held"},
		{"does Ritual of Summoning cost a soul shard?", "no spell reagent table"},
		{"what does the Ritual of Doom require?", "no ritual row"},
		{"what is the cooldown of Grounding Totem?", "class_spells has no cooldown column"},
	} {
		f := ask.LimitFacts(tc.q)
		out = append(out, benchRow{tc.q, firstLine(f), tc.ref, srLimitSrc, resultIf(strings.Contains(f, "Class reagents, soul shards, totems and rituals:") && strings.Contains(f, "Say so"), "no line that says the data holds none")})
	}
	for _, q := range []string{"which totem should my shaman equip?", "where do I buy flash powder?", "what level do I learn Searing Totem?"} {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{q, firstLine(f), "answerable: a relic's class and level, a vendor row, a spell's level; no limit line", srSpellSrc, resultIf(!strings.Contains(f, "Class reagents, soul shards, totems and rituals:"), "a limit line on an answerable question")})
	}
	// the spell id of a class spell has no reagent row
	for _, tc := range []struct {
		class, name string
		rank        int64
	}{{"DRUID", "Rebirth", 1}, {"WARLOCK", "Summon Voidwalker", 0}, {"WARLOCK", "Create Healthstone", 1}, {"PRIEST", "Levitate", 0}} {
		r := utilRow(t, tc.class, tc.name, tc.rank)
		regs, _ := ds.Filter("recipe_reagents", store.Range{Column: "spell_id", Min: num(r["spell_id"]), Max: num(r["spell_id"])})
		out = append(out, benchRow{tc.name + ": the reagent rows of spell " + fmt.Sprint(num(r["spell_id"])), fmt.Sprintf("%d reagent rows", len(regs)), "none: recipe_reagents holds profession recipes only", srLimitSrc, resultIf(len(regs) == 0, "a reagent row for a class spell")})
	}
	lock := usabChar("WARLOCK", 30, "Horde")
	facts, _, _ := levelup.Facts(lock, ds, nil, 10, 30)
	lower := strings.ToLower(facts)
	out = append(out, benchRow{"warlock levels 10 to 30: the level facts", firstLine(facts), "class_spells: the spells and levels only; no reagent or shard count", srLimitSrc,
		resultIf(!strings.Contains(lower, "shard") && !strings.Contains(lower, "reagent"), "a shard or reagent was named")})
	finish(t, "advice-reagent-no-answer", "Offline reagents, totems and rituals: no answer in the dataset", out)
}

// TestOfflineBenchmarkReagentDatasetNotes: what the dataset files that the advisor passes on: the placeholder totems.
func TestOfflineBenchmarkReagentDatasetNotes(t *testing.T) {
	ds := consStore()
	var out []benchRow
	for _, it := range []string{"Deprecated Blood Totem"} {
		row := rowByName(t, ds, "items", it)
		f := ask.ItemTooltipFacts(ds, "what are the levels of ["+it+"]?")
		out = append(out, benchRow{it + ": its tooltip", strings.ReplaceAll(strings.TrimPrefix(f, "Item tooltips (from the item rows; answer from these, not from memory):\n"), "\n", " "),
			fmt.Sprintf("a placeholder item (\"Deprecated\"): item level %d, required level %d, bonding %d; the row has no flag that marks it unused", num(row["item_level"]), num(row["required_level"]), num(row["bonding"])), srDataNoteS,
			"differ (data): the dataset files a deprecated placeholder like any item, so the advisor gives it a tooltip"})
	}
	ts := tsStore()
	row := rowByName(t, ts, "items", "Test Totem")
	out = append(out, benchRow{"Test Totem: filed as", fmt.Sprintf("class %d, subclass %d, bonding %d, required level %d", num(row["class_id"]), num(row["subclass_id"]), num(row["bonding"]), num(row["required_level"])),
		"a test item: not a weapon any player holds", srDataNoteS, "differ (data): the dataset files a test row as a one-handed weapon; no player item"})
	finish(t, "advice-reagent-dataset-notes", "Offline reagents and totems: placeholder rows the dataset holds", out)
}
