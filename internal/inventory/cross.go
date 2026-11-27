package inventory

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/classreagent"
	"github.com/WoW-Companion/advisor/internal/gear"
	"github.com/WoW-Companion/advisor/internal/petfood"
	"github.com/WoW-Companion/advisor/internal/professions"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Action is what to do with a stack, judged across all the captured characters.
type Action string

const (
	Sell   Action = "sell"
	Mail   Action = "mail"
	Keep_  Action = "keep"
	Toss   Action = "toss"
	Unsure Action = "unsure"
)

// Actions is the order the answer lists them in.
var Actions = []Action{Sell, Mail, Keep_, Toss, Unsure}

// FarBelow is how many levels under a character an item's required level must be for gear to count
// as far below their level.
const FarBelow = 10

// Line is one stack and what to do with it.
type Line struct {
	Character string
	ItemID    int64
	Item      string
	Count     int
	Where     string // "bags" or "bank"
	Action    Action
	To        string // the character to mail to, for Mail
	Reason    string
	Price     *int64 // copper per item the vendor pays; nil when the dataset has none
	AH        bool   // Sell: advice is the Auction House, so no vendor value is counted
}

// CrossResult is the review across all characters.
type CrossResult struct {
	Lines []Line
	// SellValue is the copper the vendor stacks sell for, over the stacks with a known price.
	SellValue    int64
	SellUnpriced int // vendor stacks with no known price, left out of SellValue
	SellAuction  int // Sell stacks the advice sends to the Auction House: the dataset holds no price for them
	SlotsFreed   int // bag slots emptied by selling, tossing or mailing what sits in the bags
	Notes        []string
}

func displayName(c character.Character) string {
	if c.Name != "" {
		return c.Name
	}
	return c.Key
}

func unseenNotes(chars []character.Character) []string {
	var out []string
	for _, c := range chars {
		if !c.BankCaptured {
			out = append(out, displayName(c)+": bank not seen yet (open it once with the addon running); only the bags count.")
		}
	}
	return out
}

func stackName(s character.ItemStack, ds store.DataStore) string {
	if s.Name != "" {
		return s.Name
	}
	if ds != nil {
		if row, err := ds.Get("items", int64(s.ItemID)); err == nil && row != nil {
			if n, _ := row["name"].(string); n != "" {
				return n
			}
		}
	}
	return fmt.Sprintf("item %d", s.ItemID)
}

// Holding is one character's stack of a found item.
type Holding struct {
	Character string
	Where     string
	Count     int
}

// Found is one item and everyone who holds it.
type Found struct {
	ItemID   int
	Item     string
	Holdings []Holding
	Total    int
}

// FindResult answers "where is my X".
type FindResult struct {
	Query string
	Items []Found
	Notes []string // characters whose bank was never seen
}

// Find lists every character holding an item whose name contains query (a trailing plural "s" is
// forgiven), in bags or bank, and says which banks were never seen.
func Find(chars []character.Character, query string, ds store.DataStore) FindResult {
	r := FindResult{Query: strings.TrimSpace(query)}
	q := strings.ToLower(r.Query)
	q = strings.TrimSpace(strings.TrimPrefix(q, "my "))
	alt := strings.TrimSuffix(q, "s")
	by := map[int]*Found{}
	var order []int
	if q != "" {
		for _, c := range chars {
			for _, loc := range []struct {
				where  string
				stacks []character.ItemStack
			}{{"bags", c.Bags}, {"bank", c.Bank}} {
				for _, s := range loc.stacks {
					n := stackName(s, ds)
					ln := strings.ToLower(n)
					if !strings.Contains(ln, q) && !strings.Contains(ln, alt) {
						continue
					}
					f := by[s.ItemID]
					if f == nil {
						f = &Found{ItemID: s.ItemID, Item: n}
						by[s.ItemID] = f
						order = append(order, s.ItemID)
					}
					merged := false
					for i, h := range f.Holdings {
						if h.Character == displayName(c) && h.Where == loc.where {
							f.Holdings[i].Count += s.Count
							merged = true
						}
					}
					if !merged {
						f.Holdings = append(f.Holdings, Holding{displayName(c), loc.where, s.Count})
					}
					f.Total += s.Count
				}
			}
		}
	}
	for _, id := range order {
		r.Items = append(r.Items, *by[id])
	}
	sort.SliceStable(r.Items, func(i, j int) bool { return r.Items[i].Item < r.Items[j].Item })
	r.Notes = unseenNotes(chars)
	return r
}

