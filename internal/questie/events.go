package questie

import (
	"strings"
	"time"
)

// Event is a seasonal holiday and the typical dates it runs each year. The windows are the usual
// public calendar (the in-game calendar and the wowhead / wowpedia event pages); the game moves
// some of them a few days from year to year, so each window is the usual span, not a promise.
type Event struct {
	Name                 string
	StartMonth, StartDay int
	EndMonth, EndDay     int // inclusive; an end before the start runs over the new year
}

// Events are the holidays whose quests are never offered outside their window.
var Events = []Event{
	{"Midsummer Fire Festival", 6, 21, 7, 5},
	{"Hallow's End", 10, 18, 11, 1},
	{"Feast of Winter Veil", 12, 15, 1, 2},
	{"Lunar Festival", 1, 25, 2, 15},
	{"Love is in the Air", 2, 6, 2, 20},
	{"Noblegarden", 4, 1, 4, 14},
	{"Children's Week", 5, 1, 5, 8},
	{"Harvest Festival", 9, 20, 9, 27},
	{"Brewfest", 9, 20, 10, 6},
}

// Open reports whether t falls inside the event's window.
func (e Event) Open(t time.Time) bool {
	md := int(t.Month())*100 + t.Day()
	s, f := e.StartMonth*100+e.StartDay, e.EndMonth*100+e.EndDay
	if s <= f {
		return md >= s && md <= f
	}
	return md >= s || md <= f
}

func eventNamed(name string) *Event {
	for i := range Events {
		if Events[i].Name == name {
			return &Events[i]
		}
	}
	return nil
}

// eventGivers are NPCs that stand only while their event runs (lower case), and the event.
var eventGivers = map[string]string{
	"festival talespinner":      "Midsummer Fire Festival",
	"festival flamekeeper":      "Midsummer Fire Festival",
	"festival loremaster":       "Midsummer Fire Festival",
	"great-father winter":       "Feast of Winter Veil",
	"greatmother frostfire":     "Feast of Winter Veil",
	"orphan matron nightingale": "Children's Week",
	"orphan matron battlewail":  "Children's Week",
	"candy bucket":              "Hallow's End",
	"lunar festival harbinger":  "Lunar Festival",
}

// eventQuestWords are fragments of event quest names (lower case), and the event.
var eventQuestWords = map[string]string{
	"flickering flames":   "Midsummer Fire Festival",
	"midsummer":           "Midsummer Fire Festival",
	"honor the flame":     "Midsummer Fire Festival",
	"desecrate the flame": "Midsummer Fire Festival",
	"hallow's end":        "Hallow's End",
	"winter veil":         "Feast of Winter Veil",
	"lunar festival":      "Lunar Festival",
	"love is in the air":  "Love is in the Air",
	"noblegarden":         "Noblegarden",
	"children's week":     "Children's Week",
	"harvest festival":    "Harvest Festival",
	"brewfest":            "Brewfest",
}

// EventOfName is the event whose quest this name looks like, "" for an ordinary quest.
func EventOfName(name string) string {
	n := strings.ToLower(name)
	for w, ev := range eventQuestWords {
		if strings.Contains(n, w) {
			return ev
		}
	}
	return ""
}

// tagEvents marks the holiday quests: those named for an event, and those whose every giver is an
// event-only NPC.
func (ix *Index) tagEvents() {
	for _, q := range ix.Quests {
		q.Event = EventOfName(q.Name)
		if q.Event != "" || len(q.Starters) == 0 {
			continue
		}
		for _, r := range q.Starters {
			ev := eventGivers[strings.ToLower(ix.entityName(r))]
			if ev == "" {
				q.Event = ""
				break
			}
			q.Event = ev
		}
	}
}

func (ix *Index) entityName(r Ref) string {
	e, _ := ix.entity(r)
	return e.Name
}

// EventClosed reports whether the quest is a holiday quest whose event is not running at t. A zero
// t is unknown and never closes anything.
func (q *Quest) EventClosed(t time.Time) bool {
	if q.Event == "" || t.IsZero() {
		return false
	}
	e := eventNamed(q.Event)
	return e != nil && !e.Open(t)
}
