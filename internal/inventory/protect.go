package inventory

import (
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/advice"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

// Needs is what the quests in a character's log need, by rule: an item a quest in the log needs is
// never advised for sale, toss or a mail to an alt. A ready quest still needs its items, so an
// objective marked done counts.
//
// The sources are the log's own item objectives (by name), and, from the player's QuestieDB when
// there is one, each log quest's item objectives, required source items and start item (by id).
type Needs struct {
	byID    map[int64][]string
	byName  map[string][]string
	texts   map[string]string // lower-cased quest title -> the objective text of the quest, lower-cased
	titles  map[string]string // lower-cased quest title -> title
	qx      *questie.Index
	done    map[int]bool
	titleOf map[int]string
	starts  map[int64][]string // item id -> titles of the quests the item starts that the character can still take
}

// NeedsOf reads what the quests in c's log need. qx may be nil.
func NeedsOf(c character.Character, qx *questie.Index) Needs {
	n := Needs{byID: map[int64][]string{}, byName: map[string][]string{}, texts: map[string]string{}, titles: map[string]string{}, qx: qx,
		done: map[int]bool{}, titleOf: map[int]string{}, starts: map[int64][]string{}}
	for _, id := range c.CompletedQuests {
		n.done[id] = true
	}
	for _, q := range c.Quests {
		if q.Failed != nil && *q.Failed {
			continue
		}
		var text strings.Builder
		for _, o := range q.Objectives {
			text.WriteString(" " + o.Text)
			if !strings.EqualFold(o.Type, "item") {
				continue
			}
			name, _, _ := strings.Cut(o.Text, ":")
			if name = strings.ToLower(strings.TrimSpace(name)); name != "" && !contains(n.byName[name], q.Title) {
				n.byName[name] = append(n.byName[name], q.Title)
			}
		}
		if qx != nil && q.QuestID != 0 {
			if qq := qx.Quests[q.QuestID]; qq != nil {
				text.WriteString(" " + qq.Objective)
				for _, id := range append(append([]int{}, qq.Items...), qq.SrcItems...) {
					if !contains(n.byID[int64(id)], q.Title) {
						n.byID[int64(id)] = append(n.byID[int64(id)], q.Title)
					}
				}
			}
		}
		n.titles[strings.ToLower(q.Title)] = q.Title
		n.texts[strings.ToLower(q.Title)] = strings.ToLower(text.String())
	}
	if qx != nil {
		for id, q := range qx.Quests {
			n.titleOf[id] = q.Name
		}
		n.startedByItems(c)
	}
	return n
}

// startedByItems notes the items that start a quest the character can still take: a quest that starts from a dropped item has no giver to
// go back to, so destroying the item loses the quest. A quest that is done, in the log, or closed to the character's race or class is no such quest.
func (n *Needs) startedByItems(c character.Character) {
	lv := 0
	if c.Level != nil {
		lv = *c.Level
	}
	c.Level = &lv
	w := advice.WhoOf(c)
	for id, q := range n.qx.Quests {
		if len(q.StartItems) == 0 || q.Name == "" || w.Done[id] || w.InLog[id] || !w.RaceAllows(q.Races) || (q.Classes != 0 && w.ClassID > 0 && q.Classes&(1<<(w.ClassID-1)) == 0) {
			continue
		}
		for _, it := range q.StartItems {
			if !contains(n.starts[int64(it)], q.Name) {
				n.starts[int64(it)] = append(n.starts[int64(it)], q.Name)
			}
		}
	}
}

// Starts returns the names of the quests the item starts that the character has not taken or done and could take, empty when none.
func (n Needs) Starts(id int64) []string {
	out := append([]string(nil), n.starts[id]...)
	sort.Strings(out)
	return out
}

// StartsReason is the keep line for an item that starts a quest.
func StartsReason(quests []string) string {
	return "starts " + strings.Join(quests, ", ") + ": use it to pick the quest up before you destroy it, there is no quest giver to go back to"
}

// For returns the titles of the log quests that need the item, empty when none does. A quest-bound
// item (quest class or bonding 4) is also needed when a log quest's objective text names it.
func (n Needs) For(id int64, name string, row store.Row) []string {
	var out []string
	add := func(l []string) {
		for _, t := range l {
			if !contains(out, t) {
				out = append(out, t)
			}
		}
	}
	add(n.byID[id])
	if name != "" {
		add(n.byName[strings.ToLower(name)])
	}
	if len(out) == 0 && row != nil && isQuestItem(row) && name != "" {
		ln := strings.ToLower(name)
		var titles []string
		for t, text := range n.texts {
			if strings.Contains(text, ln) {
				titles = append(titles, t)
			}
		}
		sort.Strings(titles)
		for _, t := range titles {
			add([]string{n.titles[t]})
		}
	}
	sort.Strings(out)
	return out
}

// Names lists the lower-cased item names the log's item objectives name.
func (n Needs) Names() []string {
	var out []string
	for k := range n.byName {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Finished names a completed quest that used the item, or "" when none is known.
func (n Needs) Finished(id int64) string {
	if n.qx == nil {
		return ""
	}
	var best int
	for qid, q := range n.qx.Quests {
		if !n.done[qid] {
			continue
		}
		for _, it := range append(append([]int{}, q.Items...), q.SrcItems...) {
			if int64(it) == id && (best == 0 || qid < best) {
				best = qid
			}
		}
	}
	if best == 0 {
		return ""
	}
	return n.qx.Quests[best].Name
}

// Reason is the keep line for a protected item.
func Reason(quests []string) string { return "needed for " + strings.Join(quests, ", ") }

// Protected is one stack the quest log needs.
type Protected struct {
	ItemID int64
	Item   string
	Quests []string
}

// Reason is "needed for <quest>".
func (p Protected) Reason() string { return Reason(p.Quests) }

// ProtectedItems lists, once per item, the items in the bags and bank that a quest in the log needs.
// ds and qx may be nil.
func ProtectedItems(c character.Character, ds store.DataStore, qx *questie.Index) []Protected {
	n := NeedsOf(c, qx)
	seen := map[int64]bool{}
	var out []Protected
	for _, stacks := range [][]character.ItemStack{c.Bags, c.Bank} {
		for _, s := range stacks {
			id := int64(s.ItemID)
			if seen[id] {
				continue
			}
			var row store.Row
			if ds != nil {
				if r, err := ds.Get("items", id); err == nil {
					row = r
				}
			}
			name := stackName(s, ds)
			if qs := n.For(id, name, row); len(qs) > 0 {
				seen[id] = true
				out = append(out, Protected{ItemID: id, Item: name, Quests: qs})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Item < out[j].Item })
	return out
}
