package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func rel(v string, hoursAgo int) Rel {
	return Rel{Version: v, Published: t0.Add(-time.Duration(hoursAgo) * time.Hour)}
}

// The offer rule: newest, not pulled, not skipped, newer than running, 24 h settled unless "right away".
func TestPick(t *testing.T) {
	rels := []Rel{rel("0.7.56", 1), rel("0.7.55", 30), rel("0.7.54", 50), rel("0.7.53", 90), rel("0.7.52", 200)}
	pull := func(vs ...string) Pulls {
		var p Pulls
		for _, v := range vs {
			p.Pulled = append(p.Pulled, Pull{Version: v})
		}
		return p
	}
	for _, c := range []struct {
		name  string
		rels  []Rel
		pulls Pulls
		o     Options
		want  string
	}{
		{"settled newest wins over a fresh one", rels, Pulls{}, Options{Running: "0.7.53"}, "0.7.55"},
		{"right away takes the fresh one", rels, Pulls{}, Options{Running: "0.7.53", Immediate: true}, "0.7.56"},
		{"pulled is passed over", rels, pull("0.7.55"), Options{Running: "0.7.53"}, "0.7.54"},
		{"skipped is passed over", rels, Pulls{}, Options{Running: "0.7.53", Skipped: []string{"v0.7.55"}}, "0.7.54"},
		{"skipping the newest keeps an older one on offer", rels, Pulls{}, Options{Running: "0.7.53", Immediate: true, Skipped: []string{"0.7.56"}}, "0.7.55"},
		{"nothing newer than running", rels, Pulls{}, Options{Running: "0.7.55"}, ""},
		{"all newer ones still settling", []Rel{rel("0.7.56", 2)}, Pulls{}, Options{Running: "0.7.55"}, ""},
		{"exactly 24 h counts as settled", []Rel{rel("0.7.56", 24)}, Pulls{}, Options{Running: "0.7.55"}, "0.7.56"},
		{"every newer one pulled", rels, pull("0.7.54", "0.7.55", "0.7.56"), Options{Running: "0.7.53", Immediate: true}, ""},
		{"development build gets no offer", rels, Pulls{}, Options{Running: "dev"}, ""},
		{"no releases", nil, Pulls{}, Options{Running: "0.7.53"}, ""},
	} {
		c.o.Now = t0
		got, ok := Pick(c.rels, c.pulls, c.o)
		if (c.want == "") == ok || got.Version != c.want {
			t.Errorf("%s: got %q (%v), want %q", c.name, got.Version, ok, c.want)
		}
	}
}

func TestParsePullsFailuresPullNothing(t *testing.T) {
	for _, c := range []struct{ name, body string }{
		{"empty", ""}, {"not json", "<html>404</html>"}, {"wrong shape", `{"pulled": "all"}`}, {"truncated", `{"schema":1,"pulled":[{"version":"0.7.5`},
	} {
		if p := ParsePulls([]byte(c.body)); len(p.Pulled) != 0 {
			t.Errorf("%s: pulled %+v", c.name, p.Pulled)
		}
	}
	p := ParsePulls([]byte(`{"schema":1,"pulled":[{"version":"v0.7.52","reason":"crashes","back_to":"v0.7.51"},{"version":""}],"addon_pulled":[],"notice":null}`))
	if x, ok := p.Find("0.7.52"); len(p.Pulled) != 1 || !ok || x.Reason != "crashes" || x.BackTo != "0.7.51" {
		t.Errorf("parsed %+v", p)
	}
	if p := ParsePulls([]byte(`{"schema":1,"pulled":[],"addon_pulled":[],"notice":null}`)); len(p.Pulled) != 0 {
		t.Error("the shipped empty file pulls something")
	}
}

func TestBackToAndComputeWhenRunningIsPulled(t *testing.T) {
	rels := []Rel{rel("0.7.53", 10), rel("0.7.52", 50), rel("0.7.51", 90), rel("0.7.50", 130)}
	p := Pulls{Pulled: []Pull{{Version: "0.7.52", Reason: "it crashes", BackTo: "0.7.50"}, {Version: "0.7.53", Reason: "x"}}}
	if v, ok := BackTo("0.7.52", rels, p); !ok || v != "0.7.50" {
		t.Errorf("explicit back_to: %q", v)
	}
	if v, ok := BackTo("0.7.53", rels, p); !ok || v != "0.7.51" {
		t.Errorf("missing back_to falls to the newest older unpulled: %q", v)
	}
	p.Pulled[0].BackTo = "0.7.53" // itself pulled
	if v, _ := BackTo("0.7.52", rels, p); v != "0.7.51" {
		t.Errorf("a pulled back_to is not used: %q", v)
	}
	st := Compute(rels, p, Options{Running: "0.7.52", Now: t0})
	if st.Pulled == nil || st.Pulled.Reason != "it crashes" || st.Pulled.BackTo != "0.7.51" || st.Available {
		t.Errorf("status %+v", st)
	}
}

func TestComputeVersionList(t *testing.T) {
	var rels []Rel
	for i, v := range []string{"0.7.60", "0.7.59", "0.7.58", "0.7.57", "0.7.56", "0.7.55", "0.7.54"} {
		rels = append(rels, rel(v, 10*(i+1)))
	}
	st := Compute(rels, Pulls{Pulled: []Pull{{Version: "0.7.58"}}}, Options{Running: "0.7.56", Now: t0})
	var got []string
	for _, it := range st.Versions {
		got = append(got, it.Version)
	}
	want := []string{"0.7.56", "0.7.60", "0.7.59", "0.7.57", "0.7.55", "0.7.54"}
	if len(got) != len(want) || !st.Versions[0].Current || st.Versions[0].Date == "" {
		t.Fatalf("list %v (%+v)", got, st.Versions)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("list %v want %v", got, want)
		}
	}
}

func TestSourceReadsAndKeepsTheAnswer(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/pulls" {
			http.NotFound(w, r) // an unreadable pull file pulls nothing
			return
		}
		hits++
		w.Write([]byte(`[{"tag_name":"v0.7.9","draft":false,"published_at":"2026-11-20T00:00:00Z","body":"notes","assets":[{"name":"WoW-Companion-Setup-0.7.9-x64.exe"}]},
		 {"tag_name":"v0.7.10","draft":true,"published_at":"2026-11-21T00:00:00Z","assets":[{"name":"WoW-Companion-Setup-0.7.10-x64.exe"}]},
		 {"tag_name":"v0.7.8","draft":false,"published_at":"2026-11-19T00:00:00Z","assets":[]}]`))
	}))
	defer srv.Close()
	s := &Source{Client: srv.Client(), ListURL: srv.URL + "/list", PullsURL: srv.URL + "/pulls"}
	for i := 0; i < 2; i++ {
		rels, pulls, err := s.Get(context.Background(), false)
		if err != nil || len(rels) != 1 || rels[0].Version != "0.7.9" || rels[0].Body != "notes" || len(pulls.Pulled) != 0 {
			t.Fatalf("%+v %+v %v", rels, pulls, err)
		}
	}
	if hits != 1 {
		t.Errorf("the second call should reuse the kept answer, hits = %d", hits)
	}
}
