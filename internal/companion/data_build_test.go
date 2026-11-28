package companion

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func wowcCaptureFor(build string, at int) string {
	return fmt.Sprintf(`WoWCompanionDB = { characters = { ["R:New"] = { name = "New", realm = "R", class = "ROGUE", level = 20, capturedAt = %d, client = { version = "1.60.1", build = %q } } } }
`, at, build)
}

func TestDatasetSyncFollowsNewestCapture(t *testing.T) {
	rs := newRepublishServer(t) // publishes dataBuild (71037) only
	s, _, _ := digestEnv(t, true, true)
	s.cfg.Cache = rs.store
	dir := filepath.Dir(s.cfg.ForeverScribe)
	write(t, s.cfg.ForeverScribe, strings.Replace(captureFor("71000"), "capturedAt = 200", "capturedAt = 100", 1))
	write(t, filepath.Join(dir, "WoWCompanion.lua"), wowcCaptureFor("71037", 900))
	s.cfg.Now = func() time.Time { return time.Unix(1_000_000, 0) }

	if !s.checkDatasetUpdate(context.Background()) || s.datasetDir() != rs.store.Dir(dataBuild) {
		t.Fatalf("did not sync the WoW Companion build: %q", s.datasetDir())
	}
}

func TestDatasetNotDownloadedNote(t *testing.T) {
	rs := newRepublishServer(t)
	s, _, digestOut := digestEnv(t, true, true)
	s.cfg.Cache = rs.store
	write(t, s.cfg.ForeverScribe, captureFor("71037"))
	ctx := context.Background()
	if _, err := rs.store.Ensure(ctx, "71037", false); err != nil {
		t.Fatal(err)
	}
	s.data.dir = rs.store.Dir(dataBuild)
	clock := time.Unix(1_000_000, 0)
	s.cfg.Now = func() time.Time { return clock }

	// The game moves to 71139, which is not published and the server is offline: the note says the
	// check failed with its reason, never "No data".
	write(t, s.cfg.ForeverScribe, captureFor("71139"))
	rs.failing.Store(true)
	s.checkDatasetUpdate(ctx)
	b, err := os.ReadFile(digestOut)
	if err != nil || !strings.Contains(string(b), "Data for game build 71139 not downloaded yet (") || strings.Contains(string(b), "No data for") {
		t.Fatalf("offline note: %v\n%s", err, b)
	}
	if !strings.Contains(string(b), "using 71037") {
		t.Errorf("note lacks the build in use:\n%s", b)
	}
}
