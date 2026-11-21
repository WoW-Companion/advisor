package svfind

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func put(t *testing.T, root, flavor, account string, age time.Duration) string {
	t.Helper()
	p := filepath.Join(root, flavor, "WTF", "Account", account, "SavedVariables", "ForeverScribe.lua")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-age)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestFind(t *testing.T) {
	t.Run("one match", func(t *testing.T) {
		root := t.TempDir()
		want := put(t, root, "_forever_", "TESTACCOUNT", 0)
		res := Find([]string{root})
		if res.Path != want || len(res.Others) != 0 {
			t.Fatalf("got %+v", res)
		}
		if n := res.Note("WOWC_SAVEDVARIABLES"); !strings.Contains(n, want) || !strings.Contains(n, "WOWC_SAVEDVARIABLES") {
			t.Fatalf("note %q", n)
		}
	})
	t.Run("several accounts pick the newest", func(t *testing.T) {
		root := t.TempDir()
		old := put(t, root, "_forever_", "TESTACCOUNT", time.Hour)
		want := put(t, root, "_forever_", "TESTACCOUNT2", time.Minute)
		res := Find([]string{root})
		if res.Path != want || len(res.Others) != 1 || res.Others[0] != old {
			t.Fatalf("got %+v", res)
		}
		if n := res.Note("E"); !strings.Contains(n, old) {
			t.Fatalf("note %q does not list the other", n)
		}
	})
	t.Run("forever beats a newer retail", func(t *testing.T) {
		root := t.TempDir()
		want := put(t, root, "_forever_", "TESTACCOUNT", time.Hour)
		put(t, root, "_retail_", "TESTACCOUNT", 0)
		if res := Find([]string{root}); res.Path != want {
			t.Fatalf("got %+v", res)
		}
	})
	t.Run("falls back to classic era and retail", func(t *testing.T) {
		root := t.TempDir()
		want := put(t, root, "_classic_era_", "TESTACCOUNT", 0)
		put(t, root, "_retail_", "TESTACCOUNT", 0)
		if res := Find([]string{root}); res.Path != want {
			t.Fatalf("got %+v", res)
		}
	})
	t.Run("earlier root wins", func(t *testing.T) {
		a, b := t.TempDir(), t.TempDir()
		want := put(t, a, "_forever_", "TESTACCOUNT", time.Hour)
		put(t, b, "_forever_", "TESTACCOUNT", 0)
		if res := Find([]string{a, b}); res.Path != want {
			t.Fatalf("got %+v", res)
		}
	})
	t.Run("none names the searched paths", func(t *testing.T) {
		root := t.TempDir()
		res := Find([]string{root})
		if res.Path != "" || res.Note("E") != "" {
			t.Fatalf("got %+v", res)
		}
		if msg := res.NotFound("WOWC_SAVEDVARIABLES"); !strings.Contains(msg, root) || !strings.Contains(msg, "WOWC_SAVEDVARIABLES") {
			t.Fatalf("message %q", msg)
		}
	})
}

func TestRoots(t *testing.T) {
	has := func(root string) bool { return root == `D:\` || root == `C:\` }
	got := strings.Join(Roots("windows", has), "|")
	want := `C:\Program Files (x86)\World of Warcraft|C:\Program Files\World of Warcraft|` +
		`D:\Program Files (x86)\World of Warcraft|D:\Program Files\World of Warcraft`
	if got != want {
		t.Fatalf("got %s", got)
	}
	if r := Roots("darwin", nil); len(r) != 1 || r[0] != "/Applications/World of Warcraft" {
		t.Fatalf("darwin %v", r)
	}
	if r := Roots("linux", nil); len(r) != 0 {
		t.Fatalf("linux %v", r)
	}
}
