package ui

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The saved chat, old-ask card, suggestions, quick bar and dot run as real code under Node.
func TestLaunchLogicUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if out, err := exec.Command(node, "launch_logic_test.js").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// funcBody is the source of the named top-level function in js, up to its closing brace at column 0.
func funcBody(t *testing.T, js, name string) string {
	t.Helper()
	i := strings.Index(js, "function "+name+"(")
	if i < 0 {
		t.Fatalf("app.js has no %s", name)
	}
	j := strings.Index(js[i:], "\n}\n")
	if j < 0 {
		t.Fatalf("%s does not end", name)
	}
	return js[i : i+j]
}

// A launch opens on the saved chat and sends nothing: restoring and rendering never call the ask
// endpoints; only the Answer now button reaches the pending one.
func TestLaunchSendsNothing(t *testing.T) {
	js := read(t, "app.js")
	for _, name := range []string{"restoreChat", "render", "showPending", "load"} {
		body := funcBody(t, js, name)
		for _, bad := range []string{"/api/ask", "/api/answer-pending", "ask(", "askEntry(", "answerPending("} {
			if strings.Contains(strings.ReplaceAll(body, "function showPending(", ""), bad) && !(name == "showPending" && bad == "answerPending(") {
				t.Errorf("%s calls %s: a launch must send nothing", name, bad)
			}
		}
	}
	if !strings.Contains(funcBody(t, js, "load"), "restoreChat()") {
		t.Error("load does not restore the saved chat")
	}
	if strings.Count(js, "/api/answer-pending") != 1 {
		t.Error("the pending endpoint must be reached from one place, the Answer now button")
	}
}

// Settings stay behind the gear: hidden in the page and forced closed at launch, never remembered.
func TestSettingsHiddenAtLaunch(t *testing.T) {
	js, html := read(t, "app.js"), read(t, "index.html")
	if !regexp.MustCompile(`<section id="settings" class="settings" hidden>`).MatchString(html) {
		t.Error("the settings section is not hidden in the page")
	}
	if !strings.Contains(js, "cfg.settingsOpen = false;") {
		t.Error("a launch must start with settings closed")
	}
	if !strings.Contains(js, "$('settings-btn').onclick") {
		t.Error("the gear does not toggle settings")
	}
	settings := html[strings.Index(html, `id="settings"`):strings.Index(html, `<div class="bar">`)]
	if !strings.Contains(settings, `id="version"`) {
		t.Error("the version line belongs inside the settings view")
	}
}

// The page wires the suggestions row, the quick bar (hideable under Shortcuts), why lines and the dot.
func TestLaunchWiring(t *testing.T) {
	js, html, css := read(t, "app.js"), read(t, "index.html"), read(t, "style.css")
	for _, want := range []string{"WowcLaunch.visibleSuggestions", "WowcLaunch.suggestionRow", "WowcLaunch.quickBar", "WowcLaunch.quickAction", "/api/refresh-plan", "WowcLaunch.freshness", "WowcLaunch.chatStore", "chats.clear(", "WowcLaunch.planLine"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
	for _, want := range []string{`src="launch.js"`, `id="suggs"`, `id="quick-slot"`, `id="quick-on"`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	if strings.Index(html, `id="quick-slot"`) > strings.Index(html, `id="form"`) || strings.Index(html, `id="log"`) > strings.Index(html, `id="quick-slot"`) {
		t.Error("the quick bar sits between the chat and the ask box")
	}
	// The refresh path makes no AI call.
	if body := funcBody(t, js, "refreshPlan"); strings.Contains(body, "/api/ask") {
		t.Error("Refresh must not ask the AI")
	}
	for _, want := range []string{".dot.amber", ".dot.red", ".entry-why", ".gap", ".pending", ".quick"} {
		if !strings.Contains(css, want) {
			t.Errorf("style.css lacks %s", want)
		}
	}
}

// Nothing the player reads says "map units".
func TestNoMapUnitsInPlayerText(t *testing.T) {
	for _, f := range []string{"app.js", "asking.js", "nextlist.js", "index.html"} {
		src := regexp.MustCompile(`(?s)/\*.*?\*/|<!--.*?-->`).ReplaceAllString(read(t, f), "")
		for _, line := range strings.Split(src, "\n") {
			if strings.Contains(line, "map units") && !strings.HasPrefix(strings.TrimSpace(line), "//") {
				t.Errorf("%s shows map units: %s", f, line)
			}
		}
	}
	for _, line := range strings.Split(read(t, "launch.js"), "\n") {
		if strings.Contains(line, "map units") && !strings.Contains(line, "planLine") && !strings.HasPrefix(strings.TrimSpace(line), "//") && !strings.Contains(line, "/map units/") {
			t.Errorf("launch.js shows map units: %s", line)
		}
	}
}
