package companion

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/gamesetup"
	"github.com/WoW-Companion/advisor/internal/provider"
)

const diagSave = `WoWCompanionDB = {
	["characters"] = { ["Realm1:Char1"] = { ["name"] = "Zorbin", ["realm"] = "Mankritz", ["guild"] = "Ironquills", ["level"] = 12, ["class"] = "MAGE", ["capturedAt"] = 2000 } },
	["raw"] = { ["errors"] = {
		{ ["kind"] = "lua", ["time"] = "10:01", ["message"] = "old one" },
		{ ["kind"] = "lua", ["time"] = "10:02", ["message"] = "e2" },
		{ ["kind"] = "lua", ["time"] = "10:03", ["message"] = "e3" },
		{ ["kind"] = "lua", ["time"] = "10:04", ["message"] = "e4" },
		{ ["kind"] = "lua", ["time"] = "10:05", ["message"] = "e5" },
		{ ["kind"] = "lua", ["time"] = "10:06", ["message"] = "Zorbin of Mankritz in <Ironquills> failed at C:\\Users\\bobby\\Games\\x.lua:4 mail zorbin@example.com Bearer abc123def sk-ant-abcdefghijkl0123 tok 0123456789abcdef0123456789abcdef0123" },
	} },
}`

// diagServer is a companion whose save, game folder and home are all synthetic.
func diagServer(t *testing.T) (*Server, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	game := filepath.Join(home, "Games", "WoW")
	svDir := filepath.Join(game, "WTF", "Account", "ACCT", "SavedVariables")
	addon := filepath.Join(game, "Interface", "AddOns", gamesetup.AddonName)
	slot := filepath.Join(game, "Interface", "AddOns", digest.SlotAddon)
	for _, d := range []string{svDir, addon, slot} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sv := filepath.Join(svDir, "WoWCompanion.lua")
	os.WriteFile(sv, []byte(diagSave), 0o644)
	os.WriteFile(filepath.Join(addon, "WoWCompanion.toc"), []byte("## Version: 0.9.1\n"), 0o644)
	saved := time.Unix(1_700_000_000, 0)
	os.Chtimes(sv, saved, saved)
	prov := filepath.Join(t.TempDir(), "providers.json")
	os.WriteFile(prov, []byte(`{"clients":[{"kind":"api","provider":"openai","model":"gpt-test","base_url":"https://x.test/v1?token=SECRETENDPOINT"}]}`), 0o644)
	keys := provider.FileKeyStore{Dir: t.TempDir()}
	keys.Set("openai", "sk-FAKEKEY1234567890abcdef")
	s := New(Config{ForeverScribe: sv, GameDir: game, Providers: prov, Keys: keys, Version: "9.8.7", Now: func() time.Time { return saved.Add(3 * time.Hour) }})
	s.reload()
	return s, home
}

func TestDiagnosticsHasEverySectionAndStripsNames(t *testing.T) {
	s, home := diagServer(t)
	shell := strings.Repeat("filler line\n", 40) + "1 exit requested: tray quit\n2 note " + home + "/Library/x\n"
	out := s.diagnosticsText(diagRequest{ShellLog: shell, Guard: `{"version":"9.8.7","open":false,"kept":true}`, OS: "Windows 11 (10.0.22631)"})
	for _, want := range []string{"app 9.8.7", "addon installed 0.9.1", "dataset none", "OS Windows 11", "== AI ==", "api, model gpt-test",
		"Digest.lua written after it: missing", "Plan.lua written after it: missing", "3h old", "== Addon errors (last 5) ==", "e5", "== App log ==", "exit requested", "== Launch guard ==", `"kept":true`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	if strings.Contains(out, "old one") {
		t.Error("only the last 5 addon errors are listed")
	}
	if !strings.Contains(out, "Char1") || !strings.Contains(out, "Realm1") || !strings.Contains(out, "Guild1") {
		t.Errorf("placeholders missing:\n%s", out)
	}
	for _, bad := range []string{"Zorbin", "zorbin", "Mankritz", "Ironquills", "bobby", home, "example.com", "abc123def", "sk-ant", "0123456789abcdef", "FAKEKEY", "SECRETENDPOINT", "x.test"} {
		if strings.Contains(out, bad) {
			t.Errorf("%q survived:\n%s", bad, out)
		}
	}
	if !strings.Contains(out, "~/Library/x") {
		t.Errorf("the home folder should show as ~:\n%s", out)
	}
}

func TestDiagnosticsWroteAfterTheSave(t *testing.T) {
	s, home := diagServer(t)
	dir := filepath.Join(home, "Games", "WoW", "Interface", "AddOns", digest.SlotAddon)
	os.WriteFile(filepath.Join(dir, "Digest.lua"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "Plan.lua"), []byte("x"), 0o644)
	old := time.Unix(1_600_000_000, 0)
	os.Chtimes(filepath.Join(dir, "Plan.lua"), old, old)
	os.Chtimes(filepath.Join(dir, "Digest.lua"), time.Unix(1_700_000_100, 0), time.Unix(1_700_000_100, 0))
	out := s.diagnosticsText(diagRequest{})
	if !strings.Contains(out, "Digest.lua written after it: yes") || !strings.Contains(out, "Plan.lua written after it: no") {
		t.Errorf("file ages wrong:\n%s", out)
	}
}

func TestDiagnosticsSizeCap(t *testing.T) {
	s, _ := diagServer(t)
	huge := strings.Repeat(strings.Repeat("ordinary words ", 16)+"\n", 100)
	out := s.diagnosticsText(diagRequest{ShellLog: huge, Guard: strings.Repeat("g", 5000), OS: strings.Repeat("o", 9000)})
	if len(out) > diagMaxBytes {
		t.Errorf("block is %d bytes, cap %d", len(out), diagMaxBytes)
	}
	if !strings.Contains(out, "== Launch guard ==") || !strings.Contains(out, "== Addon errors") {
		t.Error("the old log lines go first; the other sections stay")
	}
	if got := capText(strings.Repeat("x\n", 5000), 100); len(got) > 100 || !strings.Contains(got, "[cut to fit]") {
		t.Errorf("capText: %d bytes", len(got))
	}
}

func TestStripperEachKind(t *testing.T) {
	strip := stripper(map[string]string{"Zorbin": "Char1", "Mankritz": "Realm1"}, "/home/pat")
	cases := map[string]string{
		"ZORBIN-Mankritz":                      "Char1-Realm1",
		"open /home/pat/x/y.lua":               "open ~/x/y.lua",
		`open C:\Users\Someone\AppData\x`:      `open ~\AppData\x`,
		"mail me at a.b+c@mail.example.org ok": "mail me at <email> ok",
		"Authorization: Bearer abcDEF123":      "Authorization: Bearer <key>",
		"key sk-proj-abcdef123456 end":         "key <key> end",
		"api_key=hunter2hunter2":               "api_key=<key>",
		"hex 0123456789abcdef0123456789abcdef": "hex <key>",
		"plain text stays":                     "plain text stays",
	}
	for in, want := range cases {
		if got := strip(in); got != want {
			t.Errorf("strip(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDiagnosticsEndpoint(t *testing.T) {
	s, _ := diagServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/diagnostics", strings.NewReader(`{"shell_log":"1 start","os":"TestOS 1"}`))
	rec := httptest.NewRecorder()
	s.diagnostics(rec, req)
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || !strings.Contains(got["text"], "TestOS 1") {
		t.Errorf("answer %q (%v)", rec.Body.String(), err)
	}
}
