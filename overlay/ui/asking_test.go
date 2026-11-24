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
	for _, want := range []string{"WowcAsking.entryList", "body.ref = ref", "WowcAsking.fit", "WowcAsking.onAskKey", "'reask'", "saveStyle", "setStyle", "WowcAsking.workingLine", "WowcAsking.specCard", "addEventListener('profile'"} {
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

// Nothing in the page blocks a paste into the ask box: the only paste handler is the one that
// falls back to the native clipboard, the only keydown handler on the box is Enter-to-send (which
// ignores Ctrl+V and Shift+Insert), and no page-wide handler cancels keys or paste events.
func TestAskBoxPasteNotBlocked(t *testing.T) {
	app, html := read(t, "app.js"), read(t, "index.html")
	for _, bad := range []string{"onpaste", "oncut"} {
		if strings.Contains(app, bad) || strings.Contains(html, bad) || strings.Contains(read(t, "style.css"), bad) {
			t.Errorf("the page contains %q", bad)
		}
	}
	if strings.Contains(app, "addEventListener('paste'") || strings.Contains(app, `addEventListener("paste"`) {
		t.Error("app.js has its own paste handler")
	}
	// Every preventDefault in app.js sits in a link, grip, form or shortcut-capture handler.
	for _, line := range strings.Split(app, "\n") {
		if strings.Contains(line, "preventDefault") && !strings.Contains(line, "a.onclick") && !strings.Contains(line, "grip") && !strings.Contains(line, "onsubmit") && strings.TrimSpace(line) != "ev.preventDefault();" {
			t.Errorf("unexpected preventDefault: %s", line)
		}
	}
	if !strings.Contains(app, "WowcAsking.wirePaste(text") || !strings.Contains(app, "read_clipboard") || !strings.Contains(html, `id="paste"`) || !strings.Contains(html, "Paste from clipboard") {
		t.Error("the native paste fallback or the paste button is not wired")
	}
}
