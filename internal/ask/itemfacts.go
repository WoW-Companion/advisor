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

// bagWord is a container a player asks the size of: a bag, the bank, the key ring, a quiver or ammo pouch, or a class bag.
const bagWord = `(?:bag|bags|backpack|bank|bank slots?|key ?rings?|quivers?|ammo ?pouch(?:es)?|ammo bags?|soul (?:bags?|pouch(?:es)?)|herb(?:alism)? bags?|enchanting bags?|mining bags?|gem bags?)`

// limitTopics are the game limits and tools the game data holds nothing on: a question that touches one gets the line that says so.
var limitTopics = []struct {
	about *regexp.Regexp
	fact  string
}{
	{regexp.MustCompile(`(?i)\b` + bagWord + `\b.*\b(slots?|size|sizes|space|capacity|big|bigger|biggest|large|larger|largest|many|price|cost|hold|holds|fit|fits|more)\b|\b(slots?|size|capacity|big|bigger|biggest|large|larger|largest|many|price|cost)\b.*\b` + bagWord + `\b`),
		"Bag and bank size: the game data holds no bag slot count, no backpack, bank or key ring size, no quiver, ammo pouch or special bag (soul, herb, enchanting) size and no bank slot price. Say so; give no slot count or price from memory."},
	{regexp.MustCompile(`(?i)\bstack(s|ed|ing)?\b|\bhow many\b.*\b(carry|hold|fit)\b`),
		"Stack size: the game data holds no maximum stack size for any item. Say so; give no stack size from memory (the capture's own counts are what the character holds)."},
	{regexp.MustCompile(`(?i)\bunique(?:-equipped)?\b|\bonly one\b.*\b(carry|equip|wear|hold)\b|\b(carry|equip|wear|hold)\b.*\bonly one\b`),
		"Unique items: the game data holds no unique or unique-equipped flag and no per-character limit on an item. Say so; call no item unique."},
	{regexp.MustCompile(`(?i)\b(mail|mailbox|mails|postage|cod)\b.*\b(attach\w*|limit|expire\w*|days?|cost|fee|postage|cod|how many|max\w*|return\w*|stay|stays|kept|keep|last|lasts|held|wait|sent|send|sending|arrive|arrives|delay|delivered|delivery|gold|money|soulbound|bound)\b|\b(attach\w*|postage|cod)\b.*\bmail|\b(send|sending|sent|attach\w*)\b.{0,30}\b(through|by|via|in|with) (the )?(mail|mailbox)\b`),
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
	{encounterAsk,
		"Boss abilities and strategy: the game data holds no boss ability, spell, phase, enrage timer, health or resistance for any boss (instance_bosses has a name, an order and a mode, and no boss is linked to a creature), and no encounter strategy or interrupt list. Say so; name no ability, phase, timer or tactic from memory. What it does hold is a dungeon's or raid's bosses in order and its group size and level range."},
	{lockoutAsk,
		"Raid lockouts and resets: the game data holds no lockout, reset day or period, saved-instance or raid ID rule, and no limit on instances entered per hour. Say so; give no reset day, length or entry limit from memory. What it does hold is a raid's group size, level and entrance."},
	{difficultyAsk,
		"Dungeon difficulty and modes: the game data holds no difficulty rating, heroic or hard mode, and no gear or level recommendation beyond a dungeon's minimum and maximum level; an extra mode of a map in it is an alternate set of encounters with an id, not a named difficulty. Say so; call no dungeon heroic, easy or hard from memory. What it does hold is a dungeon's level range and group size."},
	{rolesAsk,
		"Group roles and threat: the game data holds no tank, healer or damage count for any dungeon or raid (instances has a group size and a level range only), no role flag on a spec, and no threat, aggro or taunt value. Say so; give no group composition or threat figure from memory. The advisor's own tables hold which talent trees fit a tank, a healer or a damage dealer and which stats each role weighs; use those for a spec or role question."},
	{classQuestAsk,
		"Class quest chains: the game data holds no class quest line and no prerequisite, follow-up or class restriction for any quest (its three quest lines are none of them a class's), and no quest reward; players' reports name only a few class quests, by title, level and faction. Say so; name no class quest, its order, its giver or its reward from memory. The class quests the player's own Questie holds are the ones in the Next list; name none besides."},
	{achievementAsk,
		"Achievements and titles: the game data holds no achievement, achievement point, criteria or achievement reward, and no character title (earned or otherwise); a quest row's title is only its name. Say so; name no achievement, point value or title and no way to earn one from memory."},
	{vanityAsk,
		"Companion pets and vanity items: the game data holds a pet, tabard or toy only as an item row (name, item level, binding, vendor price, and a loot or vendor source for some): no pet species, summon spell, pet ability or level, no collection or active-pet limit, no pet battle and no tabard or toy effect. Say so; give no limit, ability or effect from memory. Where a pet comes from is only what its loot and vendor rows say."},
	{toolEffectAsk,
		"Profession tool effects: the game data holds a profession tool only as an item row (name, binding, buy price, vendor): no skill bonus, durability, lure or bait effect, speed or charges, and no row saying what a tool is needed for. Say so; give no bonus or comparison between tools from memory. The advisor's own table of which profession needs which tool (pick, knife, hammer, spanner, rod, pole) is the only tool rule it states."},
	{regexp.MustCompile(`(?i)\bmacros?\b|/cast\b|/use\b|/castsequence\b`),
		"Macros: the game data holds no macro commands or macro text and the advisor has no macro advice. Say so; do not write a macro and present it as from the game data."},
}

// classQuestAsk is a question about a class's quests or quest chain: the class-only quests that teach a spell, a pet, a totem or a mount.
var classQuestAsk = regexp.MustCompile(`(?i)\bclass[- ]quests?\b|\bclass quest ?(chains?|lines?)\b|\b(warrior|paladin|hunter|rogue|priest|shaman|mage|warlock|druid)s?(?:'s)? (?:\w+ ){0,2}(?:quest ?chains?|quest ?lines?|quests?)\b|\b(quest ?chains?|quest ?lines?|quests?)\b.{0,25}\b(for|of|as) (?:a |an |the )?(warrior|paladin|hunter|rogue|priest|shaman|mage|warlock|druid)s?\b`)

// achievementAsk is a question about an achievement, its points or a character title: the Classic titles the advisor knows by name, or earning, unlocking or a reward of a title.
var achievementAsk = regexp.MustCompile(`(?i)\bachievements?\b|\bachievement points?\b|\b(earn|earns|earning|get|gets|getting|gain|unlock|unlocks|obtain|win|wins|how do i)\b.{0,25}\b(the )?titles?\b|\btitles?\b.{0,25}\b(earn\w*|rewards?|unlock\w*|obtain\w*|available|requirements?|for (killing|exploring|completing|reaching|winning))\b|\bwhat titles\b|\b(scarab lord|bloodsail admiral|loremaster|the explorer|the insane|salty|the diplomat|the seeker)\b.{0,15}\btitles?\b|\btitles?\b.{0,15}\b(scarab lord|bloodsail admiral|loremaster|the explorer|the insane|salty|the diplomat|the seeker)\b`)

// vanityAsk is a question about companion pets as a collection, a pet's abilities, tabards, toys or vanity items; a hunter's or warlock's pet is no companion.
var vanityAsk = regexp.MustCompile(`(?i)\b(companion|vanity|non-?combat|critter|battle) pets?\b|\bpet (journal|battles?|collection|species)\b|\b(my|the|your) (companions?|critters?)\b|\bcompanions? (limit|slots?|abilit\w+|collection)\b|\b(toys?|tabards?|vanity items?)\b`)

// toolEffectAsk is a question about what a profession tool gives (a skill bonus, durability, a lure) or which of two is better.
var toolEffectAsk = regexp.MustCompile(`(?i)\b(mining picks?|pickaxes?|skinning knives|skinning knife|blacksmith(?:ing)? hammers?|(?:runed )?(?:copper|silver|golden|truesilver|arcanite) rods?|enchanting rods?|arclight spanners?|fishing (?:poles?|rods?)|fishing lures?|bobbers?|thieves'? tools|jewelcrafting chisels?)\b.{0,60}\b(bonus|\+ ?\d+|skill|durability|break\w*|wears?|degrade\w*|last|faster|speed|best|better|upgrade\w*|stats?|charges?|difference)\b|\b(bonus|durability|best|better|faster|upgrade\w*|stats?|skill)\b.{0,40}\b(mining picks?|pickaxes?|skinning knives|skinning knife|blacksmith(?:ing)? hammers?|enchanting rods?|arclight spanners?|fishing (?:poles?|rods?|lures?|bait)|bobbers?)\b`)

// goldMaking is a question about making gold: a profession, a farm, a craft or an item for profit, or what sells well.
var goldMaking = regexp.MustCompile(`(?i)\b(make|making|makes|earn|earning|earns|farm|farming)\b.{0,25}\b(gold|money)\b|\bgold[- ]?(making|per hour|an hour|farm\w*|guide|tips)\b|\b(profit|profits|profitable|profiting|margins?|money[- ]?making|flip|flipping|undercut\w*)\b|\b(best|most)\b.{0,30}\b(gold|money)\b|\b(gold|money)\b.{0,30}\b(per hour|an hour|each hour|hourly)\b|\bwhat sells (best|well|fast)\b|\bworth (crafting|making|selling|farming|gathering)\b`)

// goldFees is a question about the gold cap, a repair bill, a respec or a guild bank.
var goldFees = regexp.MustCompile(`(?i)\bgold cap\b|\b(max|maximum|most|limit)\b.{0,15}\b(gold|money)\b.{0,15}\b(carry|hold|have)\b|\bhow much (gold|money)\b.{0,20}\b(carry|hold)\b|\brepair\w*\b.{0,30}\b(costs?|price|fees?|bills?|how much)\b|\b(costs?|price|how much)\b.{0,30}\brepair\w*\b|\brespec\w*\b.{0,30}\b(costs?|price|how much|gold)\b|\b(costs?|price|how much)\b.{0,30}\brespec\w*\b|\bguild bank\b`)

// holidayAsk is a question about a holiday or world event: one the advisor lists by name, a Darkmoon Faire, or "holiday" or "world event" words.
var holidayAsk = regexp.MustCompile(`(?i)\b(holidays?|world events?|seasonal events?|darkmoon faire|midsummer|fire festival|hallow'?s end|winter'?s? veil|lunar festival|love is in the air|noblegarden|children'?s week|harvest festival|brewfest)\b`)

// bossNames are the encounters a player names without the word boss: the bosses of the Classic raids and the last bosses of its dungeons.
const bossNames = `ragnaros|onyxia|nefarian|hakkar|kel'?thuzad|c'?thun|majordomo(?: executus)?|razorgore|vaelastrasz|broodlord(?: lashlayer)?|chromaggus|firemaw|ebonroc|flamegor|lucifron|magmadar|gehennas|garr|baron geddon|shazzrah|sulfuron(?: harbinger)?|golemagg|ossirian|kurinnaxx|rajaxx|moam|buru|ayamiss|anub'?rekhan|faerlina|maexxna|patchwerk|grobbulus|gluth|thaddius|heigan|loatheb|razuvious|gothik|sapphiron|jeklik|venoxis|mandokir|gri'?lek|hazza'?rah|renataki|wushoolay|thekal|arlokk|jin'?do|edwin vancleef|vancleef|arugal|rivendare|ras frostwhisper|rend blackhand|wyrmthalan|dagran thaurissan|emperor thaurissan|mekgineer thermaplugg|aku'?mai|ukorz sandscalp|amnennar|whitemane|hearthsinger forresten|morphaz|princess theradras|chief ukorz`

// instanceNames are the dungeons and raids a player names in a question about their difficulty or reset.
const instanceNames = `ragefire chasm|wailing caverns|deadmines|shadowfang keep|blackfathom deeps|stockade|gnomeregan|razorfen kraul|razorfen downs|scarlet monastery|uldaman|zul'?farrak|maraudon|sunken temple|temple of atal'?hakkar|blackrock depths|brd|blackrock spire|lbrs|ubrs|dire maul|scholomance|stratholme|strat|molten core|mc|onyxia'?s lair|blackwing lair|bwl|zul'?gurub|zg|ruins of ahn'?qiraj|aq20|ahn'?qiraj temple|aq40|naxxramas|naxx`

var (
	bossWord = `\b(boss(?:es)?|encounters?|` + bossNames + `)\b`
	// encounterAsk is a question about a boss's abilities, phases, health or the way to beat it.
	encounterAsk = regexp.MustCompile(`(?i)` + bossWord + `.{0,80}\b(abilit(?:y|ies)|phases?|enrages?|mechanics?|strateg(?:y|ies)|tactics?|interrupt\w*|spells?|casts?|resist\w*|immune|immunit\w*|hit points|health|hp|stun\w*|fear\w*|debuffs?|adds)\b|\b(abilit(?:y|ies)|phases?|enrages?|mechanics?|strateg(?:y|ies)|tactics?|interrupt\w*|spells?|casts?|resist\w*|immune|hit points|health|hp)\b.{0,80}` + bossWord + `|\bhow (?:do|can|should|would|to)\b.{0,12}\b(?:beat|kill|defeat|handle|survive|fight|solo|tank|pull)\b.{0,50}` + bossWord)

	// lockoutAsk is a question about a raid's or dungeon's lockout, reset or the limit on entering instances.
	lockoutAsk = regexp.MustCompile(`(?i)\blockouts?\b|\block(?:ed)? out\b|\braid ids?\b|\bsaved (?:to|raids?|instances?|dungeons?)\b|\binstances?\b.{0,30}\b(?:per|an|each|every) hour\b|\b(?:per|an|each|every) hour\b.{0,30}\binstances?\b|\b(?:raids?|instances?|dungeons?|` + instanceNames + `)\b.{0,40}\bresets?\b|\bresets?\b.{0,40}\b(?:raids?|instances?|dungeons?|` + instanceNames + `)\b`)

	// difficultyAsk is a question about a dungeon's or raid's difficulty or mode.
	difficultyAsk = regexp.MustCompile(`(?i)\bheroics?\b|\bmythics?\b|\bhard ?modes?\b|\bnormal (?:mode|difficulty)\b|\bdifficulty (?:levels?|settings?|ratings?)\b|\b(?:dungeons?|raids?|instances?|boss(?:es)?|` + instanceNames + `|` + bossNames + `)\b.{0,40}\b(?:difficult\w*|how hard|too hard|hard enough|how easy|easy enough)\b|\b(?:difficult\w*|how hard|too hard|hard enough|how easy|easy enough)\b.{0,40}\b(?:dungeons?|raids?|instances?|boss(?:es)?|` + instanceNames + `|` + bossNames + `)\b`)

	// rolesAsk is a question about the tanks, healers or damage dealers a group needs, or about threat.
	rolesAsk = regexp.MustCompile(`(?i)\b(?:how many|number of|ratio of|enough|composition|comp|setup)\b.{0,50}\b(?:tanks?|healers?|dps|damage dealers?)\b|\b(?:tanks?|healers?|dps|damage dealers?)\b.{0,50}\b(?:how many|number of|composition|ratio|enough)\b|\b(?:need|needs|needed|require\w*|bring)\b.{0,30}\b(?:tanks?|healers?|dps|damage dealers?)\b.{0,40}\b(?:dungeons?|raids?|instances?|groups?|part(?:y|ies)|` + instanceNames + `)\b|\b(?:group|party|raid) (?:composition|comp|setup|makeup|roles?)\b|\b(?:threat|aggro)\b.{0,40}\b(?:how much|numbers?|table|per second|tps|multipliers?|modifiers?|generat\w*|cap|limit|\d)`)
)

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

// mailArmor is the armor type: a question about mail armor is no question about the post.
var mailArmor = regexp.MustCompile(`(?i)\bmail (armou?r|gear|helms?|chest|pieces?|wearers?|proficiency|items?|boots|gloves|legs|shoulders|bracers|belts?)\b|\b(wear|wearing|wears|equip) mail\b|\bmail (and|or) (plate|leather|cloth)\b|\b(plate|leather|cloth|chain)(,)? (and|or) mail\b|\bmail at level\b`)

// rainOfFire is the warlock spell: a question about it is no question about weather.
var rainOfFire = regexp.MustCompile(`(?i)\brain of fire\b`)

// LimitFacts says what the game data does not hold about bags, banks, stacks, unique items, mail, the Auction House, gold making and gold fees, the trade window,
// holiday events, weather and time of day, zone levels, boss abilities, raid lockouts, dungeon difficulty, group roles and threat, battlegrounds, honor ranks, travel times, riding and mounts, class trainer prices, racial traits, capital city services, faction leaders and macros, for a question that asks about one. "" for any other question.
func LimitFacts(text string) string {
	text = mailArmor.ReplaceAllStringFunc(rainOfFire.ReplaceAllString(valleyOfHonor.ReplaceAllString(text, " "), " "), func(m string) string { return strings.ReplaceAll(m, "mail", "chainarmor") })
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
