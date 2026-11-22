package ui

import (
	"strings"
	"testing"
)

// The Ask feed paints a question asked in game at once, marked as from the game, and the answer as
// it streams; the "Question from game" toast is off unless the setting is on.
func TestAskFeedShowsInGameAsks(t *testing.T) {
	js := read(t, "app.js")
	for _, want := range []string{"'asked in game'", "e.status === 'answering'", "cfg.gameToast === true", "gameToast: false", "Question from game", "e.source === 'game'"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
	if !strings.Contains(js, "events.addEventListener('history'") {
		t.Error("the feed does not follow history events")
	}
}
