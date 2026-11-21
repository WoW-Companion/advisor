package main

import (
	"encoding/json"
	"net"
	"testing"
)

func TestHandshakeLine(t *testing.T) {
	var got struct{ URL, Token string }
	if err := json.Unmarshal([]byte(handshakeLine(4321, "abc")), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != "http://127.0.0.1:4321" || got.Token != "abc" {
		t.Errorf("got %+v", got)
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
