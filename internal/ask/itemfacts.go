package ask

import (
	"regexp"
	"strings"

	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/learn"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/store"
)

// tooltipWords make a question one about an item's tooltip: its levels, its binding, or whether it can be traded, mailed, auctioned or stacked.
var tooltipWords = regexp.MustCompile(`(?i)\b(trade|trading|tradeable|tradable|mail|mailed|mailing|soulbound|bound|bind|binds|bop|boe|auction|ah|sell|sells|selling|sold|vendor|price|worth|tooltip|item level|ilvl|required level|requires|level|stack|stacks|stacking|unique)\b`)

// ItemTooltipFacts is the tooltip for each item the question links by [name]: item level, required level, binding and what the binding
// lets a player do with it, from the dataset row. "" for a question that is not about those or that links no item. An item the dataset does
// not hold says so and states nothing about it.
func ItemTooltipFacts(ds store.DataStore, text string) string {
	if ds == nil || !tooltipWords.MatchString(text) {
		return ""
	}
	var lines []string
	for _, m := range itemInBrackets.FindAllStringSubmatch(text, maxLootItems) {
		name := strings.TrimSpace(m[1])
		hits, _, err := names.Find(ds, names.Items, name, 1)
		var row store.Row
		if err == nil && len(hits) == 1 && strings.EqualFold(hits[0].Name, name) {
			row, _ = ds.Get("items", hits[0].ID)
		}
		tip, ok := gear.ItemTooltip(row)
		if !ok {
			lines = append(lines, "- "+name+": the dataset has no row for this item, so its levels, binding and whether it can be traded are not known")
			continue
		}
		lines = append(lines, "- "+name+": "+tip.Text())
	}
	if len(lines) == 0 {
		return ""
	}
	return "Item tooltips (from the item rows; answer from these, not from memory):\n" + strings.Join(lines, "\n")
}

// classTrainer is a class's trainer by name: "mage trainer", "priest trainers".
const classTrainer = `(?:warrior|paladin|hunter|rogue|priest|shaman|mage|warlock|druid)s?(?:'s)? trainers?`

