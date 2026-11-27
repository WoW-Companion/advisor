package paths_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/inventory"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/store"
)

// The offline vanity pet, title, achievement and profession tool benchmark: what the advisor says when a player asks about a companion pet as a
// collection, a tabard or toy, an achievement or a title, or about what a profession tool gives, beside the dataset's rows and the game's own rules, with no
// network. It reads testdata/companions_snapshot (the companion pet items), testdata/tradeskills_snapshot (the tool items and profession routes). The dataset
// holds a pet only as an item row and has no achievement, title, pet ability, collection limit or tool bonus, so the cases of those check that the answer
// is framed with the line that says so (ask.LimitFacts) and that nothing is stated from memory. A difference the dataset's own rows cause is marked "(data)".
// make offline-benchmark runs it; WOWC_BENCH_REPORT receives the tables.

const (
	vtPetSrc   = "dataset items (class 15, subclass 2; bonding, buy_price, sell_price), loot_sources, vendor_sources; no pet species, ability or collection table"
	vtLimitSrc = "dataset items, quests (a quest's title is its name); no achievement, achievement point, title, pet ability, collection limit or tool bonus column or table; game: Classic has no achievements"
	vtToolSrc  = "dataset items (class 2 subclass 14, 16 and 20: name, stats_status none, buy_price), vendor_sources; advisor tool table (pick, knife, hammer, spanner, rod, pole)"
)

// vtSays is true when a fact is the named limit line and tells the answer to say the data holds nothing.
func vtSays(f, head string) bool {
	return strings.Contains(f, head) && strings.Contains(f, "Say so") && strings.Contains(f, "from memory")
}

// vtLimitBatch checks a list of questions: each must carry the named limit line, or none of it when want is "".
func vtLimitBatch(t *testing.T, area, title, src, head string, qs []string, ref string) {
	var out []benchRow
	for _, q := range qs {
		f := ask.LimitFacts(q)
		got := "no limit line: the answer is free to state a figure"
		if f != "" {
			got = strings.SplitN(firstLine(f), ":", 2)[0] + ": the data has none, say so"
		}
		out = append(out, benchRow{q, got, ref, src, resultIf(vtSays(f, head), "no line saying the data holds nothing, so one may be made up from memory")})
	}
	finish(t, area, title, out)
}

// vtScopeBatch checks that questions the data answers (or another fact covers) do not get the named limit line.
func vtScopeBatch(t *testing.T, area, title, src, head string, qs []string, ref string) {
	var out []benchRow
	for _, q := range qs {
		f := ask.LimitFacts(q)
		out = append(out, benchRow{q, strings.SplitN(firstLine(f), ":", 2)[0] + " (no " + strings.SplitN(head, ":", 2)[0] + " line)", ref, src, resultIf(!strings.Contains(f, head), "the limit line was added to a question the data answers")})
	}
	finish(t, area, title, out)
}

// ---- companion pets and vanity items ----

// TestOfflineBenchmarkVanityNoAnswer: a question about the pet collection, a pet's abilities, pet battles, a tabard's effect or a toy: the dataset holds the item row and
// no more, so the answer is framed with the line that says it holds no species, ability, limit or effect.
func TestOfflineBenchmarkVanityNoAnswer(t *testing.T) {
	vtLimitBatch(t, "advice-vanity-limits", "Offline companion pets and vanity items: the data holds the item, not the pet", vtPetSrc, "Companion pets and vanity items:", []string{
		"how many companion pets can I keep in my collection?",
		"what abilities does my vanity pet have?",
		"can I have two non-combat pets out at once?",
		"what is in the pet journal for a level 20 character?",
		"does the Argent Dawn tabard give reputation in dungeons?",
		"is there a toy I can buy with Darkmoon cards?",
		"how do I level my critter pets in pet battles?",
		"which vanity items are worth collecting?",
	}, "the item row only: no species, ability, collection limit, battle or tabard and toy effect")
}

