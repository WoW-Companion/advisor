package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestHandshakeLine(t *testing.T) {
	for _, tc := range []struct{ agent, source, wantSource string }{
		{"claude", "auto", "auto"},
		{"codex", "flag", "flag"},
		{"", "", "none"},
	} {
		var got map[string]string
		if err := json.Unmarshal([]byte(handshakeLine(4321, "abc", tc.agent, tc.source, "/bin/"+tc.agent, "0.2.0")), &got); err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"url": "http://127.0.0.1:4321", "token": "abc", "agent": tc.agent, "agent_source": tc.wantSource, "agent_path": "/bin/" + tc.agent, "version": "0.2.0"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %v, want %v", got, want)
		}
	}
}

func TestSetConfigAgent(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "ask.json")
	read := func() map[string]any {
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	if err := setConfigAgent(p, "codex"); err != nil || read()["agent"] != "codex" {
		t.Fatalf("create: %v %v", err, read())
	}
	os.WriteFile(p, []byte(`{"agent":"codex","other":1}`), 0o600)
	if err := setConfigAgent(p, "my cli"); err != nil || read()["agent"] != "my cli" || read()["other"] != float64(1) {
		t.Fatalf("update: %v %v", err, read())
	}
	if err := setConfigAgent(p, ""); err != nil || read()["agent"] != nil || read()["other"] != float64(1) {
		t.Fatalf("remove: %v %v", err, read())
	}
}

func TestListenFromAnyPort(t *testing.T) {
	ln, err := listenFrom("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if ln.Addr().(*net.TCPAddr).Port == 0 {
		t.Error("no port chosen")
	}
}

// ask writes Answers.lua only behind --answers-file; without it the error points at the overlay.
func TestAskNeedsAnswersFile(t *testing.T) {
	err := askCmd(context.Background(), nil, io.Discard, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--answers-file") || !strings.Contains(err.Error(), "overlay") {
		t.Errorf("err = %v", err)
	}
}
