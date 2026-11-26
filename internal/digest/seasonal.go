package digest

import (
	"time"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
)

// ClockOf is the player's clock: the capture's time, else fallback.
func ClockOf(c character.Character, fallback time.Time) time.Time {
	if c.CapturedAt != nil && *c.CapturedAt > 0 {
		return time.Unix(*c.CapturedAt, 0)
	}
	return fallback
}

// seasonalMention is the one line shown while a holiday event runs and the player has not opted in
// to it: no steps, only the offer to ask.
func seasonalMention(c character.Character, opt Options) (Line, bool) {
	now := ClockOf(c, opt.Now)
	if now.IsZero() {
		return Line{}, false
	}
	set := questie.Seasonal(c.Seasonal)
	for _, e := range questie.RunningEvents(now) {
		if set.Wants(e.Key) {
			continue
		}
		q := "How can I take part in " + e.Name + "?"
		return Line{Text: e.Mention(), Kind: KindNote, Asks: asksOf(q, q), Ref: noteRef("event-" + e.Key)}, true
	}
	return Line{}, false
}