// TestOfflineBenchmarkVanityNotOverMatched: a hunter's or warlock's pet, and a question about where a pet comes from, get no vanity line: the farm rows answer the second and the
// class pets are not companions.
func TestOfflineBenchmarkVanityNotOverMatched(t *testing.T) {
	vtScopeBatch(t, "advice-vanity-scope", "Offline companion pets: questions the data answers get no such line", vtPetSrc, "Companion pets and vanity items:", []string{
		"where do I buy the Cat Carrier (Bombay)?",
		"which creature drops a Hawk Owl?",
		"what abilities does my hunter's pet learn at level 20?",
		"how do I get my warlock's imp to cast Firebolt?",
		"how do I tame a pet at level 10?",
	}, "the item's own loot and vendor rows, or a class's own pet rules")
}

// TestOfflineBenchmarkPetItemRows: the pet items of the dataset, asked about as a trade or a sale: the advisor states each pet's binding and vendor price from the item row,
// says a bind-on-use pet can be traded until used, and a pet in the bags is never sold or tossed.
func TestOfflineBenchmarkPetItemRows(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, id := range []int64{4401, 284664, 8485, 13342, 18963, 19450, 21277} {
		it := itemsRowOf(t, id)
		name := text(it["name"])
		tip := ask.ItemTooltipFacts(ds, "can I trade or sell ["+name+"]?")
		bind := num(it["bonding"])
		ok := strings.Contains(tip, name)
		switch bind {
		case 0:
			ok = ok && strings.Contains(tip, "binds to no one") && strings.Contains(tip, "can be traded, mailed and put on the Auction House")
		case 1:
			ok = ok && strings.Contains(tip, "Bind on Pickup") && strings.Contains(tip, "cannot be traded")
		case 3:
			ok = ok && strings.Contains(tip, "Bind on Use") && strings.Contains(tip, "until it is used it can be traded")
		}
		if sp := num(it["sell_price"]); sp > 0 {
			ok = ok && strings.Contains(tip, "sells to a vendor for") && strings.Contains(tip, fmt.Sprintf("(%d copper)", sp))
		} else {
			ok = ok && !strings.Contains(tip, "sells to a vendor for")
		}
		c := compChar("Alliance", "MAGE", 40, "", 0, 0)
		c.Name, c.BagsCaptured, c.BankCaptured = "Solo", true, true
		c.Bags = []character.ItemStack{{ItemID: int(id), Count: 1, Name: name}}
		var action inventory.Action
		for _, l := range inventory.ReviewAll([]character.Character{c}, lgItemsOnly{ds}).Lines {
			action = l.Action
		}
		ok = ok && action != inventory.Sell && action != inventory.Toss
		line := strings.ReplaceAll(strings.TrimSpace(strings.TrimPrefix(tip, "Item tooltips (from the item rows; answer from these, not from memory):")), "\n", "; ")
		ref := fmt.Sprintf("bonding %d, sell_price %d copper, class %d subclass %d; never sold or tossed", bind, num(it["sell_price"]), num(it["class_id"]), num(it["subclass_id"]))
		out = append(out, benchRow{name + ": can I trade or sell it", fmt.Sprintf("%s; bag review: %s", line, action), ref, vtPetSrc, resultIf(ok, "the binding or price differs from the item row, or the pet was sold or tossed")})
	}
	finish(t, "advice-pet-rows", "Offline companion pets: binding, price and the bag review of each pet item", out)
}

// TestOfflineBenchmarkKeyRingNoAnswer: the key ring's size is no more in the dataset than a bag's: the answer is framed with the bag and bank size line.
func TestOfflineBenchmarkKeyRingNoAnswer(t *testing.T) {
	var out []benchRow
	for _, q := range []string{"how many slots does the key ring have?", "how many keys does my keyring hold?", "how big is the key ring?", "what is the keyring size?"} {
		f := ask.LimitFacts(q)
		ok := vtSays(f, "Bag and bank size:") && strings.Contains(f, "key ring size")
		out = append(out, benchRow{q, strings.SplitN(firstLine(f), ":", 2)[0] + ": the data has none, say so", "no key ring row or size in the dataset (items hold bags by class 1, no key ring slot count)", vtToolSrc, resultIf(ok, "no bag size line naming the key ring")})
	}
	finish(t, "advice-keyring", "Offline key ring: the data holds no size", out)
}

