package server

import (
	"os"
	"path/filepath"
	"testing"
)

// The tests never read or write the player's real play-style store.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wowc-profiles")
	if err != nil {
		panic(err)
	}
	os.Setenv("WOWC_PROFILES", filepath.Join(dir, "profiles.json"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
