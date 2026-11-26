package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) string {
	root := t.TempDir()
	write(t, root, "VERSION", "0.7.9\n")
	write(t, root, "CHANGELOG.md", "# Changelog\n\n## 0.7.9\n\n- Old.\n")
	write(t, root, "overlay/src-tauri/tauri.conf.json", "{\n  \"version\": \"0.7.9\",\n  \"x\": 1\n}\n")
	write(t, root, "overlay/src-tauri/Cargo.toml", "[package]\nname = \"wow-companion\"\nversion = \"0.7.9\"\n[dependencies]\nfoo = { version = \"1\" }\n")
	write(t, root, "overlay/src-tauri/Cargo.lock", "[[package]]\nname = \"other\"\nversion = \"0.7.9\"\n\n[[package]]\nname = \"wow-companion\"\nversion = \"0.7.9\"\n")
	return root
}

func read(t *testing.T, root, name string) string {
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPrepareTwoFragments(t *testing.T) {
	root := fixture(t)
	write(t, root, "changes/ADV-2.md", "- Second.\n")
	write(t, root, "changes/ADV-1.md", "- First.\n- First b.\n")
	write(t, root, "changes/README.md", "docs\n")
	v, err := prepare(root, "")
	if err != nil || v != "0.7.10" {
		t.Fatalf("prepare = %q, %v", v, err)
	}
	want := "# Changelog\n\n## 0.7.10\n\n- First.\n- First b.\n- Second.\n\n## 0.7.9\n\n- Old.\n"
	if got := read(t, root, "CHANGELOG.md"); got != want {
		t.Errorf("CHANGELOG:\n%s", got)
	}
	if read(t, root, "VERSION") != "0.7.10\n" {
		t.Error("VERSION not bumped")
	}
	if !strings.Contains(read(t, root, "overlay/src-tauri/tauri.conf.json"), `"version": "0.7.10"`) ||
		!strings.Contains(read(t, root, "overlay/src-tauri/Cargo.toml"), "version = \"0.7.10\"\n[dep") ||
		!strings.Contains(read(t, root, "overlay/src-tauri/Cargo.toml"), `foo = { version = "1" }`) {
		t.Error("version fields not bumped")
	}
	lock := read(t, root, "overlay/src-tauri/Cargo.lock")
	if !strings.Contains(lock, "name = \"other\"\nversion = \"0.7.9\"") || !strings.Contains(lock, "name = \"wow-companion\"\nversion = \"0.7.10\"") {
		t.Errorf("lock:\n%s", lock)
	}
	for _, n := range []string{"ADV-1.md", "ADV-2.md"} {
		if _, err := os.Stat(filepath.Join(root, "changes", n)); err == nil {
			t.Errorf("%s not deleted", n)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "changes", "README.md")); err != nil {
		t.Error("README deleted")
	}
}

func TestPrepareMinorAndRefusals(t *testing.T) {
	root := fixture(t)
	if _, err := prepare(root, ""); err == nil {
		t.Error("no fragments should be refused")
	}
	write(t, root, "changes/ADV-1.md", "- One.\n")
	if _, err := prepare(root, "major"); err == nil {
		t.Error("bad bump accepted")
	}
	if read(t, root, "VERSION") != "0.7.9\n" {
		t.Error("refusal changed VERSION")
	}
	if v, err := prepare(root, "minor"); err != nil || v != "0.8.0" {
		t.Errorf("minor = %q, %v", v, err)
	}
}

func TestViolation(t *testing.T) {
	cases := []struct {
		files []string
		bad   bool
	}{
		{[]string{"internal/a.go", "changes/ADV-1.md"}, false},
		{[]string{"VERSION", "internal/a.go"}, true},
		{[]string{"CHANGELOG.md"}, true},
		{[]string{"VERSION", "CHANGELOG.md", "overlay/src-tauri/Cargo.toml", "overlay/src-tauri/tauri.conf.json", "overlay/src-tauri/Cargo.lock", "changes/ADV-1.md"}, false},
		{[]string{"overlay/src-tauri/Cargo.lock", "internal/a.go"}, false},
	}
	for _, c := range cases {
		if got := violation(c.files) != ""; got != c.bad {
			t.Errorf("violation(%v) = %v", c.files, got)
		}
	}
}

func TestFragmentLengthRule(t *testing.T) {
	ok := "- " + strings.Repeat("a", 100) + "\n"
	long := "- " + strings.Repeat("a", 101) + "\n"
	root := fixture(t)
	write(t, root, "changes/ADV-1.md", ok)
	if err := checkFragments(root); err != nil {
		t.Errorf("100 characters refused: %v", err)
	}
	write(t, root, "changes/ADV-2.md", long)
	err := checkFragments(root)
	if err == nil || !strings.Contains(err.Error(), "ADV-2.md") || strings.Contains(err.Error(), "ADV-1.md") {
		t.Errorf("101 characters: %v", err)
	}
	if _, err := prepare(root, ""); err == nil || !strings.Contains(err.Error(), "ADV-2.md") {
		t.Errorf("prepare accepted an over-long line: %v", err)
	}
	if read(t, root, "VERSION") != "0.7.9\n" {
		t.Error("refusal changed VERSION")
	}
}
