package gamesetup

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/release"
)

func TestPlanAddon(t *testing.T) {
	pull := func(v, back string) release.Pull { return release.Pull{Version: v, Reason: "crashes", BackTo: back} }
	tests := []struct {
		name         string
		have, latest string
		pulls        []release.Pull
		want         AddonOffer
	}{
		{"nothing pulled, newer exists", "1.0.0", "1.1.0", nil, AddonOffer{Version: "1.1.0"}},
		{"up to date", "1.1.0", "1.1.0", nil, AddonOffer{}},
		{"fresh install", "", "1.1.0", nil, AddonOffer{Version: "1.1.0"}},
		{"newest is pulled: offer its back_to", "1.0.0", "1.2.0", []release.Pull{pull("1.2.0", "1.1.0")}, AddonOffer{Version: "1.1.0"}},
		{"newest is pulled and nothing newer is safe", "1.1.0", "1.2.0", []release.Pull{pull("1.2.0", "1.1.0")}, AddonOffer{}},
		{"newest is pulled, no back_to", "1.0.0", "1.2.0", []release.Pull{pull("1.2.0", "")}, AddonOffer{}},
		{"back_to is pulled too: follows the chain", "1.0.0", "1.3.0", []release.Pull{pull("1.3.0", "1.2.0"), pull("1.2.0", "1.1.0")}, AddonOffer{Version: "1.1.0"}},
		{"a loop ends with no offer", "", "1.3.0", []release.Pull{pull("1.3.0", "1.2.0"), pull("1.2.0", "1.3.0")}, AddonOffer{}},
		{"fresh install, newest pulled", "", "1.2.0", []release.Pull{pull("1.2.0", "1.1.0")}, AddonOffer{Version: "1.1.0"}},
		{"installed is pulled: reinstall back_to", "1.2.0", "1.2.0", []release.Pull{pull("1.2.0", "1.1.0")}, AddonOffer{Version: "1.1.0", Reinstall: true, Reason: "crashes", Pulled: true}},
		{"installed is pulled, no back_to: the newest safe", "1.1.0", "1.2.0", []release.Pull{pull("1.1.0", "")}, AddonOffer{Version: "1.2.0", Reinstall: true, Reason: "crashes", Pulled: true}},
		{"installed is pulled, nothing safe", "1.2.0", "1.2.0", []release.Pull{pull("1.2.0", "")}, AddonOffer{Reason: "crashes", Pulled: true}},
		{"a v prefix is the same version", "v1.2.0", "1.2.0", []release.Pull{pull("v1.2.0", "v1.1.0")}, AddonOffer{Version: "1.1.0", Reinstall: true, Reason: "crashes", Pulled: true}},
	}
	for _, tc := range tests {
		if got := PlanAddon(tc.have, tc.latest, tc.pulls); got != tc.want {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func mkzip(t *testing.T, files ...string) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for _, f := range files {
		w, _ := zw.Create(f)
		w.Write([]byte("x"))
	}
	zw.Close()
	return b.Bytes()
}

func TestNewFilesDecidesReloadOrRestart(t *testing.T) {
	addons := t.TempDir()
	os.MkdirAll(filepath.Join(addons, "WoWCompanion"), 0o755)
	os.WriteFile(filepath.Join(addons, "WoWCompanion", "Core.lua"), []byte("old"), 0o644)
	same, err := NewFiles(mkzip(t, "WoWCompanion/Core.lua"), addons)
	if err != nil || len(same) != 0 || !strings.Contains(ReloadNote(same), "/reload") || strings.Contains(ReloadNote(same), "quit") {
		t.Errorf("changed files only: %v %v %q", same, err, ReloadNote(same))
	}
	added, err := NewFiles(mkzip(t, "WoWCompanion/Core.lua", "WoWCompanion/Extra.lua"), addons)
	if err != nil || len(added) != 1 || added[0] != "WoWCompanion/Extra.lua" || !strings.Contains(ReloadNote(added), "quit the game fully") {
		t.Errorf("a new file: %v %v %q", added, err, ReloadNote(added))
	}
	if _, err := NewFiles(mkzip(t, "../evil.lua"), addons); err == nil {
		t.Error("an unsafe entry must be refused")
	}
}

func TestAddonItemNeverOffersAPulledVersion(t *testing.T) {
	game := t.TempDir()
	os.MkdirAll(filepath.Join(game, "Interface", "AddOns", "WoWCompanion"), 0o755)
	os.WriteFile(filepath.Join(game, "Interface", "AddOns", "WoWCompanion", "WoWCompanion.toc"), []byte("## Version: 1.2.0\n"), 0o644)
	bad := []release.Pull{{Version: "1.2.0", Reason: "crashes on login", BackTo: "1.1.0"}}
	o := Options{GameDir: game, Latest: &AddonRelease{Version: "1.2.0"}, Pulled: bad, Fallback: &AddonRelease{Version: "1.1.0"}}
	it := addonItem(game, o)
	if it.Action != ActUpdateAddon || it.Label != "Reinstall 1.1.0" || !strings.Contains(it.Detail, "crashes on login") {
		t.Errorf("installed pulled version: %+v", it)
	}
	o.Fallback = nil
	if it := addonItem(game, o); it.Action != "" || !strings.Contains(it.Detail, "known problem") {
		t.Errorf("no safe version known: %+v", it)
	}
}
