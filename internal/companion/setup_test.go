package companion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/dataset"
)

func TestSetupEndpoints(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "_forever_", "Interface", "AddOns"), 0o755)
	// A release lookup that is refused (not github.com) keeps the test off the network.
	s := New(Config{Cache: dataset.Store{Root: t.TempDir()}, AddonAPI: "http://127.0.0.1:1/x", Ask: ask.Config{Name: "claude", Command: []string{"/nonexistent-agent"}}, App: true})
	s.port = "4317"
	h := s.Handler()
	do := func(method, path, body string) map[string]any {
		r := httptest.NewRequest(method, "http://127.0.0.1:4317"+path, strings.NewReader(body))
		r.Header.Set(TokenHeader, s.Token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
		}
		var m map[string]any
		json.Unmarshal(w.Body.Bytes(), &m)
		return m
	}
	m := do("GET", "/api/setup?game="+root, "")
	items, _ := m["items"].([]any)
	if len(items) != 7 {
		t.Fatalf("want 6 rows, got %v", m)
	}
	res := do("POST", "/api/setup/install-addon", `{"game":"`+root+`"}`)
	if res["error"] == nil || res["ok"] != nil {
		t.Errorf("install with no reachable release should report an error: %v", res)
	}
	if res := do("POST", "/api/setup/test-ai", ""); res["ok"] != false {
		t.Errorf("test-ai with a missing agent: %v", res)
	}
}