// limitTopics are the game limits and tools the game data holds nothing on: a question that touches one gets the line that says so.
var limitTopics = []struct {
	about *regexp.Regexp
	fact  string
}{
	{regexp.MustCompile(`(?i)\b(bag|bags|backpack|bank|bank slots?)\b.*\b(slots?|size|sizes|space|capacity|big|bigger|biggest|large|larger|largest|many|price|cost)\b|\b(slots?|size|capacity|big|bigger|biggest|large|larger|largest|many|price|cost)\b.*\b(bag|bags|backpack|bank)\b`),
		"Bag and bank size: the game data holds no bag slot count, no backpack or bank size and no bank slot price. Say so; give no slot count or price from memory."},
	{regexp.MustCompile(`(?i)\bstack(s|ed|ing)?\b|\bhow many\b.*\b(carry|hold|fit)\b`),
		"Stack size: the game data holds no maximum stack size for any item. Say so; give no stack size from memory (the capture's own counts are what the character holds)."},
	{regexp.MustCompile(`(?i)\bunique(?:-equipped)?\b|\bonly one\b.*\b(carry|equip|wear|hold)\b|\b(carry|equip|wear|hold)\b.*\bonly one\b`),
		"Unique items: the game data holds no unique or unique-equipped flag and no per-character limit on an item. Say so; call no item unique."},
	{regexp.MustCompile(`(?i)\b(mail|mailbox|mails|postage|cod)\b.*\b(attach\w*|limit|expire\w*|days?|cost|fee|postage|cod|how many|max\w*|return\w*)\b|\b(attach\w*|postage|cod)\b.*\bmail`),
		"Mail: the game data holds no mail rules (items per mail, postage, cash on delivery, how long a mail is kept). Say so; give no figure from memory. What the data does hold is an item's binding, which decides whether it can be mailed at all."},
	{regexp.MustCompile(`(?i)\b(auction|auctions|auction house|ah)\b.*\b(deposit|fee|fees|cut|duration|hours?|how long|listing|listings|limit|max\w*|price|prices|worth|cost|much|expire\w*|sell|sells|selling|sold|list|profit\w*|margin|undercut\w*|bid|bids|buyout)\b|\b(deposit|fee|fees|cut|duration|hours?|how long|listing|listings|limit|max\w*|price|prices|worth|cost|much|expire\w*|sell|sells|selling|sold|list|profit\w*|margin|undercut\w*|bid|bids|buyout)\b.*\b(auction|auctions|auction house|ah)\b`),
		"Auction House: the game data holds no Auction House prices, deposit, cut, listing duration or listing limit. Say so; name no price or fee from memory. What the data does hold is an item's binding and quality, which decide whether it can be listed."},
	{goldMaking,
		"Gold making: the game data holds vendor prices only (an item's sell price to a vendor and a vendor's asking price): no Auction House price, no demand or how fast an item sells, and no profit, margin or gold per hour for a craft, a farm, a profession or a route. Say so; give no market price, profit or gold-per-hour figure from memory and do not rank professions or items by gold. What the data does hold is each item's vendor sell price, a recipe's materials and a drop's chance, which give the vendor value of what is made or farmed."},
	{goldFees,
		"Gold and fees: the game data holds no gold cap, no repair cost or rate, no talent respec cost and no guild bank rule. Say so; give no figure from memory."},
	{regexp.MustCompile(`(?i)\btrade (window|slots?)\b|\b(trade|trading)\b.*\b(how many|limit|max\w*|slots?|gold|distance|range)\b`),
		"Trade window: the game data holds no trade window rules (items or gold per trade, range). Say so; give no figure from memory. What the data does hold is an item's binding, which decides whether it can be traded."},
	{regexp.MustCompile(`(?i)\b(battlegrounds?|bgs?|wsg|warsong gulch|arathi basin|alterac valley)\b`),
		"Battlegrounds: the game data holds no battleground row (no entry level, level bracket, team size, queue or objective rule, map or reward). Say so; give no level or bracket from memory. What the data does hold is the level, class and binding of a battleground's consumable or rank item."},
	{regexp.MustCompile(`(?i)\bhonou?r (ranks?|points?|system|vendors?|rewards?|kills?|decay|standing|costs?|tokens?)\b|\b(how much|how many|earn|earns|gain|gains|gives?|gets?)\b.{0,30}\bhonou?r\b|\b(ranks?|points?)\b.{0,20}\bhonou?r\b|\bmarks? of honou?r\b|\bpvp\b.{0,20}\b(ranks?|titles?|vendors?|rewards?|sets?|gear)\b|\b(grand marshal|field marshal|high warlord|lieutenant (commander|general)|knight-(captain|lieutenant|champion)|stone guard|blood guard|legionnaire|centurion)\b`),
		"Honor and PvP ranks: the game data holds no honor points, rank, rank requirement or title, and no honor vendor or its prices. Say so; give no honor cost, rank number or title from memory. What the data does hold is a rank piece's required level, class and binding, and which items only one faction can wear."},
	{regexp.MustCompile(`(?i)\bhearth(?:stone|stones)?\b.{0,40}\b(cooldown|cool down|timer|how often|reuse\w*|how long|minutes?)\b|\b(cooldown|timer)\b.{0,20}\bhearth|\b(boats?|zeppelins?|ferr(?:y|ies)|airships?)\b.{0,50}\b(schedule|how often|how long|every|minutes?|wait\w*|fares?|costs?|price)\b|\b(schedule|how often|how long)\b.{0,30}\b(boats?|zeppelins?|ferr(?:y|ies)|airships?)\b|\bflight (path|master)s?\b.{0,40}\b(costs?|price|fares?|how much|how long|how many minutes)\b|\bhow (long|much)\b.{0,20}\b(is|does)\b.{0,15}\bflight\b`),
		"Travel times: the game data holds no hearthstone cooldown, no boat, zeppelin or tram schedule or ride time, and no flight path fare or flight time. Say so; give no cooldown, schedule, fare or duration from memory. The routes and minute estimates a plan shows come from the advisor's own hand-written travel list, not from the game data."},
	{regexp.MustCompile(`(?i)\briding\b|\b(mounts?|mounted|saddles?|bridles?|steeds?|warhorse|felsteed|dreadsteed)\b`),
		"Riding and mounts: the game data holds no riding skill, riding trainer, riding training cost or level, no mount vendor or drop, and no reputation or race requirement for a mount. What it does hold is a mount item's required level and binding. Say so; give no trainer, training or gold price, riding level or reputation from memory."},
	{regexp.MustCompile(`(?i)\b(class trainers?|` + classTrainer + `)\b.*\b(costs?|prices?|charges?|gold|silver|copper|how much)\b|\b(costs?|prices?|charges?|how much)\b.*\b(class trainers?|` + classTrainer + `)\b|\b(how much|costs?|prices?)\b.*\b(train|trains|training|learn|learning)\b.*\b(spells?|abilit(y|ies))\b`),
		"Class trainer prices: the game data holds no training cost for a class spell or ability and no class trainer's name or position. Say so; give no price from memory. What it does hold is the level a spell is learned at."},
	{regexp.MustCompile(`(?i)\bracials?\b|\b(race|races)\b.{0,25}\b(bonus|bonuses|traits?|abilit(y|ies)|passives?|perks?|advantages?)\b|\b(blood fury|berserking|war stomp|stoneform|shadowmeld|will of the forsaken|cannibalize|escape artist|expansive mind|cultivation|engineering specialization|beast slaying|wisp spirit|gun specialization|bow specialization|throwing specialization)\b|\b(blood fury|berserking|war stomp|stoneform|shadowmeld|will of the forsaken|cannibalize|escape artist|expansive mind|cultivation|engineering specialization|beast slaying|wisp spirit|gun specialization|bow specialization|throwing specialization|sword specialization|mace specialization|axe specialization|frost resistance|nature resistance|shadow resistance|fire resistance|underwater breathing|the human spirit|hardiness|regeneration|perception|diplomacy|quickness|endurance)\b.{0,40}\b(human|orc|dwarf|dwarves|gnome|gnomes|troll|trolls|tauren|undead|forsaken|night elf|night elves)\b|\b(human|orc|dwarf|dwarves|gnome|gnomes|troll|trolls|tauren|undead|forsaken|night elf|night elves)\b.{0,40}\b(blood fury|berserking|war stomp|stoneform|shadowmeld|will of the forsaken|cannibalize|escape artist|expansive mind|cultivation|engineering specialization|beast slaying|wisp spirit|gun specialization|bow specialization|throwing specialization|sword specialization|mace specialization|axe specialization|frost resistance|nature resistance|shadow resistance|fire resistance|underwater breathing|the human spirit|hardiness|regeneration|perception|diplomacy|quickness|endurance)\b`),
		"Racial traits: the game data holds no race table, no racial ability or passive, no weapon skill, profession skill or resistance bonus by race, and no race requirement on a spell or item; the advisor knows only the race and faction the capture reports. Say so; give no racial ability, bonus or percentage from memory."},
	{regexp.MustCompile(`(?i)\b(bankers?|auctioneers?|mail ?box(?:es)?|innkeepers?|stable ?masters?|battlemasters?|barbers?|portals?|teleport(?:s|ation)? trainers?)\b|\b(where|find|nearest|closest|locat\w+|which)\b.{0,40}\b(bank|auction house|ah|inn|mail ?box|stables?)\b|\b(bank|auction house|ah|inn|mail ?box)\b.{0,30}\b(in|at|of)\b.{0,15}\b(orgrimmar|undercity|thunder bluff|stormwind|ironforge|darnassus|silvermoon|exodar|capital|capitals|city|cities)\b`),
		"Capital city services: the game data holds no bank, auction house, mailbox, innkeeper, stable master, battlemaster, barber or portal row, no mage portal or teleport destination, and no position for any of them in a capital. What it holds is the capital's map, its profession trainers and Weapon Masters (by zone, not position). Say so; name no NPC, building or position from memory."},
	{regexp.MustCompile(`(?i)\b(warchief|faction leaders?|city leaders?|(horde|alliance) (leader|leaders|leadership|king|queen|ruler)|leaders? of (the )?(horde|alliance|orgrimmar|undercity|thunder bluff|stormwind|ironforge|darnassus)|who (leads|rules)|high king|archdruid|thrall|cairne|sylvanas|vol'?jin|bolvar|varian|magni|tyrande|anduin|garrosh|saurfang)\b`),
		"Faction leaders: the game data marks no NPC as a faction or city leader (no title, ruler or leadership row) and holds no leader's position, level or reputation reward. Say so; name no leader, title or position from memory."},
	{holidayAsk,
		"Holiday and world events: the game data holds no event table: no event dates or calendar, no event vendor, reward, currency or item source (an event item's row names no event), and no Darkmoon Faire row. Say so; give no date, vendor, price or reward from memory. State when an event runs only from the 'Running today' lines or the seasonal_events tool; the advisor's own table gives the usual windows of its holiday events, which the game moves by a few days some years."},
	{weatherAsk,
		"Weather and time of day: the game data holds no weather, no day and night cycle or in-game clock, no night-only or day-only spawn and no spawn or respawn timer. Say so; give no weather, hour or spawn time from memory. The only time the advisor knows is the date of the capture, which decides which holiday event runs."},
	{regexp.MustCompile(`(?i)\bmacros?\b|/cast\b|/use\b|/castsequence\b`),
		"Macros: the game data holds no macro commands or macro text and the advisor has no macro advice. Say so; do not write a macro and present it as from the game data."},
}

