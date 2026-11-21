package ask

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRegistry struct{ paths []string }

func (f *fakeRegistry) Paths(func(string) string) []string { return f.paths }

// winFake puts the package in Windows mode with a fake profile and registry, and returns the
// profile directory. Files are made empty; Windows mode treats any regular file as executable.
func winFake(t *testing.T, reg *fakeRegistry, files ...string) string {
	t.Helper()
	oldReg := registry
	winMode, registry = true, reg
	t.Cleanup(func() { winMode, registry = os.PathSeparator == '\\', oldReg })
	home := t.TempDir()
	for _, f := range files {
		p := filepath.Join(home, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", "")
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(home, "AppData", "Local"))
	t.Setenv("ProgramFiles", filepath.Join(home, "Program Files"))
	return home
}

func TestLookPathLayouts(t *testing.T) {
	for _, tc := range []struct{ name, file, cmd string }{
		{"npm", "AppData/Roaming/npm/codex.cmd", "codex"},
		{"winget", "AppData/Local/Microsoft/WinGet/Links/codex.exe", "codex"},
		{"native", ".local/bin/claude.exe", "claude"},
		{"scoop", "scoop/shims/gemini.cmd", "gemini"},
	} {
		home := winFake(t, &fakeRegistry{}, tc.file)
		got, err := LookPath(tc.cmd)
		if want := filepath.Join(home, filepath.FromSlash(tc.file)); err != nil || got != want {
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, want)
		}
	}
}

func TestLookPathOrder(t *testing.T) {
	// The registry's folder beats the process PATH, which beats the install folders; in one
	// folder a real program beats a shim, and a .ps1 gives way to a sibling .cmd.
	home := winFake(t, &fakeRegistry{}, "reg/codex.cmd", "reg/codex.exe", "proc/codex.exe", "AppData/Roaming/npm/codex.cmd",
		"ps/claude.ps1", "ps/claude.cmd", "only/gemini.ps1")
	reg := &fakeRegistry{paths: []string{filepath.Join(home, "reg")}}
	registry = reg
	t.Setenv("PATH", filepath.Join(home, "proc"))
	if got, _ := LookPath("codex"); got != filepath.Join(home, "reg", "codex.exe") {
		t.Errorf("registry first: %q", got)
	}
	reg.paths = nil
	if got, _ := LookPath("codex"); got != filepath.Join(home, "proc", "codex.exe") {
		t.Errorf("process PATH second: %q", got)
	}
	t.Setenv("PATH", "")
	if got, _ := LookPath("codex"); got != filepath.Join(home, "AppData", "Roaming", "npm", "codex.cmd") {
		t.Errorf("install folders last: %q", got)
	}
	reg.paths = []string{filepath.Join(home, "ps"), filepath.Join(home, "only")}
	if got, _ := LookPath("claude"); got != filepath.Join(home, "ps", "claude.cmd") {
		t.Errorf("ps1 sibling: %q", got)
	}
	if got, _ := LookPath("gemini"); got != filepath.Join(home, "only", "gemini.ps1") {
		t.Errorf("lone ps1: %q", got)
	}
}

func TestFreshPathMerges(t *testing.T) {
	sep := string(os.PathListSeparator)
	winFake(t, &fakeRegistry{paths: []string{"/a" + sep + "/b", "/A" + sep + "/c"}})
	t.Setenv("PATH", "/c"+sep+"/proc")
	want := strings.Join([]string{"/a", "/b", "/c", "/proc"}, sep)
	if got := freshPath(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	n := 0
	for _, kv := range withEnv("X=1") {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			n++
			if kv != "PATH="+want {
				t.Errorf("env PATH = %q", kv)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d PATH entries", n)
	}
}

// A client installed while the program runs is found on the next resolve, with no restart.
func TestRescanSeesNewInstall(t *testing.T) {
	home := winFake(t, &fakeRegistry{}, "AppData/Roaming/npm/codex.cmd")
	extraDirs = func(func(string) string) []string { return nil }
	t.Cleanup(func() { extraDirs = ExtraDirs })
	conf := filepath.Join(t.TempDir(), "none.json")
	if a, err := ResolveAuto("", conf); err != nil || a.Source != SourceNone {
		t.Fatalf("before install: %+v, %v", a, err)
	}
	newDir := filepath.Join(home, "new")
	os.MkdirAll(newDir, 0o755)
	os.WriteFile(filepath.Join(newDir, "codex.cmd"), []byte("x"), 0o755)
	registry.(*fakeRegistry).paths = []string{newDir}
	a, err := ResolveAuto("", conf)
	if err != nil || a.Name != "codex" || a.Command[0] != filepath.Join(newDir, "codex.cmd") {
		t.Fatalf("after install: %+v, %v", a, err)
	}
	if !strings.Contains(strings.Join(withEnv(), "\n"), newDir) {
		t.Error("the child environment lacks the new PATH entry")
	}
}

func TestScriptQuoting(t *testing.T) {
	q := `say "hi" to C:\Program Files\x\ & 100%`
	_, line, _ := scriptLine([]string{`C:\Users\me\AppData\Roaming\npm\codex.cmd`, "exec", q})
	if want := `cmd.exe /d /s /c ""C:\Users\me\AppData\Roaming\npm\codex.cmd" "exec" "say 'hi' to C:\Program Files\x\ & 100％""`; line != want {
		t.Errorf("cmd:\n got %s\nwant %s", line, want)
	}
	_, line, _ = scriptLine([]string{`C:\x\codex.ps1`, q})
	if want := `-File "C:\x\codex.ps1" "say \"hi\" to C:\Program Files\x\ & 100%"`; !strings.HasSuffix(line, want) {
		t.Errorf("ps1: %s", line)
	}
}

func TestErrorText(t *testing.T) {
	if _, err := Launch(context.Background(), []string{"codex-not-here"}); err == nil ||
		!strings.HasPrefix(err.Error(), "Couldn't find Codex-not-here:") {
		t.Errorf("not found: %v", err)
	}
	// A file that exists but cannot be run: the line names the path and the OS reason.
	bad := filepath.Join(t.TempDir(), "codex")
	os.WriteFile(bad, []byte("not a program"), 0o755)
	err := execAgent(context.Background(), Config{Command: []string{bad}}, Question{}, "hi", nil, &capped{w: &strings.Builder{}, left: 10})
	if err == nil || !strings.HasPrefix(err.Error(), "Couldn't start Codex ("+bad+"): ") || strings.Contains(err.Error(), "fork/exec") {
		t.Errorf("start failure: %v", err)
	}
	// Early non-zero exit with no output: the first stderr line; with output: the status.
	t.Setenv("FAKE_AGENT", "fail")
	err = execAgent(context.Background(), Config{Command: []string{os.Args[0]}}, Question{}, "hi", nil, &capped{w: &strings.Builder{}, left: 10})
	if err == nil || err.Error() != "boom" {
		t.Errorf("early exit: %v", err)
	}
}
