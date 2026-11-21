package gamesetup

import (
	"context"
	"strings"

	"github.com/WoW-Companion/advisor/internal/ask"
)

// AIPrompt is the tiny prompt the Test button sends.
const AIPrompt = "Reply with the single word OK."

// AIInstallNote is shown when no AI client is found.
const AIInstallNote = "Install Claude Code, Codex or Antigravity CLI (agy), then sign in once by running it in a terminal."

// AIItem is the AI client row; name is "" when none is detected.
func AIItem(name string) Item {
	it := Item{ID: IDAI, Title: "AI client"}
	if name == "" {
		it.Status, it.Detail = Bad, "None found. "+AIInstallNote
		return it
	}
	it.Status, it.Detail = OK, name+" detected."
	it.Action, it.Label = ActTestAI, "Test"
	return it
}

// TestAI sends one tiny prompt through command; ok says it answered, msg the answer or the
// error's first line.
func TestAI(ctx context.Context, command []string) (ok bool, msg string) {
	line, err := ask.Probe(ctx, command, AIPrompt)
	if err != nil {
		return false, err.Error()
	}
	if strings.Contains(strings.ToUpper(line), "OK") {
		return true, "OK"
	}
	return true, "OK (answered: " + line + ")"
}
