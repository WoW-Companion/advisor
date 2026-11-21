package doctor

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/setup"
)

// captured is the capturedAt in testdata/foreverscribe/tree_priest.lua; its client is 9.9.9 build 99999.
const captured = 1700100000

func write(t *testing.T, p, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

type world struct {
	sv, game, cache, home string
	opt                   Options
}

// build makes a healthy game folder, dataset cache and client config under a temp dir.
func build(t *testing.T) world {
	root := t.TempDir()
	w := world{game: filepath.Join(root, "game"), cache: filepath.Join(root, "cache"), home: filepath.Join(root, "home")}
	w.sv = filepath.Join(w.game, "WTF", "Account", "ACCT", "SavedVariables", "ForeverScribe.lua")
	src, err := os.ReadFile(filepath.Join("..", "..", "testdata", "foreverscribe", "tree_priest.lua"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, w.sv, string(src))
	for _, a := range Addons {
		write(t, filepath.Join(w.game, "Interface", "AddOns", a, a+".toc"), "## Interface: 16001\n## Version: 1.2.3\n")
	}
	sum := sha256.Sum256([]byte("rows"))
	write(t, filepath.Join(w.cache, "9.9.9.99999", "t.bin"), "rows")
	write(t, filepath.Join(w.cache, "9.9.9.99999", "manifest.json"),
		fmt.Sprintf(`{"dataset_version":"v1","game_build":"9.9.9.99999","tables":[{"path":"t.bin","sha256":%q}]}`, hex.EncodeToString(sum[:])))
	write(t, filepath.Join(w.home, ".gemini", "settings.json"), `{"mcpServers":{"wowc-advisor":{}}}`)
	write(t, filepath.Join(w.home, ".codex", "config.toml"), "model = 'x'\n")
	w.opt = Options{SavedVariables: w.sv, Cache: w.cache, Now: time.Unix(captured+600, 0),
		Env: setup.Env{GOOS: "linux", Home: w.home, Cwd: filepath.Join(w.home, "proj")}}
	return w
}

func find(rep Report, name string) Check {
	for _, c := range rep.Checks {
		if c.Name == name {
			return c
		}
	}
	return Check{Name: name, Status: "missing"}
}

func TestRun(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(t *testing.T, w *world)
		check  string
		status string
		detail string // substring
		fix    string // substring; empty when the check must carry no fix
	}{
		{"healthy sv", func(*testing.T, *world) {}, "SavedVariables", Pass, "1 character", ""},
		{"sv unset", func(_ *testing.T, w *world) { w.opt.SavedVariables = "" }, "SavedVariables", Fail, "no SavedVariables", "--savedvariables"},
		{"sv missing", func(t *testing.T, w *world) { os.Remove(w.sv) }, "SavedVariables", Fail, "not found", "log in"},
		{"sv unparsable", func(t *testing.T, w *world) { write(t, w.sv, "ForeverScribeDB = {") }, "SavedVariables", Fail, "could not read", "log in"},
		{"sv no characters", func(t *testing.T, w *world) { write(t, w.sv, "ForeverScribeDB = {}") }, "SavedVariables", Fail, "no characters", "log out"},

		{"fresh capture", func(*testing.T, *world) {}, "Capture age", Pass, "10m0s ago", ""},
		{"stale capture", func(_ *testing.T, w *world) { w.opt.Now = time.Unix(captured+2*3600, 0) }, "Capture age", Fail, "2h0m0s ago", "/reload in game"},
		{"no capture", func(t *testing.T, w *world) { os.Remove(w.sv) }, "Capture age", Fail, "no capture", "SavedVariables check"},

		{"addon installed", func(*testing.T, *world) {}, "Addon WoWCompanion", Pass, "version 1.2.3", ""},
		{"addon no version", func(t *testing.T, w *world) {
			write(t, filepath.Join(w.game, "Interface", "AddOns", "WoWCompanion_Digest", "WoWCompanion_Digest.toc"), "## Title: x\n")
		}, "Addon WoWCompanion_Digest", Pass, "version unknown", ""},
		{"addon missing", func(t *testing.T, w *world) {
			os.RemoveAll(filepath.Join(w.game, "Interface", "AddOns", "WoWCompanion_Answers"))
		}, "Addon WoWCompanion_Answers", Fail, "not installed", "install WoWCompanion_Answers"},
		{"addons outside a game folder", func(_ *testing.T, w *world) { w.opt.SavedVariables = filepath.Join(w.home, "x.lua") },
			"Addon WoWCompanion", Fail, "cannot find", "<game>/WTF"},

		{"dataset ok", func(*testing.T, *world) {}, "Dataset cache", Pass, "1 verified", ""},
		{"dataset absent", func(_ *testing.T, w *world) { os.RemoveAll(w.cache) }, "Dataset cache", Fail, "no dataset cached", "dataset update"},
		{"dataset corrupt", func(t *testing.T, w *world) { write(t, filepath.Join(w.cache, "9.9.9.99999", "t.bin"), "bad") },
			"Dataset cache", Fail, "failed verification", "dataset update --build 9.9.9.99999"},
		{"dataset matches", func(*testing.T, *world) {}, "Dataset build", Pass, "matches client build 99999", ""},
		{"dataset other build", func(t *testing.T, w *world) {
			if err := os.Rename(filepath.Join(w.cache, "9.9.9.99999"), filepath.Join(w.cache, "9.9.9.11111")); err != nil {
				t.Fatal(err)
			}
			write(t, filepath.Join(w.cache, "9.9.9.11111", "manifest.json"), strings.ReplaceAll(readFile(t, filepath.Join(w.cache, "9.9.9.11111", "manifest.json")), "99999", "11111"))
		}, "Dataset build", Fail, "no cached dataset matches", "dataset update"},
		{"dataset build without cache", func(_ *testing.T, w *world) { os.RemoveAll(w.cache) }, "Dataset build", Fail, "no verified dataset", "dataset cache"},

		{"digest writable", func(*testing.T, *world) {}, "Digest.lua writable", Pass, "Digest.lua", ""},
		{"digest folder gone", func(t *testing.T, w *world) {
			os.RemoveAll(filepath.Join(w.game, "Interface", "AddOns", "WoWCompanion_Digest"))
		}, "Digest.lua writable", Fail, "cannot write", "install WoWCompanion_Digest"},
		{"digest read-only file", func(t *testing.T, w *world) {
			p := filepath.Join(w.game, "Interface", "AddOns", "WoWCompanion_Digest", "Digest.lua")
			write(t, p, "x")
			os.Chmod(p, 0o444)
		}, "Digest.lua writable", Fail, "read-only", "make"},
		{"answers writable", func(*testing.T, *world) {}, "Answers.lua writable", Pass, "Answers.lua", ""},
		{"answers folder gone", func(t *testing.T, w *world) {
			os.RemoveAll(filepath.Join(w.game, "Interface", "AddOns", "WoWCompanion_Answers"))
		}, "Answers.lua writable", Fail, "cannot write", "install WoWCompanion_Answers"},

		{"client configured", func(*testing.T, *world) {}, "Client gemini", Info, "mentions wowc-advisor", ""},
		{"client lacks entry", func(*testing.T, *world) {}, "Client codex", Info, "does not mention", "setup codex --write"},
		{"client not installed", func(*testing.T, *world) {}, "Client claude-code", Info, "no config", "setup claude-code --write"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w := build(t)
			c.mutate(t, &w)
			got := find(Run(w.opt), c.check)
			if got.Status != c.status || !strings.Contains(got.Detail, c.detail) {
				t.Fatalf("%s = %+v, want %s containing %q", c.check, got, c.status, c.detail)
			}
			if c.fix == "" && got.Fix != "" || !strings.Contains(got.Fix, c.fix) {
				t.Errorf("%s fix = %q, want %q", c.check, got.Fix, c.fix)
			}
		})
	}
}

func readFile(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestReport is the one smoke test: a healthy setup is ok and renders every check, and info
// lines never fail it.
func TestReport(t *testing.T) {
	w := build(t)
	rep := Run(w.opt)
	if !rep.OK || !strings.Contains(rep.Text, "All checks passed.") || strings.Count(rep.Text, "\n") < len(rep.Checks) {
		t.Errorf("healthy report: %+v\n%s", rep.OK, rep.Text)
	}
	w.opt.Now = time.Unix(captured+7200, 0)
	rep = Run(w.opt)
	if rep.OK || !strings.Contains(rep.Text, "FAIL  Capture age") || !strings.Contains(rep.Text, "fix: /reload in game") {
		t.Errorf("stale report:\n%s", rep.Text)
	}
}
