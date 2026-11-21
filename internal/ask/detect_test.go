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

// winDetect builds a fake Windows profile with one file at rel under it and the environment that
// points at it, and runs Detect in Windows mode.
func winDetect(t *testing.T, rel, pathext string) (name, path string, ok bool) {
	t.Helper()
	winMode = true
	t.Cleanup(func() { winMode = os.PathSeparator == '\\' })
	t.Setenv("PATHEXT", pathext)
	home := t.TempDir()
	full := filepath.Join(home, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{
		"USERPROFILE":  home,
		"APPDATA":      filepath.Join(home, "AppData", "Roaming"),
		"LOCALAPPDATA": filepath.Join(home, "AppData", "Local"),
		"ProgramFiles": filepath.Join(home, "Program Files"),
	}
	return Detect("", ExtraDirs(func(k string) string { return env[k] }))
}

func TestDetectWindowsLocations(t *testing.T) {
	for _, tc := range []struct{ name, rel, want string }{
		{"native installer", `.local/bin/claude.exe`, "claude"},
		{"npm claude shim", `AppData/Roaming/npm/claude.cmd`, "claude"},
		{"npm codex shim", `AppData/Roaming/npm/codex.cmd`, "codex"},
		{"npm gemini shim", `AppData/Roaming/npm/gemini.cmd`, "gemini"},
		{"npm powershell shim", `AppData/Roaming/npm/claude.ps1`, "claude"},
		{"winget link", `AppData/Local/Microsoft/WinGet/Links/claude.exe`, "claude"},
		{"scoop shim", `scoop/shims/gemini.exe`, "gemini"},
		{"scoop cmd shim", `scoop/shims/codex.cmd`, "codex"},
	} {
		got, p, ok := winDetect(t, tc.rel, ".COM;.EXE;.BAT;.CMD")
		if !ok || got != tc.want || filepath.Base(p) != filepath.Base(tc.rel) {
			t.Errorf("%s: %q %q %v", tc.name, got, p, ok)
		}
	}
	if _, _, ok := winDetect(t, `.local/bin/claude.txt`, ""); ok {
		t.Error("a .txt file was taken for claude")
	}
}

func TestPathExts(t *testing.T) {
	got := pathExts(".CMD;.VBS")
	want := []string{".exe", ".com", ".cmd", ".vbs", ".bat", ".ps1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRegistryPath(t *testing.T) {
	env := map[string]string{"USERPROFILE": `C:\Users\me`}
	out := "\r\nHKEY_CURRENT_USER\\Environment\r\n    Path    REG_EXPAND_SZ    %USERPROFILE%\\.local\\bin;C:\\Program Files\\Tools;%NOPE%\r\n\r\n"
	want := `C:\Users\me\.local\bin;C:\Program Files\Tools;%NOPE%`
	if got := parseRegPath(out, func(k string) string { return env[k] }); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := parseRegPath("ERROR: not found", nil); got != "" {
		t.Errorf("got %q", got)
	}
}

func TestScriptLine(t *testing.T) {
	prompt := "a \"b\" 50% & c\nline 2 \\"
	for _, tc := range []struct {
		name string
		argv []string
		exe  string
		line string
	}{
		{"cmd shim", []string{`C:\Users\me\AppData\Roaming\npm\claude.cmd`, "-p", prompt},
			"cmd.exe", `cmd.exe /d /s /c ""C:\Users\me\AppData\Roaming\npm\claude.cmd" "-p" "a 'b' 50％ & c line 2 \\""`},
		{"powershell script", []string{`C:\x\claude.ps1`, "-p", `say "hi"\`},
			"powershell.exe", `powershell.exe -NoProfile -NonInteractive -ExecutionPolicy Bypass -File "C:\x\claude.ps1" "-p" "say \"hi\"\\"`},
	} {
		exe, line, ok := scriptLine(tc.argv)
		if !ok || exe != tc.exe || line != tc.line {
			t.Errorf("%s:\n got %q %q %v\nwant %q %q", tc.name, exe, line, ok, tc.exe, tc.line)
		}
	}
	if _, _, ok := scriptLine([]string{`C:\x\claude.exe`}); ok {
		t.Error(".exe is not a script")
	}
}