// RenderFind is the plain-text form of a Find result.
func RenderFind(r FindResult) string {
	var b strings.Builder
	if len(r.Items) == 0 {
		fmt.Fprintf(&b, "No character has %q in the bags or banks I can see.\n", r.Query)
	}
	for _, f := range r.Items {
		fmt.Fprintf(&b, "%s: %d in all\n", f.Item, f.Total)
		for _, h := range f.Holdings {
			fmt.Fprintf(&b, "- %s: %d in the %s\n", h.Character, h.Count, h.Where)
		}
	}
	for _, n := range r.Notes {
		b.WriteString("Note: " + n + "\n")
	}
	return b.String()
}

var drinkWord = regexp.MustCompile(`(?i)\b(water|drink|juice|tea|milk|wine|ale|brew|cider|coffee)\b`)

// item classes read from the dataset's items table
const (
	classConsumable = 0
	classAmmo       = 6
	classQuiver     = 11 // a quiver or an ammo pouch; class 1 is a plain or special bag
	classQuest      = 12
	classKey        = 13 // a key: a dungeon, door or chest key, kept on the key ring
	subFood         = 5
)

func rowInt(row store.Row, col string) (int64, bool) {
	v, ok := row[col].(int64)
	return v, ok
}

// consumableKind names the stack's kind for the keep-the-best rule: "food", "drink", "arrows" or
// "bullets"; "" for anything else.
func consumableKind(name string, row store.Row) string {
	cl, _ := rowInt(row, "class_id")
	sub, _ := rowInt(row, "subclass_id")
	switch {
	case cl == classConsumable && sub == subFood:
		if drinkWord.MatchString(name) {
			return "drink"
		}
		return "food"
	case cl == classAmmo && sub == 2:
		return "arrows"
	case cl == classAmmo && sub == 3:
		return "bullets"
	}
	return ""
}

// containerAction places a bag, quiver or ammo pouch that the holder cannot use: one whose class restriction leaves
// their class out (a Soul Pouch is a warlock's), or a quiver or ammo pouch for a class that cannot use ammo. It goes to
// an alt that can use it, else it sells. ok is false when the holder can use it: the dataset holds no bag size to compare.
func containerAction(chars []character.Character, i int, row store.Row) (a Action, to, why string, ok bool) {
	cl, _ := rowInt(row, "class_id")
	canHold := func(o character.Character) (hold, known bool) {
		allowed, known := gear.AllowsClass(o.Class, row)
		if !known {
			return false, false
		}
		return allowed && (cl != classQuiver || usesAmmo(o.Class)), true
	}
	if hold, known := canHold(chars[i]); hold || !known {
		return "", "", "", false
	}
	what := "its class restriction leaves your class out"
	if allowed, _ := gear.AllowsClass(chars[i].Class, row); allowed {
		what = "your class cannot use ammo, so it has no use for a quiver or ammo pouch"
	}
	for j, o := range chars {
		if hold, _ := canHold(o); j != i && hold {
			if boundOnPickup(row) {
				return Sell, "", what + "; " + displayName(o) + " could use it, but it is bound to you when picked up and cannot be mailed", true
			}
			return Mail, displayName(o), displayName(o) + " can use it", true
		}
	}
	return Sell, "", what, true
}

