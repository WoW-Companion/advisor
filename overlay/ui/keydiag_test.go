package ui

import (
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// The keyboard test records and caps its lines, under Node.
func TestKeyDiagLogicUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if out, err := exec.Command(node, "keydiag_logic_test.js").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// The page loads the test, shows it under Updates, and asks Rust for the native line.
func TestKeyDiagWiring(t *testing.T) {
	app, html := read(t, "app.js"), read(t, "index.html")
	for _, want := range []string{"WowcKeyDiag", "key_native_state", "ask: focus", "ask: blur"} {
		if !strings.Contains(app, want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
	for _, want := range []string{`src="keydiag.js"`, `id="key-test"`, "Keyboard test"} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
}

// The ask box no longer steals focus, has the editing-key safety net, and shortcut boxes reset on Backspace.
func TestAskFocusAndShortcutKeys(t *testing.T) {
	app, html := read(t, "app.js"), read(t, "index.html")
	for _, want := range []string{
		"WowcAsking.wireEditKeys(text, fitBox)",
		"WowcKeyDiag.attach(text, kl, 'ask')",
		"ev.key === 'Tab'",
		"DEFAULTS[field]",
	} {
		if !strings.Contains(app, want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
	asking := read(t, "asking.js")
	for name, src := range map[string]string{"app.js": app, "asking.js": asking} {
		if strings.Contains(src, "focus_window") {
			t.Errorf("%s must not invoke focus_window: native focus calls break the ask box", name)
		}
	}
	if !strings.Contains(html, "Press a key combo · Backspace resets.") {
		t.Error("index.html lacks the shortcut hint")
	}
}

// An on... property handler that returns false cancels the key, so app.js must not assign one that can.
func TestNoCancellingKeyHandlers(t *testing.T) {
	app := read(t, "app.js")
	if strings.Contains(app, "onkeydown = ev => WowcAsking.onAskKey") {
		t.Error("app.js assigns onAskKey's result to onkeydown, which cancels every key")
	}
	if !strings.Contains(app, "text.addEventListener('keydown', ev => { WowcAsking.onAskKey(") {
		t.Error("the ask box key handler must be an addEventListener with a block body")
	}
	// Any on<event> property assigned an expression-bodied arrow whose value is a handler's result.
	re := regexp.MustCompile(`\.on(keydown|keyup|keypress|beforeinput|paste|input)\s*=\s*(\([^)]*\)|\w+)\s*=>\s*[^{\s]`)
	for _, f := range []string{"app.js", "asking.js", "launch.js", "version.js"} {
		if m := re.FindString(read(t, f)); m != "" {
			t.Errorf("%s: %q returns its expression's value; use a block body", f, m)
		}
	}
}
