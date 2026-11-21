// Package ui checks the overlay's static UI files: the opacity slider may only fade the panel
// background, never text, controls or the border.
package ui

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var ruleRE = regexp.MustCompile(`([^{}@]+)\{([^{}]*)\}`)

// Every declaration that reads --alpha must be a background; --alpha must never reach text, borders
// or whole-element opacity.
func TestAlphaFeedsBackgroundsOnly(t *testing.T) {
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(read(t, "style.css"), "")
	used := 0
	for _, m := range ruleRE.FindAllStringSubmatch(css, -1) {
		sel := strings.TrimSpace(m[1])
		for _, decl := range strings.Split(m[2], ";") {
			prop, val, ok := strings.Cut(decl, ":")
			if !ok || !strings.Contains(val, "--alpha") {
				continue
			}
			used++
			if p := strings.TrimSpace(prop); p != "background" && p != "background-color" && p != "transition" {
				t.Errorf("%s: --alpha drives %q, want a background", sel, p)
			}
			if regexp.MustCompile(`button|input|select|textarea|h1|\.tag|\.key|#text|#send|#who`).MatchString(sel) {
				t.Errorf("%s: --alpha must not touch text or controls", sel)
			}
		}
	}
	if used < 4 {
		t.Errorf("only %d background declarations use --alpha; panel and section backgrounds should", used)
	}
}

func TestNoWholeElementOpacity(t *testing.T) {
	css := read(t, "style.css")
	for _, sel := range []string{"html", "body", ".panel"} {
		for _, m := range ruleRE.FindAllStringSubmatch(css, -1) {
			if strings.TrimSpace(m[1]) == sel && regexp.MustCompile(`(^|[;\s])opacity\s*:`).MatchString(m[2]) {
				t.Errorf("%s sets opacity; only the background may fade", sel)
			}
		}
	}
	js := read(t, "app.js")
	if regexp.MustCompile(`style\.opacity|setProperty\('opacity'|setOpacity\(.*(win|window)\.`).MatchString(js) {
		t.Error("app.js sets element or window opacity")
	}
}

func TestSliderReachesZeroAndMapsToAlpha(t *testing.T) {
	html, js := read(t, "index.html"), read(t, "app.js")
	if !regexp.MustCompile(`id="opacity"[^>]*min="0"[^>]*max="100"`).MatchString(html) {
		t.Error("opacity slider must span 0 to 100")
	}
	if n := strings.Count(js, "setProperty('--alpha'"); n != 1 {
		t.Errorf("--alpha set in %d places, want one (paint)", n)
	}
	if !strings.Contains(js, "opacity: 92,") || !strings.Contains(js, "solidHover: false") {
		t.Error("defaults must be opacity 92 and solid-on-hover off")
	}
}

func TestControlsKeepOwnBackground(t *testing.T) {
	css := read(t, "style.css")
	if !strings.Contains(css, "rgba(var(--slate), .65)") {
		t.Error("controls need a fixed minimum background")
	}
}

// Author rules such as .banner { display: flex } outrank the user-agent [hidden] rule, so a hidden
// element would still show. One global rule must win over all of them. The rendered check (computed
// display per UI state, in headless Chrome) is tools/status-check/run.sh.
func TestHiddenAlwaysWins(t *testing.T) {
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(read(t, "style.css"), "")
	if !regexp.MustCompile(`(?m)^\[hidden\]\s*\{\s*display:\s*none\s*!important;?\s*\}`).MatchString(css) {
		t.Error("style.css needs a global [hidden] { display: none !important; }")
	}
	for _, id := range []string{"banner", "status", "aibar", "notice", "settings", "agent-cmd-row", "ct"} {
		if !regexp.MustCompile(`id="` + id + `"[^>]*\shidden`).MatchString(read(t, "index.html")) {
			t.Errorf("#%s must start hidden", id)
		}
	}
}