// rangedAmmo is the ammo the character's equipped ranged weapon fires, "arrows" for a bow or a crossbow and "bullets" for a gun; "" when no
// ranged weapon is equipped or known, or it fires none (a wand, a thrown weapon).
func rangedAmmo(c character.Character, ds store.DataStore) string {
	it, ok := c.Equipment["ranged"]
	if !ok || it.ItemID == nil || ds == nil {
		return ""
	}
	row, err := ds.Get("items", int64(*it.ItemID))
	if err != nil {
		return ""
	}
	if cl, _ := rowInt(row, "class_id"); cl != 2 {
		return ""
	}
	switch sub, _ := rowInt(row, "subclass_id"); sub {
	case 2, 18: // bow, crossbow
		return "arrows"
	case 3: // gun
		return "bullets"
	}
	return ""
}

func usesAmmo(class string) bool {
	switch strings.ToUpper(class) {
	case "HUNTER", "ROGUE", "WARRIOR":
		return true
	}
	return false
}

// isQuestItem is a quest item by its class or its quest binding; a key is no quest item whatever its binding (a dungeon key is bound to a
// quest in the dataset, yet it is kept, never destroyed).
func isQuestItem(row store.Row) bool {
	cl, _ := rowInt(row, "class_id")
	b, _ := rowInt(row, "bonding")
	return cl != classKey && (cl == classQuest || b == 4 || b == 5)
}

// isKey is a key by its item class.
func isKey(row store.Row) bool {
	cl, _ := rowInt(row, "class_id")
	return cl == classKey
}

// requiredLevel is the item's required level, estimated from its item level when the dataset
// holds 0; ok is false when neither is known.
func requiredLevel(row store.Row) (int, bool) {
	if rl, _ := rowInt(row, "required_level"); rl > 0 {
		return int(rl), true
	}
	if il, _ := rowInt(row, "item_level"); il > 0 {
		return gear.EstimateRequiredLevel(int(il)), true
	}
	return 0, false
}

func level(c character.Character) int {
	if c.Level == nil {
		return 0
	}
	return *c.Level
}

func hasProfession(c character.Character, p string) bool { return HasProfession(c, p) }

// ReviewAll judges every stack of every character: sell, mail to an alt, keep or toss. It never
// guesses: an item the dataset lacks, or a rule the dataset cannot settle, is "unsure", and a price
// the dataset lacks stays unknown.
func ReviewAll(chars []character.Character, ds store.DataStore) CrossResult {
	return ReviewAllWith(chars, ds, nil)
}