// goldMaking is a question about making gold: a profession, a farm, a craft or an item for profit, or what sells well.
var goldMaking = regexp.MustCompile(`(?i)\b(make|making|makes|earn|earning|earns|farm|farming)\b.{0,25}\b(gold|money)\b|\bgold[- ]?(making|per hour|an hour|farm\w*|guide|tips)\b|\b(profit|profits|profitable|profiting|margins?|money[- ]?making|flip|flipping|undercut\w*)\b|\b(best|most)\b.{0,30}\b(gold|money)\b|\b(gold|money)\b.{0,30}\b(per hour|an hour|each hour|hourly)\b|\bwhat sells (best|well|fast)\b|\bworth (crafting|making|selling|farming|gathering)\b`)

// goldFees is a question about the gold cap, a repair bill, a respec or a guild bank.
var goldFees = regexp.MustCompile(`(?i)\bgold cap\b|\b(max|maximum|most|limit)\b.{0,15}\b(gold|money)\b.{0,15}\b(carry|hold|have)\b|\bhow much (gold|money)\b.{0,20}\b(carry|hold)\b|\brepair\w*\b.{0,30}\b(costs?|price|fees?|bills?|how much)\b|\b(costs?|price|how much)\b.{0,30}\brepair\w*\b|\brespec\w*\b.{0,30}\b(costs?|price|how much|gold)\b|\b(costs?|price|how much)\b.{0,30}\brespec\w*\b|\bguild bank\b`)

