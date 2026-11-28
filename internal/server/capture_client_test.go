package server

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func fsCapture(chars ...string) string {
	s := "ForeverScribeDB = { client = { version = \"1.60.1\", build = \"71091\" }, chars = {\n"
	for _, c := range chars {
		s += c + "\n"
	}
	return s + "} }\n"
}

func fsChar(name string, at int, build string) string {
	return fmt.Sprintf(`["R:%s"] = { snapshot = { identity = { name = %q, realm = "R" }, level = 20, capturedAt = %d }, client = { version = "1.60.1", build = %q } },`, name, name, at, build)
}

func wowcCapture(name string, at int, build string) string {
	return fmt.Sprintf(`WoWCompanionDB = { characters = { ["R:%s"] = { name = %q, realm = "R", level = 20, capturedAt = %d, client = { version = "1.60.1", build = %q } } } }
`, name, name, at, build)
}

func TestCaptureClientNewest(t *testing.T) {
	cases := []struct {
		name, fs, wowc, want string
	}{
		{"stale ForeverScribe, newer WoW Companion", fsCapture(fsChar("Old", 100, "71091")), wowcCapture("New", 900, "71139"), "71139"},
		{"two characters, newest listed first", fsCapture(fsChar("Aaa", 900, "71139"), fsChar("Zzz", 100, "71091")), "", "71139"},
		{"two characters, newest listed last", fsCapture(fsChar("Aaa", 100, "71091"), fsChar("Zzz", 900, "71139")), "", "71139"},
		{"only ForeverScribe", fsCapture(fsChar("Old", 100, "71091")), "", "71091"},
		{"ForeverScribe newer than WoW Companion", fsCapture(fsChar("Old", 950, "71200")), wowcCapture("New", 900, "71139"), "71200"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			fs := filepath.Join(dir, "ForeverScribe.lua")
			os.WriteFile(fs, []byte(c.fs), 0o644)
			if c.wowc != "" {
				os.WriteFile(filepath.Join(dir, "WoWCompanion.lua"), []byte(c.wowc), 0o644)
			}
			if got := CaptureClient(fs).Build; got != c.want {
				t.Errorf("build %q, want %q", got, c.want)
			}
		})
	}
}
