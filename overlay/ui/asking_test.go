package ui

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The entry list, ask box, play-style card and chat wording run as real code under Node.
func TestAskingLogicUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if out, err := exec.Command(node, "asking_logic_test.js").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// The page wires them: chips send the ref, the box grows, a "reask" event follows the saved play
// style, and nothing the player reads tells them to /reload.
func TestAskingWiring(t *testing.T) {
	app, html := read(t, "app.js"), read(t, "index.html")
	for _, want := range []string{"WowcAsking.entryList", "body.ref = ref", "WowcAsking.fit", "WowcAsking.onAskKey", "'reask'", "saveStyle", "setStyle"} {
		if !strings.Contains(app, want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
	if !strings.Contains(html, `src="asking.js"`) {
		t.Error("the page does not load asking.js")
	}
	for _, f := range []string{"app.js", "asking.js", "index.html", "nextlist.js"} {
		src := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(read(t, f), "")
		for _, bad := range []string{"/reload", "Answers.lua", "I'm a level"} {
			for _, line := range strings.Split(src, "\n") {
				if strings.Contains(line, bad) && !strings.Contains(line, "shortQuestion") && !strings.HasPrefix(strings.TrimSpace(line), "//") {
					t.Errorf("%s shows %q: %s", f, bad, line)
				}
			}
		}
	}
}