// holidayAsk is a question about a holiday or world event: one the advisor lists by name, a Darkmoon Faire, or "holiday" or "world event" words.
var holidayAsk = regexp.MustCompile(`(?i)\b(holidays?|world events?|seasonal events?|darkmoon faire|midsummer|fire festival|hallow'?s end|winter'?s? veil|lunar festival|love is in the air|noblegarden|children'?s week|harvest festival|brewfest)\b`)

// weatherAsk is a question about weather or the time of day in the game world.
var weatherAsk = regexp.MustCompile(`(?i)\b(weather|rain|raining|snowing|foggy|day[ /-]?(and )?night|night ?time|day ?time|sunrise|sunsets?|time of day|(at|during|in) the (night|day)|(only|spawns?|appears?|comes? out) (at|during) (night|day))\b`)

// zoneLevelAsk is a question about the level a zone is meant for; the zone is a "zone" word or a zone the advisor knows by name.
var (
	zoneLevelAsk = regexp.MustCompile(`(?i)\b(what|which)\b.{0,20}\blevels?\b|\blevel (range|bracket)s?\b|\b(recommended|suggested|suitable|appropriate) levels?\b|\blevels? (is|are) (the )?(zone|area)\b`)
	zoneWord     = regexp.MustCompile(`(?i)\b(zones?|areas?)\b`)
)

const zoneLevelFact = "Zone levels: the game data holds no level range, bracket or recommended level for a zone (ui_maps has no level column). Say so; give no zone range from memory. What it does hold is each creature's level and zone (npcs) and each quest's level, which the farm and quest tools report."

// valleyOfHonor is Orgrimmar's district: a question about it is no question about honor.
var valleyOfHonor = regexp.MustCompile(`(?i)\bvalley of honou?r\b`)

// rainOfFire is the warlock spell: a question about it is no question about weather.
var rainOfFire = regexp.MustCompile(`(?i)\brain of fire\b`)

// LimitFacts says what the game data does not hold about bags, banks, stacks, unique items, mail, the Auction House, gold making and gold fees, the trade window,
// holiday events, weather and time of day, zone levels, battlegrounds, honor ranks, travel times, riding and mounts, class trainer prices, racial traits, capital city services, faction leaders and macros, for a question that asks about one. "" for any other question.
func LimitFacts(text string) string {
	text = rainOfFire.ReplaceAllString(valleyOfHonor.ReplaceAllString(text, " "), " ")
	var out []string
	for _, tp := range limitTopics {
		if tp.about.MatchString(text) {
			out = append(out, tp.fact)
		}
	}
	if zoneLevelAsk.MatchString(text) && (zoneWord.MatchString(text) || learn.ZoneIn(text)) {
		out = append(out, zoneLevelFact)
	}
	return strings.Join(out, "\n")
}
