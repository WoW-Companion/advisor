package ui

import (
	"os/exec"
	"strings"
	"testing"
)

// The banner, the settings switches and the version list run as real code under Node: the banner
// states, Later and Skip, the pulled-version banner, the notes text and the check schedule.
func TestUpdatesLogicUnderNode(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed")
	}
	if out, err := exec.Command(node, "updates_logic_test.js").CombinedOutput(); err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
}

// The banner sits in the chat's flow above the log (never over the input) and has a solid background;
// the settings carry both switches, Check now and the version list; the install is the shell's command.
func TestUpdateBannerAndSettingsWiring(t *testing.T) {
	html, js, css, rs, conf := read(t, "index.html"), read(t, "app.js"), read(t, "style.css"), read(t, "../src-tauri/src/main.rs"), read(t, "../src-tauri/tauri.conf.json")
	bar, log, form := strings.Index(html, `id="update-bar"`), strings.Index(html, `id="log"`), strings.Index(html, `id="form"`)
	if bar < 0 || bar > log || log > form {
		t.Error("the update banner must come right before the chat log and the input after it")
	}
	for _, want := range []string{`id="check-updates"`, `id="update-now"`, `id="check-now"`, `id="version-list"`, `Get updates right away`, `src="updates.js"`} {
		if !strings.Contains(html, want) {
			t.Errorf("index.html lacks %s", want)
		}
	}
	for _, want := range []string{"updateNow: false", "invoke('install_update'", "Install this version", "WowcUpdates.updateQuery", "schedule.focus()", "update-progress"} {
		if !strings.Contains(js, want) {
			t.Errorf("app.js lacks %q", want)
		}
	}
	i := strings.Index(css, ".update-bar {")
	if i < 0 || strings.Contains(css[i:i+strings.Index(css[i:], "}")], "position") || strings.Contains(css[i:i+strings.Index(css[i:], "}")], "--alpha") {
		t.Error(".update-bar must stay in the flow with a solid background")
	}
	if !strings.Contains(rs, "install_update,") || !strings.Contains(rs, "mod update;") {
		t.Error("main.rs does not register install_update")
	}
	if !strings.Contains(conf, `"allowDowngrades": true`) {
		t.Error("tauri.conf.json must allow the installer to downgrade, or going back is refused")
	}
	// The page names only a version; the address is built in the shell from fixed parts.
	up := read(t, "../src-tauri/src/update.rs")
	if !strings.Contains(up, `const DOWNLOAD_BASE: &str = "https://github.com/WoW-Companion/advisor/releases/download"`) || strings.Contains(js, "releases/download") {
		t.Error("the download address belongs to the shell only")
	}
}
