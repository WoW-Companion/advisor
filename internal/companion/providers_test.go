package companion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/provider"
)

const testKey = "sk-live-NEVER-SHOW-ME"

type memKeys map[string]string

func (m memKeys) Get(p string) (string, error) {
	if k, ok := m[p]; ok {
		return k, nil
	}
	return "", provider.ErrNoKey
}
func (m memKeys) Set(p, k string) error { m[p] = k; return nil }
func (m memKeys) Delete(p string) error { delete(m, p); return nil }

func TestProviderAPI(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	var gotAuth, gotBody string
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/models"):
			fmt.Fprint(w, `{"data":[{"id":"llama3"},{"id":"qwen"}]}`)
		default:
			gotAuth = r.Header.Get("Authorization")
			b, _ := io.ReadAll(r.Body)
			gotBody = string(b)
			fmt.Fprint(w, `{"choices":[{"message":{"content":"OK"}}]}`)
		}
	}))
	defer fake.Close()

	e := start(t, "text", false)
	keys := memKeys{}
	e.srv.cfg.Keys = keys
	e.srv.cfg.Providers = filepath.Join(e.dir, "providers.json")
	var bodies []string
	call := func(method, path, body string) (int, string) {
		resp := e.do(t, method, path, body, nil)
		b, _ := io.ReadAll(resp.Body)
		bodies = append(bodies, string(b))
		return resp.StatusCode, string(b)
	}

	if code, b := call("GET", "/api/providers", ""); code != 200 || !strings.Contains(b, `"id":"ollama"`) || !strings.Contains(b, `"claude-haiku-5-5"`) {
		t.Fatalf("list: %d %s", code, b)
	}
	if code, _ := call("POST", "/api/providers/key", `{"provider":"nope","key":"x"}`); code != 400 {
		t.Errorf("unknown provider: %d", code)
	}
	if code, _ := call("POST", "/api/providers/key", `{"provider":"anthropic","key":"a\nb"}`); code != 400 {
		t.Errorf("multi-line key: %d", code)
	}
	if code, b := call("POST", "/api/providers/key", `{"provider":"custom","key":"`+testKey+`"}`); code != 200 || !strings.Contains(b, `"has_key":true`) {
		t.Fatalf("set key: %d %s", code, b)
	}
	if keys["custom"] != testKey {
		t.Fatal("key not stored in the key store")
	}
	if code, b := call("POST", "/api/providers/active", `{"provider":"custom","base_url":"`+fake.URL+`/v1","model":"llama3"}`); code != 200 || !strings.Contains(b, `"has_key":true`) {
		t.Fatalf("active: %d %s", code, b)
	}
	if code, _ := call("POST", "/api/providers/active", `{"provider":"custom","base_url":"ftp://x"}`); code != 400 {
		t.Errorf("bad base url: %d", code)
	}
	if code, b := call("GET", "/api/providers/models?provider=custom&base_url="+fake.URL+"/v1", ""); code != 200 || !strings.Contains(b, `"qwen"`) {
		t.Errorf("models: %d %s", code, b)
	}
	if code, b := call("POST", "/api/providers/test", `{}`); code != 200 || !strings.Contains(b, `"ok":true`) {
		t.Errorf("test: %d %s", code, b)
	}
	if gotAuth != "Bearer "+testKey || strings.Contains(gotBody, `"tools"`) {
		t.Errorf("test request: auth ok: %v, tools sent: %v", gotAuth == "Bearer "+testKey, strings.Contains(gotBody, `"tools"`))
	}
	if code, b := call("POST", "/api/providers/test", `{"provider":"anthropic"}`); code != 200 || !strings.Contains(b, `"ok":false`) || !strings.Contains(b, "No API key") {
		t.Errorf("test without key: %d %s", code, b)
	}
	if code, b := call("POST", "/api/providers/clients", `{"clients":[{"kind":"api","provider":"anthropic"},{"kind":"cli","agent":"claude"}]}`); code != 200 || !strings.Contains(b, `"agent":"claude"`) {
		t.Errorf("clients: %d %s", code, b)
	}
	if code, b := call("POST", "/api/providers/key/delete", `{"provider":"custom"}`); code != 200 || !strings.Contains(b, `"has_key":false`) || keys["custom"] != "" {
		t.Errorf("delete: %d %s", code, b)
	}
	if code, _ := call("POST", "/api/providers/clients", `{"clients":[]}`); code != 200 {
		t.Errorf("reset: %d", code)
	}
	if e.srv.askCfg().API != nil || !e.srv.askCfg().Usable() {
		t.Error("an empty list should leave the CLI-only setup")
	}
	raw, _ := os.ReadFile(e.srv.cfg.Providers)
	all := strings.Join(bodies, "\n") + logs.String() + string(raw)
	if strings.Contains(all, testKey) {
		t.Errorf("the key appeared in a response, a log or the settings file")
	}
	var st provider.Settings
	if json.Unmarshal(raw, &st) != nil {
		t.Error("settings not JSON")
	}
}
