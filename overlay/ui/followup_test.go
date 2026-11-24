package ui

import (
	"os/exec"
	"strings"
	"testing"
)

// The follow-up chip and the end-of-answer scroll run as real code under Node.
func TestFollowupLogicUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if out, err := exec.Command(node, "followup_logic_test.js").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// The page wires them: the chip above the ask box, the pill under the log, and Clear and a finished answer use them.
func TestFollowupWiring(t *testing.T) {
	app, html := read(t, "app.js"), read(t, "index.html")
	for _, want := range []string{"WowcFollow.show", "WowcFollow.answerDone", "WowcFollow.onScroll", "/api/context/clear", "state.context"} {
		if !strings.Contains(app, want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
	for _, want := range []string{`src="followup.js"`, `id="follow"`, `id="new-answer"`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	// The scroll comes after the usage and notice lines are appended.
	i, j := strings.Index(app, "a.appendChild(s); }"), strings.Index(app, "WowcFollow.answerDone")
	if i < 0 || j < i {
		t.Error("answerDone must run after the usage and notice lines are appended")
	}
}
