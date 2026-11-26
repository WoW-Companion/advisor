package companion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestAddonUpdateEndpoint(t *testing.T) {
	game := t.TempDir()
	toc := filepath.Join(game, "Interface", "AddOns", "WoWCompanion", "WoWCompanion.toc")
	os.MkdirAll(filepath.Dir(toc), 0o755)
	os.WriteFile(toc, []byte("## Version: 1.2.0\n"), 0o644)
	backups := t.TempDir()
	os.MkdirAll(filepath.Join(backups, "1.1.0"), 0o755)
	s := New(Config{GameDir: game, AddonAPI: "http://127.0.0.1:1/x", AddonBackups: backups, App: true})
	s.port = "4317"
	get := func(cfgGame string) map[string]any {
		s.cfg.GameDir = cfgGame
		r := httptest.NewRequest("GET", "http://127.0.0.1:4317/api/addon/update", nil)
		r.Header.Set(TokenHeader, s.Token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		var m map[string]any
		json.Unmarshal(w.Body.Bytes(), &m)
		return m
	}
	m := get(game)
	if m["installed"] != "1.2.0" || m["available"] != false {
		t.Errorf("status: %v", m)
	}
	if b, _ := m["back"].([]any); len(b) != 1 || b[0] != "1.1.0" {
		t.Errorf("kept versions: %v", m["back"])
	}
	if m := get(filepath.Join(game, "nope")); m["message"] == nil || m["installed"] != nil {
		t.Errorf("missing folder: %v", m)
	}
}

func TestNotesSincePerVersionAndOffline(t *testing.T) {
	md := "## 1.3.0\n\n- **Added** c.\n\n## 1.2.0\n\n- **Fixed** b.\n\n## 1.1.0\n\n- **Fixed** a.\n"
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(md)) }))
	s := New(Config{AddonNotes: ts.URL + "/v%s/CHANGELOG.md", AddonBackups: filepath.Join(t.TempDir(), "addon-backups"), App: true})
	s.port = "4317"
	get := func() map[string]notesPart {
		r := httptest.NewRequest("GET", "http://127.0.0.1:4317/api/notes-since?app_from=0.7.92&app=0.7.94&addon_from=1.1.0&addon=1.3.0", nil)
		r.Header.Set(TokenHeader, s.Token)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		var m map[string]notesPart
		json.Unmarshal(w.Body.Bytes(), &m)
		return m
	}
	check := func(when string) {
		m := get()
		if a := m["addon"].Entries; len(a) != 2 || a[0].Version != "1.3.0" || a[1].Version != "1.2.0" || a[1].Lines[0] != "Fixed b." {
			t.Errorf("%s addon entries: %+v", when, a)
		}
		if a := m["app"].Entries; len(a) != 2 || a[0].Version != "0.7.94" {
			t.Errorf("%s app entries: %+v", when, a)
		}
	}
	check("online")
	ts.Close()
	check("offline")
}