// ReviewAllWith is ReviewAll that also reads the quests' items from the player's QuestieDB (qx may be nil).
// An item a quest in a character's log needs is kept by rule: never sold, tossed or mailed away.
func ReviewAllWith(chars []character.Character, ds store.DataStore, qx *questie.Index) CrossResult {
	var res CrossResult
	reviews := make([]Result, len(chars))
	for i, c := range chars {
		reviews[i] = ReviewWith(c, ds, qx)
	}
	res.Notes = unseenNotes(chars)
	if ds == nil {
		res.Notes = append(res.Notes, "No dataset configured: gear, professions, quality and prices are unknown.")
	}
	var users map[int64][]string
	usersOK := false
	if ds != nil {
		users, usersOK = professions.ReagentUsers(ds)
		if !usersOK {
			res.Notes = append(res.Notes, "The dataset has no recipes: profession materials are unknown.")
		}
	}
	// upgrades other characters could wear, and quests that name an item.
	upgrade := map[int64][]string{}
	questHolders := map[string][]string{}
	needs := make([]Needs, len(chars))
	for i, c := range chars {
		for _, e := range reviews[i].Groups[Equip] {
			upgrade[e.ItemID] = append(upgrade[e.ItemID], displayName(c)+"|"+e.Reason)
		}
		needs[i] = NeedsOf(c, qx)
		for _, name := range needs[i].Names() {
			questHolders[name] = append(questHolders[name], displayName(c))
		}
	}
	rowOf := func(id int64) store.Row {
		if ds == nil {
			return nil
		}
		if row, err := ds.Get("items", id); err == nil {
			return row
		}
		return nil
	}
	for i, c := range chars {
		me := displayName(c)
		// the best food, drink and ammo this character can already use
		best := map[string]int{}
		for _, stacks := range [][]character.ItemStack{c.Bags, c.Bank} {
			for _, s := range stacks {
				row := rowOf(int64(s.ItemID))
				if row == nil {
					continue
				}
				if k := consumableKind(stackName(s, ds), row); k != "" {
					if rl, ok := requiredLevel(row); ok && rl <= level(c) && rl > best[k] {
						best[k] = rl
					}
				}
			}
		}
		for _, g := range Order {
			for _, e := range reviews[i].Groups[g] {
				row := rowOf(e.ItemID)
				l := Line{Character: me, ItemID: e.ItemID, Item: e.Name, Count: e.Count, Where: e.Where, Price: e.SellPrice}
				switch g {
				case Equip, Keep, Quest:
					l.Action, l.Reason = Keep_, e.Reason
				case Vendor:
					if row != nil && isQuestItem(row) {
						// a grey quest item the log does not need: the quest rule decides, not the vendor
						l.Action, l.To, l.Reason, l.AH = decide(chars, i, e, row, ds, users, usersOK, upgrade, questHolders, best, needs[i])
						break
					}
					l.Action, l.Reason = Sell, "grey item, only worth its vendor price"
				default:
					l.Action, l.To, l.Reason, l.AH = decide(chars, i, e, row, ds, users, usersOK, upgrade, questHolders, best, needs[i])
				}
				res.Lines = append(res.Lines, l)
				if l.Where == "bags" && (l.Action == Sell || l.Action == Toss || l.Action == Mail) {
					res.SlotsFreed++
				}
				if l.Action == Sell && l.AH {
					res.SellAuction++
				}
				if l.Action == Sell && !l.AH {
					if l.Price == nil {
						res.SellUnpriced++
					} else {
						res.SellValue += *l.Price * int64(l.Count)
					}
				}
			}
		}
	}
	sort.SliceStable(res.Lines, func(i, j int) bool {
		a, b := res.Lines[i], res.Lines[j]
		if a.Character != b.Character {
			return a.Character < b.Character
		}
		if a.Item != b.Item {
			return a.Item < b.Item
		}
		return a.Where < b.Where
	})
	return res
}

