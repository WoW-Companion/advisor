package ask

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// fakeBin makes a directory holding executables with the given names.
func fakeBin(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if runtime.GOOS == "windows" {
			n += ".exe"
		}
		if err := os.WriteFile(filepath.Join(dir, n), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDetect(t *testing.T) {
	all, two, only := fakeBin(t, "gemini", "codex", "claude"), fakeBin(t, "gemini", "codex"), fakeBin(t, "gemini")
	plain := t.TempDir()
	if err := os.WriteFile(filepath.Join(plain, "claude"), nil, 0o644); err != nil { // not executable
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		path  string
		extra []string
		want  string
	}{
		{"claude first", filepath.Join(only, "x") + string(os.PathListSeparator) + all, nil, "claude"},
		{"codex before gemini", two, nil, "codex"},
		{"gemini alone", only, nil, "gemini"},
		{"extra dir after PATH", plain, []string{only}, "gemini"},
		{"CLI order beats PATH-before-extras", two, []string{all}, "claude"},
		{"nothing", plain, nil, ""},
	} {
		got, p, ok := Detect(tc.path, tc.extra)
		if got != tc.want || ok != (tc.want != "") || ok && filepath.Base(p)[:len(got)] != got {
			t.Errorf("%s: %q %q %v", tc.name, got, p, ok)
		}
	}
}

func TestResolveAuto(t *testing.T) {
	bin := fakeBin(t, "codex")
	t.Setenv("PATH", bin)
	extraDirs = func(func(string) string) []string { return nil }
	t.Cleanup(func() { extraDirs = ExtraDirs })
	conf := filepath.Join(t.TempDir(), "ask.json")
	os.WriteFile(conf, []byte(`{"agent": "gemini"}`), 0o644)
	none := filepath.Join(t.TempDir(), "none.json")
	for _, tc := range []struct {
		name, flag, conf, wantName, wantSource string
	}{
		{"flag beats detection", "claude", none, "claude", SourceFlag},
		{"flag beats config", "my cli", conf, "my", SourceFlag},
		{"config beats detection", "", conf, "gemini", SourceConfig},
		{"auto beats config", "auto", conf, "codex", SourceAuto},
		{"detected", "", none, "codex", SourceAuto},
	} {
		got, err := ResolveAuto(tc.flag, tc.conf)
		if err != nil || got.Name != tc.wantName || got.Source != tc.wantSource || len(got.Command) == 0 {
			t.Errorf("%s: %+v, %v", tc.name, got, err)
		}
	}
	if got, _ := ResolveAuto("", none); filepath.Dir(got.Command[0]) != bin || !reflect.DeepEqual(got.Command[1:], Presets["codex"][1:]) {
		t.Errorf("detected command = %v", got.Command)
	}
}
