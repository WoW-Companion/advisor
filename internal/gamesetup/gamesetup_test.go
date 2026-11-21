package gamesetup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

var scribeLua = func() string {
	b, err := os.ReadFile("../../testdata/foreverscribe/level1.lua")
	if err != nil {
		panic(err)
	}
	return string(b)
}()

func statuses(items []Item) map[string]string {
	m := map[string]string{}
	for _, it := range items {
		m[it.ID] = it.Status
	}
	return m
}

func TestCheckStates(t *testing.T) {
	full := func(root string) string {
		g := filepath.Join(root, "_forever_")
		write(t, filepath.Join(g, "Interface", "AddOns", "ForeverScribe", "ForeverScribe.toc"), "x")
		write(t, filepath.Join(g, "Interface", "AddOns", "WoWCompanion", "WoWCompanion.toc"), "## Version: 0.2.0\n")
		write(t, filepath.Join(g, "WTF", "Account", "A", "SavedVariables", "ForeverScribe.lua"), scribeLua)
		write(t, filepath.Join(g, "WTF", "Config.wtf"), "SET gxWindow \"1\"\nSET gxMaximize \"1\"\n")
		return g
	}
	cases := []struct {
		name  string
		build func(root string)
		want  map[string]string
	}{
		{"all present", func(r string) { full(r) }, map[string]string{IDGame: OK, IDScribe: OK, IDData: OK, IDAddon: OK, IDDisplay: OK}},
		{"scribe and addon missing", func(r string) {
			g := full(r)
			os.RemoveAll(filepath.Join(g, "Interface", "AddOns", "ForeverScribe"))
			os.RemoveAll(filepath.Join(g, "Interface", "AddOns", "WoWCompanion"))
		}, map[string]string{IDGame: OK, IDScribe: Bad, IDData: OK, IDAddon: Warn}},
		{"no data file", func(r string) {
			os.Remove(filepath.Join(full(r), "WTF", "Account", "A", "SavedVariables", "ForeverScribe.lua"))
		}, map[string]string{IDScribe: OK, IDData: Bad}},
		{"data without a character", func(r string) {
			write(t, filepath.Join(full(r), "WTF", "Account", "A", "SavedVariables", "ForeverScribe.lua"), `ForeverScribeDB = {}`)
		}, map[string]string{IDData: Bad}},
		{"no game folder", func(r string) {}, map[string]string{IDGame: Bad, IDScribe: Bad, IDData: Bad}},
		{"exclusive fullscreen", func(r string) {
			write(t, filepath.Join(full(r), "WTF", "Config.wtf"), "SET gxWindow \"0\"\nSET gxMaximize \"0\"\n")
		}, map[string]string{IDDisplay: Warn}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			c.build(root)
			got := statuses(Check(Options{GameDir: root}))
			for id, w := range c.want {
				if got[id] != w {
					t.Errorf("%s = %q, want %q (all: %v)", id, got[id], w, got)
				}
			}
		})
	}
}

func TestReadDisplay(t *testing.T) {
	cases := []struct {
		cfg       string
		known     bool
		exclusive bool
		mode      string
	}{
		{"SET gxWindow \"1\"\nSET gxMaximize \"1\"\n", true, false, "Windowed (Fullscreen)"},
		{"SET gxWindow \"1\"\nSET gxMaximize \"0\"\n", true, false, "Windowed"},
		{"SET gxWindow \"0\"\nSET gxMaximize \"0\"\n", true, true, "Fullscreen"},
		{"SET textLocale \"enUS\"\n", false, false, ""},
	}
	for _, c := range cases {
		g := t.TempDir()
		write(t, filepath.Join(g, "WTF", "Config.wtf"), c.cfg)
		d := ReadDisplay(g)
		if d.Known != c.known || d.Exclusive != c.exclusive || d.Mode != c.mode {
			t.Errorf("%q: got %+v", c.cfg, d)
		}
		if b, _ := os.ReadFile(filepath.Join(g, "WTF", "Config.wtf")); string(b) != c.cfg {
			t.Error("Config.wtf was modified")
		}
	}
	if ReadDisplay(t.TempDir()).Unreadable == nil {
		t.Error("a missing Config.wtf should be unreadable, not an error state")
	}
}

func TestGameFolderChoice(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "_forever_", "WTF", "x"), "")
	if g, ok := GameFolder(root); !ok || filepath.Base(g) != "_forever_" {
		t.Errorf("root: %q %v", g, ok)
	}
	if _, ok := GameFolder(filepath.Join(root, "nope")); ok {
		t.Error("missing folder found")
	}
}

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{{"0.3.0", "0.2.0", true}, {"0.2.0", "0.2.0", false}, {"0.10.0", "0.9.0", true}, {"v0.2.1", "0.2", true}, {"0.1.0", "0.2.0", false}} {
		if Newer(c.a, c.b) != c.want {
			t.Errorf("Newer(%s,%s)", c.a, c.b)
		}
	}
}

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for n, c := range files {
		w, err := zw.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(c))
	}
	zw.Close()
	return b.Bytes()
}

func sumsFor(name string, z []byte) []byte {
	h := sha256.Sum256(z)
	return []byte(hex.EncodeToString(h[:]) + "  " + name + "\n")
}

