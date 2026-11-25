package questie

import (
	"testing"
	"time"
)

func day(m time.Month, d int) time.Time { return time.Date(2026, m, d, 12, 0, 0, 0, time.UTC) }

func TestEventWindows(t *testing.T) {
	mid, _ := EventByKey("midsummer")
	wv, _ := EventByKey("winter-veil")
	for _, tc := range []struct {
		e    Event
		t    time.Time
		want bool
	}{
		{mid, day(time.June, 25), true}, {mid, day(time.June, 21), true}, {mid, day(time.July, 5), true},
		{mid, day(time.November, 25), false}, {mid, day(time.July, 6), false},
		{wv, day(time.December, 31), true}, {wv, day(time.January, 1), true}, {wv, day(time.March, 1), false},
	} {
		if got := tc.e.Running(tc.t); got != tc.want {
			t.Errorf("%s on %v = %v", tc.e.Key, tc.t.Format("Jan 2"), got)
		}
	}
	if mid.Until() != "July 5" || mid.Mention() != "Midsummer Fire Festival is on (until July 5). Ask me if you want its quests" {
		t.Errorf("mention %q", mid.Mention())
	}
}

func TestSeasonalSetting(t *testing.T) {
	for _, tc := range []struct {
		s    Seasonal
		key  string
		want bool
	}{{"", "midsummer", false}, {"off", "midsummer", false}, {"on", "brewfest", true}, {"on:midsummer", "midsummer", true}, {"on:midsummer", "brewfest", false}} {
		if tc.s.Wants(tc.key) != tc.want {
			t.Errorf("%q wants %s = %v", tc.s, tc.key, !tc.want)
		}
	}
	s := Seasonal("").With("midsummer", true)
	if s != "on:midsummer" || s.With("midsummer", false) != "" || s.With("", false) != "" || s.With("", true) != "on" {
		t.Errorf("With: %q", s)
	}
	if got := Seasonal("on").With("brewfest", false); got.Wants("brewfest") || !got.Wants("midsummer") {
		t.Errorf("one event off from all: %q", got)
	}
	if e, ok := FindEvent("how can I take part in Hallow's End"); !ok || e.Key != "hallows-end" {
		t.Errorf("FindEvent %v %v", e, ok)
	}
}

func TestEventQuestOffer(t *testing.T) {
	q := &Quest{ID: 1, Name: "Flickering Flames", Level: 20, Event: "midsummer", Starters: []Ref{{ID: 1}}}
	w := Who{Level: 24, Done: map[int]bool{}, InLog: map[int]bool{}, Above: 2, Below: 10}
	for _, tc := range []struct {
		name string
		now  time.Time
		set  Seasonal
		lvl  int
		log  bool
		want bool
	}{
		{"off in event", day(time.June, 25), "", 24, false, false},
		{"on in event", day(time.June, 25), "on", 24, false, true},
		{"scoped in event", day(time.June, 25), "on:midsummer", 24, false, true},
		{"scoped to another", day(time.June, 25), "on:brewfest", 24, false, false},
		{"on outside event", day(time.November, 25), "on", 24, false, false},
		{"above the player", day(time.June, 25), "on", 19, false, false},
		{"no clock", time.Time{}, "on", 24, false, false},
	} {
		w := w
		w.Now, w.Seasonal, w.Level = tc.now, tc.set, tc.lvl
		if got := q.Eligible(w); got != tc.want {
			t.Errorf("%s: eligible = %v", tc.name, got)
		}
	}
}

func TestEventOffersRankLast(t *testing.T) {
	at := []Spawn{{Area: 1, X: 50, Y: 50}}
	ix := &Index{
		Quests: map[int]*Quest{
			1: {ID: 1, Name: "Fire", Level: 24, Event: "midsummer", Starters: []Ref{{ID: 1}}},
			2: {ID: 2, Name: "Boars", Level: 20, Starters: []Ref{{ID: 1}}},
		},
		Npcs:  map[int]Entity{1: {Name: "Giver", Spawns: at}},
		Areas: map[int]Area{1: {UIMap: 7, Zone: "Isle"}},
	}
	m := 7
	w := Who{Level: 24, Done: map[int]bool{}, InLog: map[int]bool{}, Above: 2, Below: 10, Now: day(time.June, 25), Seasonal: "on"}
	got := ix.Available(w, Where{MapID: &m}, nil)
	if len(got) != 2 || got[0].Quest.ID != 2 || got[1].Quest.ID != 1 || !got[1].Event {
		t.Errorf("event quest must rank after the progress quest: %+v", got)
	}
	if q := ix.EventQuests("midsummer", w); len(q) != 1 || q[0].Start != "Giver (Isle)" {
		t.Errorf("event quests %+v", q)
	}
}

func TestEventOfNameOffered(t *testing.T) {
	jun := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	nov := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	if k := EventOfName("Flickering Flames in Kalimdor"); k != "midsummer" {
		t.Fatalf("EventOfName = %q", k)
	}
	if EventOfName("Plain Errand") != "" {
		t.Error("ordinary quest tagged")
	}
	for _, c := range []struct {
		s    Seasonal
		t    time.Time
		want bool
	}{{"on", jun, true}, {"on", nov, false}, {"", jun, false}} {
		if got := c.s.Offered("midsummer", c.t); got != c.want {
			t.Errorf("Offered(%q, %v) = %v", c.s, c.t, got)
		}
	}
}
