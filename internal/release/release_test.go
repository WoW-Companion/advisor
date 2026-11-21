package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompare(t *testing.T) {
	for _, c := range []struct {
		a, b string
		cmp  int
		ok   bool
	}{
		{"v0.2.0", "0.1.0", 1, true},
		{"v0.1.0", "0.1.0", 0, true},
		{"0.1.0", "v0.1.1", -1, true},
		{"v0.10.0", "0.9.9", 1, true},
		{"v1.0", "1.0.0", 0, true},
		{"v0.1.0", "0.1.0-3-gabc-dirty", 0, true},
		{"v0.1.0", "dev", 0, false},
		{"v0.1.0", "", 0, false},
	} {
		if cmp, ok := Compare(c.a, c.b); cmp != c.cmp || ok != c.ok {
			t.Errorf("Compare(%q, %q) = %d, %v; want %d, %v", c.a, c.b, cmp, ok, c.cmp, c.ok)
		}
	}
}

func TestCheck(t *testing.T) {
	for _, c := range []struct {
		name    string
		status  int
		body    string
		current string
		want    Result
		wantErr bool
	}{
		{"newer", 200, `{"tag_name":"v0.2.0"}`, "0.1.0", Result{"v0.2.0", true, true}, false},
		{"same", 200, `{"tag_name":"v0.1.0"}`, "0.1.0", Result{"v0.1.0", false, true}, false},
		{"dev build", 200, `{"tag_name":"v0.1.0"}`, "dev", Result{"v0.1.0", false, false}, false},
		{"none yet", 404, `{}`, "0.1.0", Result{}, true},
		{"no tag", 200, `{}`, "0.1.0", Result{}, true},
		{"bad json", 200, `nope`, "0.1.0", Result{}, true},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "" {
				t.Error("the lookup must be unauthenticated")
			}
			w.WriteHeader(c.status)
			w.Write([]byte(c.body))
		}))
		got, err := Check(context.Background(), srv.Client(), srv.URL, c.current)
		srv.Close()
		if (err != nil) != c.wantErr || got != c.want {
			t.Errorf("%s: got %+v, %v; want %+v (err %v)", c.name, got, err, c.want, c.wantErr)
		}
	}
}
