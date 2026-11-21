package ui

import (
	"strings"
	"testing"
)

// The settings panel carries the version line and the opt-out, and the page may open the release
// page only through the shell's fixed list; the installer upgrades in place via the patched template.
func TestVersionLineAndInstallerWiring(t *testing.T) {
	html, js := read(t, "index.html"), read(t, "app.js")
	for _, want := range []string{`id="version"`, `id="check-updates"`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	for _, want := range []string{"checkUpdates: true", "/api/update", " advisor ", "12 * 60 * 60 * 1000", "open_install_page"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	rs := read(t, "../src-tauri/src/main.rs")
	if !strings.Contains(rs, `"https://github.com/WoW-Companion/advisor/releases/latest"`) || !strings.Contains(rs, `TrayIconBuilder::with_id("main")`) {
		t.Error("main.rs: the release page must be on the opener's list and the tray must be findable to retitle")
	}
	conf := read(t, "../src-tauri/tauri.conf.json")
	if !strings.Contains(conf, `"template": "windows/installer.nsi"`) {
		t.Error("tauri.conf.json does not use the in-place-upgrade installer template")
	}
	nsi := read(t, "../src-tauri/windows/installer.nsi")
	if !strings.Contains(nsi, "upgrades in place") || !strings.Contains(nsi, "{{compression}}") {
		t.Error("installer.nsi lost the in-place upgrade change or is not a Tauri template")
	}
}

// The model and reasoning dropdowns sit under the AI client row, and the choice is kept per client.
func TestModelAndReasoningDropdowns(t *testing.T) {
	html, js := read(t, "index.html"), read(t, "app.js")
	agent, model, reasoning := strings.Index(html, `id="agent-cmd-row"`), strings.Index(html, `id="model"`), strings.Index(html, `id="reasoning"`)
	if agent < 0 || model < agent || reasoning < model || strings.Index(html, `id="length"`) < reasoning {
		t.Error("the model and reasoning dropdowns must follow the AI client row, side by side")
	}
	for _, want := range []string{"byClient", "cfg.byClient[clientKey()]", "model: 'efficient'", "reasoning: 'low'", "reasoning: c.reasoning", "sel.disabled = !items.length"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
}
