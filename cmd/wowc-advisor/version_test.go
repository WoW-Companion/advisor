package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/server"
)

func TestVersionAndUpdateLines(t *testing.T) {
	oldV, oldC, oldS := version, commit, server.Version
	t.Cleanup(func() { version, commit, server.Version = oldV, oldC, oldS })

	server.Version, commit = "0.1.0", ""
	if got := versionLine(); got != "wowc-advisor 0.1.0" {
		t.Errorf("versionLine = %q", got)
	}
	commit = "abc1234"
	if got := versionLine(); got != "wowc-advisor 0.1.0 (abc1234)" {
		t.Errorf("versionLine = %q", got)
	}

	latest := "v0.2.0"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"tag_name":"` + latest + `"}`))
	}))
	defer srv.Close()
	for _, c := range []struct{ current, latest, want string }{
		{"0.1.0", "v0.2.0", "v0.2.0 is available"},
		{"0.2.0", "v0.2.0", "up to date"},
		{"dev", "v0.2.0", "development build"},
	} {
		server.Version, latest = c.current, c.latest
		if got := updateLine(context.Background(), srv.Client(), srv.URL); !strings.Contains(got, c.want) {
			t.Errorf("current %s: %q lacks %q", c.current, got, c.want)
		}
	}
	srv.Close()
	if got := updateLine(context.Background(), srv.Client(), srv.URL); !strings.Contains(got, "could not look up") {
		t.Errorf("offline: %q", got)
	}
}
