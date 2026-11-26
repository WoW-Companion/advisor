package questie

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Event is a yearly holiday event. Windows are the usual dates (the game's calendar moves some by a
// few days each year); a quest of an event is offered only inside its window.
type Event struct {
	Key   string // short stable key, used in the saved setting
	Name  string
	Sort  int // the game's quest-sort id for the event's quests (a public, long-standing id list)
	Start [2]int
	End   [2]int // month, day; an End before Start runs over New Year
}

// Events is the fact table. Dates follow the public holiday calendars (Wowhead and Blizzard's
// event pages); sort ids are the quest-sort ids the game data gives holiday quests.
var Events = []Event{
	{"midsummer", "Midsummer Fire Festival", -369, [2]int{6, 21}, [2]int{7, 5}},
	{"hallows-end", "Hallow's End", -1003, [2]int{10, 18}, [2]int{11, 1}},
	{"winter-veil", "Feast of Winter Veil", -1001, [2]int{12, 16}, [2]int{1, 2}},
	{"lunar-festival", "Lunar Festival", -366, [2]int{1, 21}, [2]int{2, 4}},
	{"love-is-in-the-air", "Love is in the Air", -376, [2]int{2, 6}, [2]int{2, 20}},
	{"noblegarden", "Noblegarden", -374, [2]int{4, 1}, [2]int{4, 14}},
	{"childrens-week", "Children's Week", -1002, [2]int{5, 1}, [2]int{5, 7}},
	{"harvest-festival", "Harvest Festival", -1005, [2]int{9, 22}, [2]int{9, 29}},
	{"brewfest", "Brewfest", -370, [2]int{9, 20}, [2]int{10, 6}},
}

// eventNPCs are givers that stand only while their event runs; a quest they start belongs to it.
var eventNPCs = map[string]string{
	"festival talespinner": "midsummer", "festival flamekeeper": "midsummer", "festival loremaster": "midsummer",
}

// EventByKey finds an event by its key.
func EventByKey(key string) (Event, bool) {
	for _, e := range Events {
		if e.Key == key {
			return e, true
		}
	}
	return Event{}, false
}

// FindEvent matches free text ("Midsummer", "hallows end", "winter veil") to an event.
func FindEvent(text string) (Event, bool) {
	t := normEvent(text)
	if t == "" {
		return Event{}, false
	}
	for _, e := range Events {
		if t == normEvent(e.Key) || strings.Contains(normEvent(e.Name), t) || strings.Contains(t, normEvent(e.Name)) {
			return e, true
		}
	}
	for _, e := range Events {
		if first := strings.Fields(strings.ReplaceAll(e.Key, "-", " "))[0]; len(first) > 4 && strings.Contains(t, first) {
			return e, true
		}
	}
	return Event{}, false
}

func normEvent(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == ' ' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func eventOfSort(sort int) string {
	for _, e := range Events {
		if e.Sort == sort {
			return e.Key
		}
	}
	return ""
}

// Running reports whether the event's window holds t.
func (e Event) Running(t time.Time) bool {
	md := [2]int{int(t.Month()), t.Day()}
	less := func(a, b [2]int) bool { return a[0] < b[0] || a[0] == b[0] && a[1] <= b[1] } // a <= b
	if less(e.Start, e.End) {
		return less(e.Start, md) && less(md, e.End)
	}
	return less(e.Start, md) || less(md, e.End)
}

// Until is the window's last day as "July 5".
func (e Event) Until() string {
	return time.Month(e.End[0]).String() + " " + fmt.Sprint(e.End[1])
}

// RunningEvents are the events whose window holds t, in table order.
func RunningEvents(t time.Time) []Event {
	var out []Event
	for _, e := range Events {
		if e.Running(t) {
			out = append(out, e)
		}
	}
	return out
}

// Mention is the one line shown while an event runs and the player has not opted in.
func (e Event) Mention() string {
	return fmt.Sprintf("%s is on (until %s). Ask me if you want its quests", e.Name, e.Until())
}

// markEvents sets each quest's Event from its quest sort, else from an event-only giver.
func (ix *Index) markEvents() {
	for _, q := range ix.Quests {
		q.Event = eventOfSort(q.Zone)
		if q.Event != "" {
			continue
		}
		for _, r := range q.Starters {
			if k, ok := eventNPCs[strings.ToLower(ix.Npcs[r.ID].Name)]; ok && !r.Object {
				q.Event = k
				break
			}
		}
	}
}

// Seasonal is the player's setting: "" or "off" (the default), "on" for every event, or
// "on:midsummer,brewfest" for the listed ones.
type Seasonal string

// Wants reports whether the setting opts in to the event.
func (s Seasonal) Wants(key string) bool {
	v := string(s)
	if v == "on" {
		return true
	}
	list, ok := strings.CutPrefix(v, "on:")
	if !ok {
		return false
	}
	for _, k := range strings.Split(list, ",") {
		if k == key {
			return true
		}
	}
	return false
}

// With returns the setting after turning one event (key "" is every event) on or off. Turning
// everything off clears the scoped list; turning one event off while "on" covers all leaves the
// others on.
func (s Seasonal) With(key string, on bool) Seasonal {
	if key == "" {
		if on {
			return "on"
		}
		return ""
	}
	set := map[string]bool{}
	if s == "on" {
		for _, e := range Events {
			set[e.Key] = true
		}
	} else if list, ok := strings.CutPrefix(string(s), "on:"); ok {
		for _, k := range strings.Split(list, ",") {
			set[k] = true
		}
	}
	set[key] = on
	var keys []string
	for k, v := range set {
		if v {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return ""
	}
	return Seasonal("on:" + strings.Join(keys, ","))
}

// Describe is the setting in words.
func (s Seasonal) Describe() string {
	switch {
	case s == "on":
		return "on for every holiday event"
	case strings.HasPrefix(string(s), "on:"):
		var names []string
		for _, k := range strings.Split(string(s)[3:], ",") {
			if e, ok := EventByKey(k); ok {
				names = append(names, e.Name)
			}
		}
		return "on for " + strings.Join(names, ", ")
	}
	return "off"
}

// EventQuest is a holiday quest the character could take, with where it starts.
type EventQuest struct {
	ID    int
	Name  string
	Level int
	Start string // "Giver (Zone)"
}

// MaxEventQuests bounds the quests listed per event.
const MaxEventQuests = 12

// EventQuests lists an event's quests the character could pick up at their level (the setting is
// ignored: this is what opting in would add), nearest the level first.
func (ix *Index) EventQuests(key string, w Who) []EventQuest {
	if ix == nil {
		return nil
	}
	w.Seasonal = "on"
	w.Below, w.Above = 1<<30, 0
	var out []EventQuest
	for _, q := range ix.Quests {
		if q.Event == key && q.Eligible(w) {
			out = append(out, EventQuest{q.ID, q.Name, q.Level, ix.Describe(q.Starters)})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level > out[j].Level
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > MaxEventQuests {
		out = out[:MaxEventQuests]
	}
	return out
}

// eventQuestWords are fragments of holiday quest names (lower case) and their event's key, for
// quest rows that carry no sort id.
var eventQuestWords = map[string]string{
	"flickering flames": "midsummer", "midsummer": "midsummer", "honor the flame": "midsummer",
	"desecrate the flame": "midsummer", "hallow's end": "hallows-end", "winter veil": "winter-veil",
	"lunar festival": "lunar-festival", "love is in the air": "love-is-in-the-air",
	"noblegarden": "noblegarden", "children's week": "childrens-week",
	"harvest festival": "harvest-festival", "brewfest": "brewfest",
}

// EventOfName is the key of the event a quest of this name looks like, "" for an ordinary quest.
func EventOfName(name string) string {
	n := strings.ToLower(name)
	for w, key := range eventQuestWords {
		if strings.Contains(n, w) {
			return key
		}
	}
	return ""
}

// Offered reports whether a holiday quest of the event may be offered at t: the event's window
// holds t (the base rule) and the player has opted in to it.
func (s Seasonal) Offered(key string, t time.Time) bool {
	e, ok := EventByKey(key)
	return ok && e.Running(t) && s.Wants(key)
}
