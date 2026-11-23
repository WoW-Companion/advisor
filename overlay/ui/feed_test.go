package ui

import (
	"strings"
	"testing"
)

// The overlay no longer shows or answers questions asked in game: no pending card, no game mark,
// no answer-pending call.
func TestNoInGameAskPath(t *testing.T) {
	for _, f := range []string{"app.js", "launch.js", "asking.js"} {
		js := read(t, f)
		for _, bad := range []string{"answer-pending", "pendingCard", "gameMark", "You asked in game", "gameToast", "Question from game"} {
			if strings.Contains(js, bad) {
				t.Errorf("%s still has %s", f, bad)
			}
		}
	}
}
