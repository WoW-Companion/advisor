package digest

import (
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/questie"
)

func eventDigest(t *testing.T, when time.Time, setting string) Digest {
	t.Helper()
	ix := valanaar(false)
	delete(ix.Quests, 712) // leave room under the three pick-up cap
	ix.Quests[790] = &questie.Quest{ID: 790, Name: "Flickering Test Flames", Level: 10, Event: "midsummer", Starters: []questie.Ref{{ID: 4}}}
	c := scout()
	at := when.Unix()
	c.CapturedAt, c.Seasonal = &at, setting
	opt := synthOpt
	opt.Questie, opt.PrevLevel = ix, 8
	return Build(c, warriorStore(t), opt)
}

func allText(d Digest) string {
	var b strings.Builder
	for _, s := range d.Sections {
		for _, l := range s.Lines {
			b.WriteString(l.Text + "\n")
		}
	}
	return b.String()
}

func TestSeasonalDefaultOffShowsOnlyTheMention(t *testing.T) {
	june := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	text := allText(eventDigest(t, june, ""))
	if !strings.Contains(text, "Midsummer Fire Festival is on (until July 5). Ask me if you want its quests") {
		t.Errorf("no mention:\n%s", text)
	}
	if strings.Contains(text, "Flickering") {
		t.Errorf("an opted-out event quest is listed:\n%s", text)
	}
}

func TestSeasonalOnListsTheQuestAfterProgress(t *testing.T) {
	june := time.Date(2026, 6, 25, 12, 0, 0, 0, time.UTC)
	text := allText(eventDigest(t, june, "on"))
	if strings.Contains(text, "is on (until") {
		t.Errorf("mention shown although opted in:\n%s", text)
	}
	if !strings.Contains(text, "Flickering Test Flames") {
		t.Errorf("opted-in quest missing:\n%s", text)
	}
	if strings.Index(text, "Flickering") < strings.Index(text, "Wind Hollow Essence") {
		t.Errorf("event quest ranked before progress quests:\n%s", text)
	}
}

func TestSeasonalOnOutsideTheEventIsSilent(t *testing.T) {
	text := allText(eventDigest(t, time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC), "on"))
	if strings.Contains(text, "Flickering") || strings.Contains(text, "Midsummer") {
		t.Errorf("event content outside its window:\n%s", text)
	}
}
