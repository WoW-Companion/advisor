package ui

import (
	"os/exec"
	"strings"
	"testing"
)

// The Markdown renderer runs as real code under Node.
func TestMarkdownLogicUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if out, err := exec.Command(node, "markdown_logic_test.js").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// The page loads the renderer, offers the density setting and the answer bubbles use it, and the
// raw source is what Copy answer copies.
func TestMarkdownWiring(t *testing.T) {
	app, html, css := read(t, "app.js"), read(t, "index.html"), read(t, "style.css")
	for _, want := range []string{"WowcMd.render", "WowcMd.applyDensity", "cfg.density", "wowcSrc"} {
		if !strings.Contains(app, want) {
			t.Errorf("app.js lacks %s", want)
		}
	}
	for _, want := range []string{`src="markdown.js"`, `id="density"`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	for _, want := range []string{".md-compact", ".md-comfortable", ".md-h"} {
		if !strings.Contains(css, want) {
			t.Errorf("style.css lacks %s", want)
		}
	}
}
