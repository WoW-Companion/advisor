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
