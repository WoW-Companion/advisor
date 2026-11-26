package companion

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// The state carries the addon version the last save recorded and when the file was written, which
// the overlay uses to clear its "/reload" line.
func TestStateGameSave(t *testing.T) {
	sv := filepath.Join(t.TempDir(), "ForeverScribe.lua")
	if err := os.WriteFile(sv, []byte(captureFor("71037")), 0o644); err != nil {
		t.Fatal(err)
	}
	at := time.Unix(1_700_000_000, 0)
	os.Chtimes(sv, at, at)
	s := New(Config{ForeverScribe: sv, AddonAPI: "http://127.0.0.1:1/x", App: true})
	s.reload()
	st := s.snapshot()
	if st.SavedAt != at.Unix() {
		t.Errorf("SavedAt = %d, want %d", st.SavedAt, at.Unix())
	}
	if st.GameAddon == "" {
		t.Errorf("GameAddon empty; the capture records addonVersion")
	}
}