// ---- titles and achievements ----

// TestOfflineBenchmarkTitlesAchievementsNoAnswer: a question about an achievement, its points or a title and how to earn it: the dataset holds none, so the answer is framed
// with the line that says so and names no achievement, point value or title.
func TestOfflineBenchmarkTitlesAchievementsNoAnswer(t *testing.T) {
	vtLimitBatch(t, "advice-titles-achievements", "Offline titles and achievements: the data holds none", vtLimitSrc, "Achievements and titles:", []string{
		"what achievements can I earn in Westfall?",
		"how many achievement points do I have?",
		"how do I get the Scarab Lord title?",
		"what titles can my character earn?",
		"is there a title for exploring all of Kalimdor?",
		"how do I earn the Salty title?",
		"which achievement rewards a mount?",
		"what are the requirements for the Loremaster title?",
	}, "no achievement, point, criteria or title row in the dataset")
}

// TestOfflineBenchmarkTitlesNotOverMatched: a quest's title, a title in a quest question and an honor rank question get no achievement line: the quest rows answer the first two
// and the PvP rank line the third.
func TestOfflineBenchmarkTitlesNotOverMatched(t *testing.T) {
	vtScopeBatch(t, "advice-titles-scope", "Offline titles and achievements: questions the data answers get no such line", vtLimitSrc, "Achievements and titles:", []string{
		"what is the title of the quest that starts in Goldshire?",
		"which quest should I do next in Westfall?",
		"where is the Deadmines entrance?",
		"what is the best title of my guild ranks to hand out?",
		"how many honor points do I need for rank 10?",
	}, "the quest rows, or the PvP rank line")
}

// TestOfflineBenchmarkNoTitleTables: the dataset has no table or column for an achievement, a title or a pet species.
func TestOfflineBenchmarkNoTitleTables(t *testing.T) {
	ds := compStore()
	var out []benchRow
	for _, tb := range []string{"achievements", "achievement_criteria", "titles", "char_titles", "pet_species", "pet_abilities", "toys"} {
		_, err := ds.Filter(tb, store.Range{Column: "id", Min: int64(0)})
		out = append(out, benchRow{"table " + tb, fmt.Sprint(err), "no such table", vtLimitSrc, resultIf(err != nil, "a table the advisor never reads exists")})
	}
	cols := brColumns(ds, "items", "id")
	bad := brHasColumn(cols, "achiev", "title", "species", "summon", "toy")
	out = append(out, benchRow{"items: an achievement, title, species, summon or toy column", fmt.Sprintf("%d columns: none of them", len(cols)), "no such column", vtLimitSrc, resultIf(bad == "", "a column the advisor never reads holds "+bad)})
	finish(t, "advice-no-title-tables", "Offline titles and pets: the dataset has no such table", out)
}

// ---- profession tools ----

// TestOfflineBenchmarkToolEffectNoAnswer: what a tool gives (a skill bonus, durability, a lure's bonus) or which is best: the dataset holds the tool as an item with no stats, so
// the answer is framed with the line that says it holds no bonus.
func TestOfflineBenchmarkToolEffectNoAnswer(t *testing.T) {
	vtLimitBatch(t, "advice-tool-effects", "Offline profession tools: the data holds the item, not its bonus", vtToolSrc, "Profession tool effects:", []string{
		"what is the best fishing pole?",
		"how much skill does the Big Iron Fishing Pole give?",
		"what fishing lures are there and how much skill do they give?",
		"does a mining pick lose durability?",
		"is the Arcanite Blacksmith Hammer better than the Mithril one?",
		"how much faster is a skinning knife than my dagger?",
	}, "an item row with no stats and no effect")
}