func TestVerifySum(t *testing.T) {
	z := []byte("zip bytes")
	if err := VerifySum(z, "a.zip", sumsFor("a.zip", z)); err != nil {
		t.Error(err)
	}
	if err := VerifySum([]byte("other"), "a.zip", sumsFor("a.zip", z)); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Errorf("mismatch accepted: %v", err)
	}
	if err := VerifySum(z, "b.zip", sumsFor("a.zip", z)); err == nil {
		t.Error("missing line accepted")
	}
}

func TestExtractAddon(t *testing.T) {
	good := map[string]string{
		"WoWCompanion/WoWCompanion.toc":               "## Version: 0.3.0\n",
		"WoWCompanion/Digest.lua":                     "-- addon code",
		"WoWCompanion_Digest/WoWCompanion_Digest.toc": "toc",
	}
	bad := map[string]map[string]string{
		"digest data":  {"WoWCompanion/a.lua": "x", "WoWCompanion_Digest/Digest.lua": "evil"},
		"dotdot":       {"WoWCompanion/../../evil.lua": "x"},
		"absolute":     {"/etc/evil": "x"},
		"drive":        {"C:/evil": "x"},
		"backslash":    {`WoWCompanion\..\evil`: "x"},
		"answers":      {"WoWCompanion_Answers/Answers.lua": "x"},
		"other folder": {"Other/a.lua": "x"},
	}
	for name, files := range bad {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			addons := filepath.Join(dir, "AddOns")
			if err := ExtractAddon(makeZip(t, files), addons); err == nil {
				t.Fatal("accepted")
			}
			if _, err := os.Stat(addons); err == nil {
				t.Error("something was written before the refusal")
			}
			if _, err := os.Stat(filepath.Join(dir, "evil.lua")); err == nil {
				t.Error("escaped the folder")
			}
		})
	}
	t.Run("valid keeps data", func(t *testing.T) {
		addons := t.TempDir()
		data := filepath.Join(addons, "WoWCompanion_Digest", "Digest.lua")
		answers := filepath.Join(addons, "WoWCompanion_Answers", "Answers.lua")
		write(t, data, "player data")
		write(t, answers, "answers")
		if err := ExtractAddon(makeZip(t, good), addons); err != nil {
			t.Fatal(err)
		}
		for p, want := range map[string]string{data: "player data", answers: "answers", filepath.Join(addons, "WoWCompanion", "WoWCompanion.toc"): "## Version: 0.3.0\n"} {
			if b, _ := os.ReadFile(p); string(b) != want {
				t.Errorf("%s = %q", p, b)
			}
		}
	})
}

func TestInstallAddonRefusesMismatch(t *testing.T) {
	game := t.TempDir()
	z := makeZip(t, map[string]string{"WoWCompanion/WoWCompanion.toc": "x"})
	assets := map[string][]byte{"/z.zip": z, "/SHA256SUMS": sumsFor("WoWCompanion-1.0.0.zip", []byte("different"))}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(assets[r.URL.Path]) }))
	defer srv.Close()
	// The test server is not github.com, so the host rule refuses before any write.
	rel := &AddonRelease{Version: "1.0.0", ZipName: "WoWCompanion-1.0.0.zip", ZipURL: srv.URL + "/z.zip", SumsURL: srv.URL + "/SHA256SUMS"}
	err := InstallAddon(context.Background(), srv.Client(), game, rel)
	if err == nil || !strings.Contains(err.Error(), "github.com") {
		t.Fatalf("non-github host accepted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(game, "Interface")); err == nil {
		t.Error("wrote on refusal")
	}
	if err := InstallAddon(context.Background(), nil, game, &AddonRelease{ZipURL: "https://github.com/x.zip"}); err == nil {
		t.Error("install without SHA256SUMS accepted")
	}
}

func TestLatestAddonHostRule(t *testing.T) {
	for _, u := range []string{"http://github.com/x", "https://evil.example/x", "https://github.com.evil.example/x"} {
		if _, err := LatestAddon(context.Background(), nil, u); err == nil {
			t.Errorf("%s accepted", u)
		}
	}
}

func fakeAgent(t *testing.T, body string) []string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "agent.sh")
	write(t, p, "#!/bin/sh\n"+body+"\n")
	os.Chmod(p, 0o755)
	return []string{p}
}

func TestTestAI(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("shell fake agent")
	}
	cases := []struct {
		body, msg string
		ok        bool
	}{
		{`echo OK`, "OK", true},
		{`echo "Please sign in first" >&2; echo "more" >&2; exit 1`, "Please sign in first", false},
		{`exit 3`, "the agent exited with status 3", false},
	}
	for _, c := range cases {
		ok, msg := TestAI(context.Background(), fakeAgent(t, c.body))
		if ok != c.ok || msg != c.msg {
			t.Errorf("%q: got %v %q", c.body, ok, msg)
		}
	}
	if ok, _ := TestAI(context.Background(), nil); ok {
		t.Error("no agent should fail")
	}
	if it := AIItem(""); it.Status != Bad || it.Action != "" {
		t.Errorf("none detected: %+v", it)
	}
	if it := AIItem("claude"); it.Action != ActTestAI {
		t.Errorf("detected: %+v", it)
	}
	_ = fmt.Sprint
}