// decide is the cross-character rule for a stack the per-character review left in the bank or
// unknown groups.
func decide(chars []character.Character, i int, e Entry, row store.Row, ds store.DataStore, users map[int64][]string, usersOK bool,
	upgrade map[int64][]string, questHolders map[string][]string, best map[string]int, need Needs) (a Action, to, why string, ah bool) {
	c := chars[i]
	me := displayName(c)
	if row == nil {
		return Unsure, "", "the dataset does not know this item", false
	}
	// an upgrade for another character: a piece bound on pickup is bound to its holder and cannot be mailed
	for _, u := range upgrade[e.ItemID] {
		who, reason, _ := strings.Cut(u, "|")
		if who != me && !boundOnPickup(row) {
			return Mail, who, reason, false
		}
	}
	if isQuestItem(row) {
		holders := questHolders[strings.ToLower(e.Name)]
		for _, h := range holders {
			if h != me {
				// a quest item is bound to the one who looted it: it cannot be mailed or traded, so the alt must get their own
				return Toss, "", "a quest item cannot be mailed or traded, so " + h + " must get their own for the quest in " + h + "'s log; no quest in your log needs it", false
			}
		}
		if q := need.Finished(e.ItemID); q != "" {
			return Toss, "", "safe to destroy: " + q + " is completed and no quest in your log needs it", false
		}
		return Toss, "", "safe to destroy: no quest in your log needs it", false
	}
	if p := professions.ToolOf(e.Name); p != "" {
		// a profession's tool is no gear: its owner keeps it, and another character with the profession is sent it
		if hasProfession(c, p) {
			return Keep_, "", "the " + p + " tool: " + p + " needs it in your bags", false
		}
		for j, o := range chars {
			if j != i && hasProfession(o, p) {
				who := displayName(o)
				if boundOnPickup(row) {
					return Sell, "", "the " + p + " tool of " + who + "'s " + p + ", but it is bound to you when picked up and cannot be mailed", false
				}
				return Mail, who, "the " + p + " tool, for " + who + "'s " + p, false
			}
		}
		return Sell, "", "no character has " + p + ", so its tool has no use", false
	}
	if IsBag(row) {
		if a, to, why, ok := containerAction(chars, i, row); ok {
			return a, to, why, false
		}
		return Unsure, "", BagReason, false
	}
	if r, ok := classreagent.Of(e.Name); ok {
		// a class reagent goes to an alt of the class whose spells use it, unless it is bound to its holder
		for j, o := range chars {
			if j != i && strings.EqualFold(o.Class, r.Class) {
				who := displayName(o)
				if boundOnPickup(row) {
					return Sell, "", "a reagent for " + who + "'s " + strings.ToLower(r.Class) + " spells, but it is bound to you when picked up and cannot be mailed", false
				}
				return Mail, who, "a reagent for " + who + "'s " + strings.ToLower(r.Class) + " spells (" + r.Spells + ")", false
			}
		}
	}
	if d, ok := petfood.Of(e.Name); ok {
		// pet food goes to a hunter alt, whose pet eats it, unless it is bound to its holder
		for j, o := range chars {
			if j != i && strings.EqualFold(o.Class, "HUNTER") {
				who := displayName(o)
				if boundOnPickup(row) {
					return Sell, "", "pet food for " + who + "'s pet, but it is bound to you when picked up and cannot be mailed", false
				}
				return Mail, who, "pet food (" + d + ") for " + who + "'s pet", false
			}
		}
	}
	if k := consumableKind(e.Name, row); k != "" {
		if w := rangedAmmo(c, ds); (k == "arrows" || k == "bullets") && usesAmmo(c.Class) && w != "" && w != k {
			return Unsure, "", "your ranged weapon fires " + w + ", not " + k + "; keep them only if you plan to change weapon", false
		}
		if (k == "arrows" || k == "bullets") && !usesAmmo(c.Class) {
			// an alt that can use ammo gets it, unless it is bound to its holder
			for j, o := range chars {
				if j != i && usesAmmo(o.Class) && !boundOnPickup(row) {
					return Mail, displayName(o), displayName(o) + " can use " + k, false
				}
			}
			return Sell, "", "your class cannot use ammo", false
		}
		rl, ok := requiredLevel(row)
		switch {
		case !ok:
			return Unsure, "", "its required level is unknown", false
		case rl > level(c):
			return Keep_, "", fmt.Sprintf("you can use it from level %d", rl), false
		case rl >= best[k]:
			return Keep_, "", "the best " + k + " you can use now", false
		}
		return Sell, "", "a better " + k + " is in your bags or bank", false
	}
	if usersOK {
		if ps := users[e.ItemID]; len(ps) > 0 {
			for _, p := range ps {
				if hasProfession(c, p) {
					return Keep_, "", "material for your " + p, false
				}
			}
			var alts []string
			for j, o := range chars {
				if j == i {
					continue
				}
				for _, p := range ps {
					if hasProfession(o, p) {
						alts = append(alts, displayName(o)+"|"+p)
						break
					}
				}
			}
			if len(alts) > 0 {
				sort.Strings(alts)
				who, p, _ := strings.Cut(alts[0], "|")
				if boundOnPickup(row) {
					return Sell, "", "material for " + who + "'s " + p + ", but it is bound to you when picked up and cannot be mailed", false
				}
				return Mail, who, "material for " + who + "'s " + p, false
			}
			if q, ok := rowInt(row, "quality"); ok && q > poor {
				if b, ok := rowInt(row, "bonding"); ok && (b == 0 || b == 2) {
					return Sell, "", "no character has " + strings.Join(ps, " or ") + ": sell on the Auction House", true
				}
			}
			return Sell, "", "no character has " + strings.Join(ps, " or "), false
		}
	}
	if i64(row["slot"]) != 0 {
		a, to, why, ah := gearAction(chars, i, e, row)
		if a == Sell && auctionGear(row) {
			// a Bind on Equip piece of uncommon quality or better sells for more on the Auction House than to a vendor
			return a, to, why + ": it is tradeable, so sell it on the Auction House", true
		}
		return a, to, why, ah
	}
	return Unsure, "", "no rule places it", false
}