// TestOfflineBenchmarkToolNotOverMatched: which tool a profession needs, where to buy it and killing a mob with a lure get no effect line: the tool table and the vendor rows answer.
func TestOfflineBenchmarkToolNotOverMatched(t *testing.T) {
	vtScopeBatch(t, "advice-tool-scope", "Offline profession tools: questions the data answers get no such line", vtToolSrc, "Profession tool effects:", []string{
		"do I need a mining pick to mine copper?",
		"where do I buy a skinning knife?",
		"which rod do I need to enchant a bracer?",
		"how do I lure the guards away from the chest?",
		"what does a blacksmith hammer cost?",
	}, "the tool table and the vendor rows")
}

// TestOfflineBenchmarkToolNoProp: a creature's held prop the dataset lists as an item ("Monster - Mining Pick") is no tool: it is kept for no profession and does not stand in
// for the real tool; every real tool the dataset names is still one.
func TestOfflineBenchmarkToolNoProp(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, tc := range []struct {
		name, want string
	}{
		{"Monster - Mining Pick", ""}, {"Monster - Item, Fishing Pole", ""}, {"Monster - Skinning Knife", ""}, {"Monster - Blacksmith Hammer", ""},
		{"Mining Pick", "Mining"}, {"Fishing Pole", "Fishing"}, {"Cracked Blacksmith Hammer", "Blacksmithing"}, {"Arcanite Blacksmith Hammer", "Blacksmithing"},
		{"Runed Arcanite Rod", "Enchanting"}, {"Arclight Spanner", "Engineering"}, {"Skinning Knife", "Skinning"}, {"Arcanite Fishing Pole", "Fishing"},
	} {
		it, _ := ds.Filter("items", store.Range{Column: "name", Min: tc.name, Max: tc.name})
		listed := len(it) > 0
		got := professions.ToolOf(tc.name)
		ref := "no profession"
		if tc.want != "" {
			ref = tc.want + "'s tool"
		}
		ref += fmt.Sprintf("; %q listed in the dataset: %v", tc.name, listed)
		out = append(out, benchRow{tc.name + ": whose tool is it", fmt.Sprintf("%q", got), ref, vtToolSrc, resultIf(got == tc.want, "a creature's prop counted as a tool, or a real tool is not one")})
	}
	// a prop in the bags of a miner does not stand in for the pick
	c := tsChar("Alliance", 20, tsSkill{"Mining", 30, 75})
	c.Bags = []character.ItemStack{{ItemID: 278423, Count: 1, Name: "Monster - Mining Pick"}}
	has, needed := professions.HasTool(c, "Mining")
	out = append(out, benchRow{"a miner carrying Monster - Mining Pick: does the character have the tool", fmt.Sprintf("has %v, needed %v", has, needed), "the prop is no pick: the miner still needs one", vtToolSrc, resultIf(!has && needed, "a creature's prop stood in for the Mining Pick")})
	finish(t, "advice-tool-props", "Offline profession tools: a creature's prop is no tool", out)
}

// TestOfflineBenchmarkNoToolProfessions: a profession with no tool item is never told it needs one; the answer names no tool for it.
func TestOfflineBenchmarkNoToolProfessions(t *testing.T) {
	ds := tsStore()
	var out []benchRow
	for _, prof := range []string{"Alchemy", "Tailoring", "Leatherworking", "Cooking", "First Aid", "Herbalism"} {
		c := tsChar("Alliance", 30, tsSkill{prof, 40, 75})
		ans := professions.RouteFacts(c, ds, nil, prof)
		has, needed := professions.HasTool(c, prof)
		ok := !strings.Contains(ans, "They have no") && !needed && !has && professions.ToolOf(prof) == ""
		got := "no tool line"
		if strings.Contains(ans, "They have no") {
			got = firstMatch(ans, "They have no")
		}
		out = append(out, benchRow{prof + " at skill 40: is a tool asked for", got, "no tool item for " + prof + " in the dataset or the advisor's tool table", vtToolSrc, resultIf(ok, "a tool was asked for a profession that has none")})
	}
	finish(t, "advice-no-tool-professions", "Offline professions with no tool: none is asked for", out)
}
