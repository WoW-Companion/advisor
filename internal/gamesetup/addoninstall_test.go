package gamesetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/release"
)

// fakeGame is a game folder with an installed addon at version v and the app's data files.
func fakeGame(t *testing.T, v string) (game, addons string) {
	t.Helper()
	game = t.TempDir()
	addons = filepath.Join(game, "Interface", "AddOns")
	write(t, filepath.Join(addons, "WoWCompanion", "WoWCompanion.toc"), "## Version: "+v+"\n")
	write(t, filepath.Join(addons, "WoWCompanion", "Old.lua"), "old "+v)
	write(t, filepath.Join(addons, "WoWCompanion_Digest", "WoWCompanion_Digest.toc"), "toc")
	write(t, filepath.Join(addons, "WoWCompanion_Digest", "Digest.lua"), "digest data")
	write(t, filepath.Join(addons, "WoWCompanion_Digest", "Plan.lua"), "plan data")
	write(t, filepath.Join(addons, "WoWCompanion_Answers", "Answers.lua"), "answers")
	write(t, filepath.Join(game, "WTF", "saved.lua"), "saved")
	return
}

func release2(t *testing.T, v string, extra map[string]string) (z, sums []byte) {
	files := map[string]string{
		"WoWCompanion/WoWCompanion.toc":               "## Version: " + v + "\n",
		"WoWCompanion/New.lua":                        "new " + v,
		"WoWCompanion_Digest/WoWCompanion_Digest.toc": "toc " + v,
	}
	for k, x := range extra {
		files[k] = x
	}
	z = makeZip(t, files)
	return z, sumsFor("WoWCompanion-"+v+".zip", z)
}

func readf(p string) string { b, _ := os.ReadFile(p); return string(b) }

func TestInstallKeepsDataAndRemovesOldFiles(t *testing.T) {
	game, addons := fakeGame(t, "1.0.0")
	z, sums := release2(t, "1.1.0", map[string]string{"WoWCompanion_Digest/Digest.lua": "placeholder", "WoWCompanion_Digest/Plan.lua": "placeholder"})
	if _, err := InstallVerified(z, sums, "WoWCompanion-1.1.0.zip", game, t.TempDir()); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		"WoWCompanion_Digest/Digest.lua": "digest data", "WoWCompanion_Digest/Plan.lua": "plan data",
		"WoWCompanion_Answers/Answers.lua": "answers", "WoWCompanion/New.lua": "new 1.1.0",
		"WoWCompanion_Digest/WoWCompanion_Digest.toc": "toc 1.1.0",
	} {
		if got := readf(filepath.Join(addons, filepath.FromSlash(p))); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	if _, err := os.Stat(filepath.Join(addons, "WoWCompanion", "Old.lua")); err == nil {
		t.Error("a file the new version no longer ships was kept")
	}
	if readf(filepath.Join(game, "WTF", "saved.lua")) != "saved" {
		t.Error("WTF changed")
	}
}

func TestInstallRefusesHashMismatch(t *testing.T) {
	game, addons := fakeGame(t, "1.0.0")
	z, _ := release2(t, "1.1.0", nil)
	bad := sumsFor("WoWCompanion-1.1.0.zip", []byte("other"))
	backups := t.TempDir()
	if _, err := InstallVerified(z, bad, "WoWCompanion-1.1.0.zip", game, backups); err == nil || !strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("mismatch accepted: %v", err)
	}
	if v, _ := InstalledAddonVersion(game); v != "1.0.0" || readf(filepath.Join(addons, "WoWCompanion", "Old.lua")) != "old 1.0.0" {
		t.Error("the install changed files after a refusal")
	}
	if len(Backups(backups)) != 0 {
		t.Error("a backup was made for a refused install")
	}
}

func TestGoBackRestoresAndKeepsThree(t *testing.T) {
	game, addons := fakeGame(t, "1.0.0")
	backups := t.TempDir()
	for _, v := range []string{"1.1.0", "1.2.0", "1.3.0", "1.4.0"} {
		z, sums := release2(t, v, nil)
		if _, err := InstallVerified(z, sums, "WoWCompanion-"+v+".zip", game, backups); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(Backups(backups), ","); got != "1.3.0,1.2.0,1.1.0" {
		t.Fatalf("kept %s", got)
	}
	if _, err := GoBack(game, backups, "1.2.0"); err != nil {
		t.Fatal(err)
	}
	if v, _ := InstalledAddonVersion(game); v != "1.2.0" {
		t.Errorf("installed %s after go-back", v)
	}
	if readf(filepath.Join(addons, "WoWCompanion", "New.lua")) != "new 1.2.0" || readf(filepath.Join(addons, "WoWCompanion_Digest", "Digest.lua")) != "digest data" {
		t.Error("go-back did not restore the version or lost the data file")
	}
	if _, err := GoBack(game, backups, "9.9.9"); err == nil {
		t.Error("a version that is not kept was accepted")
	}
	if _, err := GoBack(game, backups, "../x"); err == nil {
		t.Error("a path was accepted as a version")
	}
}

func TestMissingGameFolderSaysSo(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	z, sums := release2(t, "1.1.0", nil)
	if _, err := InstallVerified(z, sums, "WoWCompanion-1.1.0.zip", missing, ""); err == nil || err.Error() != NoGameMessage {
		t.Errorf("install: %v", err)
	}
	if _, err := GoBack(missing, t.TempDir(), "1.0.0"); err == nil || err.Error() != NoGameMessage {
		t.Errorf("go back: %v", err)
	}
	if st := StatusFor(Options{GameDir: missing}, t.TempDir(), false, time.Now()); st.Message != NoGameMessage {
		t.Errorf("status: %+v", st)
	}
}

func TestStatusSettleDelayAndPulled(t *testing.T) {
	game, _ := fakeGame(t, "1.0.0")
	now := time.Date(2030, 1, 10, 12, 0, 0, 0, time.UTC)
	rel := func(age time.Duration) *AddonRelease {
		return &AddonRelease{Version: "1.1.0", Published: now.Add(-age)}
	}
	tests := []struct {
		name      string
		o         Options
		immediate bool
		want      bool
		auto      string
	}{
		{"fresh release waits", Options{Latest: rel(2 * time.Hour)}, false, false, ""},
		{"settled release is offered", Options{Latest: rel(25 * time.Hour)}, false, true, "install"},
		{"right away skips the wait", Options{Latest: rel(time.Minute)}, true, true, "install"},
		{"installed version pulled goes back at once", Options{Latest: rel(time.Minute), Pulled: []release.Pull{{Version: "1.0.0", Reason: "bad", BackTo: "0.9.0"}}, Fallback: &AddonRelease{Version: "0.9.0"}}, false, true, "goback"},
	}
	for _, tc := range tests {
		tc.o.GameDir = game
		st := StatusFor(tc.o, t.TempDir(), tc.immediate, now)
		if st.Available != tc.want || st.Auto != tc.auto {
			t.Errorf("%s: %+v", tc.name, st)
		}
	}
}

func TestStatusNeedsNewerAddon(t *testing.T) {
	game, _ := fakeGame(t, "0.1.0")
	now := time.Now()
	st := StatusFor(Options{GameDir: game, Latest: &AddonRelease{Version: "1.0.0", Published: now}}, t.TempDir(), false, now)
	if !st.NeedsNewer || !st.Available || st.Latest != "1.0.0" {
		t.Errorf("an addon older than the minimum is offered the install at once: %+v", st)
	}
}