// auctionGear reports whether a piece of gear can be sold on the Auction House: not bound on pickup and of
// uncommon quality or better (the same rule the item paths use).
func auctionGear(row store.Row) bool {
	q, _ := rowInt(row, "quality")
	b, _ := rowInt(row, "bonding")
	return q >= 2 && (b == 0 || b == 2)
}

func gearAction(chars []character.Character, i int, e Entry, row store.Row) (Action, string, string, bool) {
	rl, rlOK := requiredLevel(row)
	levelless := gear.Levelless(row) // a shirt, a tabard or cosmetic armor has no level to fall behind
	anyKnown := false
	useful, from := "", 0
	for k := 0; k < len(chars); k++ {
		j := (i + k) % len(chars) // this character first
		o := chars[j]
		// An item above the character's level is judged at the level it needs: a warrior's plate helm
		// that needs level 40 can be worn from 40, when plate is theirs, though mail is theirs now.
		at := level(o)
		if rlOK && !levelless && rl > at {
			at = rl
		}
		ok, known := gear.CanUse(o.Class, at, row)
		if !known {
			continue
		}
		anyKnown = true
		far := !levelless && rlOK && level(o) > 0 && rl <= level(o)-FarBelow
		if ok && !far {
			useful = displayName(o)
			if !levelless && rlOK && level(o) > 0 && rl > level(o) {
				from = rl // not wearable yet: the item's required level is above theirs
			}
			break
		}
	}
	switch {
	case !anyKnown:
		return Unsure, "", "gear rules are unknown for these classes", false
	case useful == "":
		return Sell, "", "no character can use it, or it is far below their level", false
	case useful == displayName(chars[i]) && from > 0:
		return Keep_, "", fmt.Sprintf("you can wear it from level %d", from), false
	case useful == displayName(chars[i]):
		return Keep_, "", "you can wear it and it is not far below your level", false
	case boundOnPickup(row):
		return Sell, "", useful + " could wear it, but it is bound to you when picked up and cannot be mailed", false
	case from > 0:
		return Mail, useful, fmt.Sprintf("%s can wear it from level %d", useful, from), false
	}
	return Mail, useful, useful + " can wear it", false
}

// boundOnPickup reports a Bind on Pickup item (bonding 1): it is bound to whoever picked it up, so it cannot be
// mailed to another character.
func boundOnPickup(row store.Row) bool {
	b, _ := rowInt(row, "bonding")
	return b == 1
}

func moneyOrUnknown(l Line) string {
	if l.AH {
		return "value on the Auction House"
	}
	if l.Price == nil {
		return "sell price unknown"
	}
	return "sells for " + Money(*l.Price*int64(l.Count))
}

const maxLines = 12

