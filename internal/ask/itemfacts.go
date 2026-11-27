package ask

import (
	"regexp"
	"strings"

	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/names"
	"github.com/WoW-Companion/advisor/internal/store"
)

// tooltipWords make a question one about an item's tooltip: its levels, its binding, or whether it can be traded, mailed, auctioned or stacked.
var tooltipWords = regexp.MustCompile(`(?i)\b(trade|trading|tradeable|tradable|mail|mailed|mailing|soulbound|bound|bind|binds|bop|boe|auction|ah|tooltip|item level|ilvl|required level|requires|level|stack|stacks|stacking|unique)\b`)

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

// limitTopics are the game limits and tools the game data holds nothing on: a question that touches one gets the line that says so.
var limitTopics = []struct {
	about *regexp.Regexp
	fact  string
}{
	{regexp.MustCompile(`(?i)\b(bag|bags|backpack|bank|bank slots?)\b.*\b(slots?|size|sizes|space|capacity|big|bigger|biggest|many|price|cost)\b|\b(slots?|size|capacity)\b.*\b(bag|bags|backpack|bank)\b`),
		"Bag and bank size: the game data holds no bag slot count, no backpack or bank size and no bank slot price. Say so; give no slot count or price from memory."},
	{regexp.MustCompile(`(?i)\bstack(s|ed|ing)?\b|\bhow many\b.*\b(carry|hold|fit)\b`),
		"Stack size: the game data holds no maximum stack size for any item. Say so; give no stack size from memory (the capture's own counts are what the character holds)."},
	{regexp.MustCompile(`(?i)\bunique(?:-equipped)?\b|\bonly one\b.*\b(carry|equip|wear|hold)\b|\b(carry|equip|wear|hold)\b.*\bonly one\b`),
		"Unique items: the game data holds no unique or unique-equipped flag and no per-character limit on an item. Say so; call no item unique."},
	{regexp.MustCompile(`(?i)\b(mail|mailbox|mails|postage|cod)\b.*\b(attach\w*|limit|expire\w*|days?|cost|fee|postage|cod|how many|max\w*|return\w*)\b|\b(attach\w*|postage|cod)\b.*\bmail`),
		"Mail: the game data holds no mail rules (items per mail, postage, cash on delivery, how long a mail is kept). Say so; give no figure from memory. What the data does hold is an item's binding, which decides whether it can be mailed at all."},
	{regexp.MustCompile(`(?i)\b(auction|auctions|auction house|ah)\b.*\b(deposit|fee|cut|duration|hours?|how long|listing|listings|limit|max\w*|price|prices|worth|cost|much|expire\w*)\b|\b(deposit|listing)\b.*\bauction`),
		"Auction House: the game data holds no Auction House prices, deposit, cut, listing duration or listing limit. Say so; name no price or fee from memory. What the data does hold is an item's binding and quality, which decide whether it can be listed."},
	{regexp.MustCompile(`(?i)\btrade (window|slots?)\b|\b(trade|trading)\b.*\b(how many|limit|max\w*|slots?|gold|distance|range)\b`),
		"Trade window: the game data holds no trade window rules (items or gold per trade, range). Say so; give no figure from memory. What the data does hold is an item's binding, which decides whether it can be traded."},
	{regexp.MustCompile(`(?i)\b(battlegrounds?|bgs?|wsg|warsong gulch|arathi basin|alterac valley)\b`),
		"Battlegrounds: the game data holds no battleground row (no entry level, level bracket, team size, queue or objective rule, map or reward). Say so; give no level or bracket from memory. What the data does hold is the level, class and binding of a battleground's consumable or rank item."},
	{regexp.MustCompile(`(?i)\bhonou?r (ranks?|points?|system|vendors?|rewards?|kills?|decay|standing|costs?|tokens?)\b|\b(how much|how many|earn|earns|gain|gains|gives?|gets?)\b.{0,30}\bhonou?r\b|\b(ranks?|points?)\b.{0,20}\bhonou?r\b|\bmarks? of honou?r\b|\bpvp\b.{0,20}\b(ranks?|titles?|vendors?|rewards?|sets?|gear)\b|\b(grand marshal|field marshal|high warlord|lieutenant (commander|general)|knight-(captain|lieutenant|champion)|stone guard|blood guard|legionnaire|centurion)\b`),
		"Honor and PvP ranks: the game data holds no honor points, rank, rank requirement or title, and no honor vendor or its prices. Say so; give no honor cost, rank number or title from memory. What the data does hold is a rank piece's required level, class and binding, and which items only one faction can wear."},
	{regexp.MustCompile(`(?i)\bmacros?\b|/cast\b|/use\b|/castsequence\b`),
		"Macros: the game data holds no macro commands or macro text and the advisor has no macro advice. Say so; do not write a macro and present it as from the game data."},
}

// valleyOfHonor is Orgrimmar's district: a question about it is no question about honor.
var valleyOfHonor = regexp.MustCompile(`(?i)\bvalley of honou?r\b`)

// LimitFacts says what the game data does not hold about bags, banks, stacks, unique items, mail, the Auction House, the trade window,
// battlegrounds, honor ranks and macros, for a question that asks about one. "" for any other question.
func LimitFacts(text string) string {
	text = valleyOfHonor.ReplaceAllString(text, " ")
	var out []string
	for _, tp := range limitTopics {
		if tp.about.MatchString(text) {
			out = append(out, tp.fact)
		}
	}
	return strings.Join(out, "\n")
}
