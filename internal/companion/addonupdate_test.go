package companion

import (
	"encoding/json"
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