// sellHead is what follows the Sell heading: the vendor value of the stacks sold to a vendor, and the stacks sent to the Auction House, which
// the dataset holds no price for and which are left out of that value (n is the stacks under Sell).
func sellHead(r CrossResult, n int) string {
	if n > 0 && r.SellAuction == n {
		return ": all for the Auction House, which the data holds no price for"
	}
	var head string
	if r.SellUnpriced > 0 && r.SellValue == 0 {
		head = ": value unknown"
	} else {
		head = ": " + Money(r.SellValue)
		if r.SellUnpriced > 0 {
			head += fmt.Sprintf(" (%d stack(s) unpriced, left out)", r.SellUnpriced)
		}
	}
	if r.SellAuction > 0 {
		head += fmt.Sprintf(" at a vendor, plus %d stack(s) for the Auction House (no price in the data)", r.SellAuction)
	}
	return head
}

// RenderAll is the plain-text form: Sell, Mail, Keep, Toss, then what is not sure.
func RenderAll(r CrossResult, chars int) string {
	var b strings.Builder
	if len(r.Lines) == 0 {
		b.WriteString("No bag or bank items captured for any character.\n")
	}
	titles := map[Action]string{Sell: "Sell", Mail: "Mail", Keep_: "Keep", Toss: "Toss", Unsure: "Not sure"}
	for _, a := range Actions {
		var ls []Line
		for _, l := range r.Lines {
			if l.Action == a {
				ls = append(ls, l)
			}
		}
		if len(ls) == 0 {
			continue
		}
		head := fmt.Sprintf("%s (%d)", titles[a], len(ls))
		if a == Sell {
			head += sellHead(r, len(ls))
		}
		b.WriteString(head + "\n")
		for n, l := range ls {
			if n == maxLines {
				fmt.Fprintf(&b, "- …and %d more\n", len(ls)-maxLines)
				break
			}
			who := ""
			if chars > 1 {
				who = " on " + l.Character
			}
			fmt.Fprintf(&b, "- %s x%d%s (%s)", l.Item, l.Count, who, l.Where)
			switch a {
			case Mail:
				fmt.Fprintf(&b, ": mail to %s; %s", l.To, l.Reason)
			case Sell:
				fmt.Fprintf(&b, ": %s; %s", l.Reason, moneyOrUnknown(l))
			default:
				b.WriteString(": " + l.Reason)
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Bag slots freed: %d\n", r.SlotsFreed)
	for _, n := range r.Notes {
		b.WriteString("Note: " + n + "\n")
	}
	return b.String()
}

// MailFor is what the other characters hold that the named character's professions or class use.
// ok is false when no captured character has that name.
func MailFor(chars []character.Character, target string, ds store.DataStore) (r CrossResult, name string, ok bool) {
	for _, c := range chars {
		if strings.EqualFold(c.Name, target) || strings.EqualFold(c.Key, target) {
			name, ok = displayName(c), true
		}
	}
	if !ok {
		return r, "", false
	}
	all := ReviewAll(chars, ds)
	r.Notes = all.Notes
	for _, l := range all.Lines {
		if l.Action == Mail && l.To == name {
			r.Lines = append(r.Lines, l)
		}
	}
	return r, name, true
}

// RenderMail is the plain-text form of MailFor.
func RenderMail(r CrossResult, name string) string {
	var b strings.Builder
	if len(r.Lines) == 0 {
		fmt.Fprintf(&b, "Nothing on your other characters is for %s's professions or class.\n", name)
	} else {
		fmt.Fprintf(&b, "Mail to %s (%d)\n", name, len(r.Lines))
		for n, l := range r.Lines {
			if n == maxLines*2 {
				fmt.Fprintf(&b, "- …and %d more\n", len(r.Lines)-n)
				break
			}
			fmt.Fprintf(&b, "- %s x%d from %s (%s): %s\n", l.Item, l.Count, l.Character, l.Where, l.Reason)
		}
	}
	for _, n := range r.Notes {
		b.WriteString("Note: " + n + "\n")
	}
	return b.String()
}
