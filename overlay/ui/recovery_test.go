package ui

import (
	"strings"
	"testing"
)

// The recovery window is a page of its own: it must not load the main UI's scripts or styles, so it
// still works when those are broken, and the shell wires it to the tray and the crash check.
func TestRecoveryWindowIsStandalone(t *testing.T) {
	html, js := read(t, "recovery.html"), read(t, "recovery.js")
	for _, bad := range []string{"app.js", "updates.js", "style.css", "version.js", "launch.js"} {
		if strings.Contains(html, bad) || strings.Contains(js, bad) {
			t.Errorf("the recovery window must not depend on %s", bad)
		}
	}
	if strings.Contains(html, "<script>") {
		t.Error("an inline script would be refused by the page's content security policy")
	}
	if !strings.Contains(html, `src="recovery.js"`) {
		t.Error("recovery.html must load recovery.js")
	}
	if !strings.Contains(js, "recovery_state") || !strings.Contains(js, "recovery_do") {
		t.Error("recovery.js must use the shell's recovery commands")
	}
}

func TestTrayAndCrashCheckAreWired(t *testing.T) {
	rs := read(t, "../src-tauri/src/main.rs")
	for _, want := range []string{`"Check for updates"`, `"Open logs folder"`, `"check" => recover::tray_check`, `"goback" => recover::tray_go_back`, `"logs" => recover::tray_logs`,
		"recover::recovery_state", "recover::recovery_do", "guard::on_start", "guard::on_clean_exit", "report_health"} {
		if !strings.Contains(rs, want) {
			t.Errorf("main.rs lacks %q", want)
		}
	}
	if !strings.Contains(read(t, "../src-tauri/capabilities/recovery.json"), `"recovery"`) {
		t.Error("the recovery window needs its capability")
	}
	if !strings.Contains(read(t, "app.js"), "schema: 1") {
		t.Error("the overlay's saved settings must carry a schema number")
	}
}
