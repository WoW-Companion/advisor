package companion

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/digest"
)

// TestMain makes the test binary double as the fake agent CLI. "stream" prints Claude-style
// stream-json chunks with delays and records when it finished; "text" prints a plain answer.
func TestMain(m *testing.M) {
	switch os.Getenv("FAKE_AGENT") {
	case "":
		os.Exit(m.Run())
	case "stream":
		for _, c := range []string{"Hel", "lo ", "there"} {
			fmt.Printf(`{"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":%q}}}`+"\n", c)
			time.Sleep(150 * time.Millisecond)
		}
		os.WriteFile(os.Getenv("FAKE_MARK"), nil, 0o644)
		fmt.Println(`{"type":"result","is_error":false,"result":"Hello there"}`)
	case "text":
		fmt.Printf("Plain answer to: %s\n", os.Getenv("WOWC_QUESTION"))
	}
	os.Exit(0)
}

const capture = `ForeverScribeDB = { chars = {
  ["R:Old"] = { snapshot = { identity = { name = "Old", realm = "R", class = "PRIEST" }, level = 10, capturedAt = 100 } },
  ["R:New"] = { snapshot = { identity = { name = "New", realm = "R", class = "ROGUE" }, level = 20, capturedAt = 200 } },
} }
`

const addonVars = `WoWCompanionDB = { asks = {
  { id = "9-1", asked_at = 5, character = "R:New", text = "What now from the game?" },
} }
`

type env struct {
	base, dir, mark string
	srv             *Server
	poll            time.Duration
}

// start serves a companion on a free local port with the fake agent in mode.
func start(t *testing.T, mode string, stream bool) *env {
	t.Helper()
	dir := t.TempDir()
	e := &env{dir: dir, mark: filepath.Join(dir, "finished")}
	t.Setenv("FAKE_AGENT", mode)
	t.Setenv("FAKE_MARK", e.mark)
	write(t, filepath.Join(dir, "ForeverScribe.lua"), capture)
	write(t, filepath.Join(dir, "WoWCompanion.lua"), "WoWCompanionDB = { asks = {} }\n")
	e.srv = New(Config{
		ForeverScribe: filepath.Join(dir, "ForeverScribe.lua"), Stream: stream, Poll: 20 * time.Millisecond,
		Ask: ask.Config{Command: []string{os.Args[0]}, Timeout: 5 * time.Second, SavedVariables: filepath.Join(dir, "WoWCompanion.lua"), AnswersPath: filepath.Join(dir, "Answers.lua")},
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.srv.Serve(ctx, ln); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	e.base = "http://" + ln.Addr().String()
	for e.srv.port == "" { // Serve sets it before it accepts
		time.Sleep(time.Millisecond)
	}
	return e
}

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// do sends a request with the token cookie unless hdr says otherwise.
func (e *env) do(t *testing.T, method, path, body string, hdr map[string]string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, e.base+path, strings.NewReader(body))
	req.AddCookie(&http.Cookie{Name: e.srv.cookieName(), Value: e.srv.Token})
	for k, v := range hdr {
		switch {
		case k == "Host":
			req.Host = v
		case k == "Cookie" && v == "":
			req.Header.Del("Cookie")
		case k == "Cookie":
			req.Header.Set("Cookie", v)
		default:
			req.Header.Set(k, v)
		}
	}
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

type sse struct{ name, data string }

// events reads an SSE body into a channel until it closes.
func events(r io.Reader) <-chan sse {
	ch := make(chan sse, 64)
	go func() {
		defer close(ch)
		sc := bufio.NewScanner(r)
		var cur sse
		for sc.Scan() {
			switch line := sc.Text(); {
			case strings.HasPrefix(line, "event: "):
				cur.name = line[7:]
			case strings.HasPrefix(line, "data: "):
				cur.data = line[6:]
			case line == "" && cur.name != "":
				ch <- cur
				cur = sse{}
			}
		}
	}()
	return ch
}

func next(t *testing.T, ch <-chan sse) sse {
	t.Helper()
	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("stream ended early")
		}
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for an event")
	}
	return sse{}
}

func (e *env) ask(t *testing.T, text string) string {
	t.Helper()
	body, _ := json.Marshal(askRequest{Character: "R:New", Text: text})
	resp := e.do(t, "POST", "/api/ask", string(body), nil)
	var out struct{ ID string }
	if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&out) != nil {
		t.Fatalf("ask: status %d", resp.StatusCode)
	}
	return out.ID
}

func TestRejects(t *testing.T) {
	e := start(t, "text", false)
	port := e.srv.port
	for _, tc := range []struct {
		name, method, path string
		hdr                map[string]string
		want               int
	}{
		{"valid", "GET", "/api/state", nil, 200},
		{"localhost host", "GET", "/api/state", map[string]string{"Host": "localhost:" + port}, 200},
		{"foreign host", "GET", "/api/state", map[string]string{"Host": "evil.example:" + port}, 403},
		{"rebinding host", "GET", "/api/state", map[string]string{"Host": "127.0.0.1.evil.example:" + port}, 403},
		{"wrong port", "GET", "/api/state", map[string]string{"Host": "127.0.0.1:1"}, 403},
		{"foreign origin", "POST", "/api/ask", map[string]string{"Origin": "http://evil.example"}, 403},
		{"own origin", "GET", "/api/state", map[string]string{"Origin": "http://127.0.0.1:" + port}, 200},
		{"no cookie", "GET", "/api/state", map[string]string{"Cookie": ""}, 403},
		{"wrong cookie", "GET", "/api/state", map[string]string{"Cookie": "wowc_token_" + port + "=nope"}, 403},
		{"page without cookie", "GET", "/", map[string]string{"Cookie": ""}, 403},
		{"wrong token in url", "GET", "/?token=nope", map[string]string{"Cookie": ""}, 403},
		{"stream without cookie", "GET", "/api/stream/t1", map[string]string{"Cookie": ""}, 403},
	} {
		if got := e.do(t, tc.method, tc.path, "{}", tc.hdr).StatusCode; got != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, got, tc.want)
		}
	}
	resp := e.do(t, "GET", "/?token="+e.srv.Token, "", map[string]string{"Cookie": ""})
	var set string
	for _, c := range resp.Cookies() {
		set += c.Name + "=" + c.Value
	}
	if resp.StatusCode != 303 || set != e.srv.cookieName()+"="+e.srv.Token || resp.Header.Get("Location") != "/" {
		t.Errorf("token url: %d cookie %q location %q", resp.StatusCode, set, resp.Header.Get("Location"))
	}
	if page := e.do(t, "GET", "/", "", nil); page.StatusCode != 200 || !strings.Contains(page.Header.Get("Content-Type"), "text/html") {
		t.Errorf("page: %d %s", page.StatusCode, page.Header.Get("Content-Type"))
	}
}

func TestAskLimits(t *testing.T) {
	e := start(t, "text", false)
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"empty", `{"text":"  "}`, 400},
		{"not json", `hello`, 400},
		{"body over the cap", `{"text":"` + strings.Repeat("a", MaxBody) + `"}`, 400},
		{"question over the cap", `{"text":"` + strings.Repeat("é", MaxQuestion+1) + `"}`, 413},
	} {
		if got := e.do(t, "POST", "/api/ask", tc.body, nil).StatusCode; got != tc.want {
			t.Errorf("%s: status %d, want %d", tc.name, got, tc.want)
		}
	}
}

// The end-to-end smoke test: the first chunk reaches the client before the agent finishes, chunks
// arrive in order, one answer at a time per client, and the finished turn feeds the next prompt.
func TestStreamEndToEnd(t *testing.T) {
	e := start(t, "stream", true)
	id := e.ask(t, "first?")
	if got := e.do(t, "POST", "/api/ask", `{"text":"second"}`, nil).StatusCode; got != 429 {
		t.Errorf("a second answer in flight: status %d, want 429", got)
	}
	resp := e.do(t, "GET", "/api/stream/"+id, "", nil)
	ch := events(resp.Body)
	first := next(t, ch)
	if _, err := os.Stat(e.mark); err == nil {
		t.Error("the agent had already finished when the first chunk arrived")
	}
	var got []string
	for ev := first; ; ev = next(t, ch) {
		if ev.name == "done" {
			var d Entry
			json.Unmarshal([]byte(ev.data), &d)
			if d.Answer != "Hello there" || !d.Streamed || d.Status != "done" {
				t.Errorf("done = %+v", d)
			}
			break
		}
		var c struct{ Text string }
		json.Unmarshal([]byte(ev.data), &c)
		got = append(got, c.Text)
	}
	if strings.Join(got, "|") != "Hel|lo |there" {
		t.Errorf("chunks = %v", got)
	}
	if _, err := os.Stat(e.mark); err != nil {
		t.Error("done arrived before the agent finished")
	}
	// A stream opened after the end replays the same events.
	if ev := next(t, events(e.do(t, "GET", "/api/stream/"+id, "", nil).Body)); ev.name != "chunk" {
		t.Errorf("replay starts with %q", ev.name)
	}
	if h := e.srv.snapshot().History; len(h) != 1 || h[0].Question != "first?" {
		t.Errorf("history = %+v", h)
	}
}

func TestNotStreamedFallback(t *testing.T) {
	e := start(t, "text", false)
	ch := events(e.do(t, "GET", "/api/stream/"+e.ask(t, "plain?"), "", nil).Body)
	ev := next(t, ch)
	var d Entry
	json.Unmarshal([]byte(ev.data), &d)
	if ev.name != "done" || d.Streamed || d.Status != "done" || !strings.HasPrefix(d.Answer, "Plain answer to: plain?") {
		t.Errorf("event %s %+v", ev.name, d)
	}
}

func TestGameDataChangeEvent(t *testing.T) {
	e := start(t, "text", false)
	ch := events(e.do(t, "GET", "/api/events", "", nil).Body)
	time.Sleep(50 * time.Millisecond) // let the poller record the first stamp
	// Renamed into place, so the poller never reads a half-written file under load.
	tmp := filepath.Join(e.dir, "ForeverScribe.tmp")
	write(t, tmp, strings.Replace(capture, "capturedAt = 200", "capturedAt = 999, extra = 1", 1))
	if err := os.Rename(tmp, filepath.Join(e.dir, "ForeverScribe.lua")); err != nil {
		t.Fatal(err)
	}
	ev := next(t, ch)
	var st State
	json.Unmarshal([]byte(ev.data), &st)
	if ev.name != "game" || st.CapturedAt != 999 || st.Default != "R:New" || len(st.Characters) != 2 {
		t.Errorf("event %s %+v", ev.name, st)
	}
}

func TestInGameQuestionIsAnswered(t *testing.T) {
	e := start(t, "text", false)
	write(t, filepath.Join(e.dir, "WoWCompanion.lua"), addonVars)
	deadline := time.Now().Add(5 * time.Second)
	for {
		h := e.srv.snapshot().History
		if len(h) == 1 && h[0].Status == "done" {
			if h[0].Source != "game" || !strings.Contains(h[0].Answer, "What now from the game?") {
				t.Errorf("entry = %+v", h[0])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no answer; history %+v", h)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if a := ask.ReadAnswers(filepath.Join(e.dir, "Answers.lua"))["9-1"]; !strings.Contains(a.Text, "What now from the game?") {
		t.Errorf("Answers.lua = %+v", a)
	}
}

func TestHistoryFor(t *testing.T) {
	long := strings.Repeat("a", 5000)
	mk := func(n int, ans string) []Entry {
		var es []Entry
		for i := 0; i < n; i++ {
			es = append(es, Entry{Character: "c", Question: fmt.Sprintf("q%d\n", i) + strings.Repeat("z", min(len(ans)/2, 450)), Answer: ans, Status: "done"})
		}
		return es
	}
	for _, tc := range []struct {
		name       string
		web        []Entry
		wantQs     []string
		wantEmpty  bool
		maxRunes   int
		notContain string
	}{
		{"none", nil, nil, true, 0, ""},
		{"other character and failures skipped", append(mk(1, "x"), Entry{Character: "d", Question: "other", Status: "done"}, Entry{Character: "c", Question: "bad", Status: "error"}), []string{"q0"}, false, 0, "other"},
		{"turn cap keeps the newest", mk(10, "x"), []string{"q4", "q5", "q6", "q7", "q8", "q9"}, false, 0, "q3"},
		{"answers cut", mk(1, long), []string{"q0"}, false, 1300, ""},
		{"size cap drops the oldest", mk(6, strings.Repeat("b", 590)), []string{"q5"}, false, HistoryChars + 200, "q0"},
	} {
		s := &Server{web: tc.web}
		got := s.historyFor("c")
		if (got == "") != tc.wantEmpty {
			t.Errorf("%s: history %q", tc.name, got)
		}
		for _, q := range tc.wantQs {
			if !bytes.Contains([]byte(got), []byte("Q: "+q)) {
				t.Errorf("%s: missing %s", tc.name, q)
			}
		}
		if tc.notContain != "" && strings.Contains(got, tc.notContain) {
			t.Errorf("%s: contains %s", tc.name, tc.notContain)
		}
		if tc.maxRunes > 0 && len([]rune(got)) > tc.maxRunes {
			t.Errorf("%s: %d runes", tc.name, len([]rune(got)))
		}
	}
}

// TestAppMode: with App set (the --handshake sidecar) the app's webview origins pass, the token
// comes in a header or the query, preflights are answered; without it none of that holds.
func TestAppMode(t *testing.T) {
	e := start(t, "text", false)
	tok := map[string]string{"Cookie": "", TokenHeader: e.srv.Token}
	with := func(h map[string]string, kv ...string) map[string]string {
		out := map[string]string{}
		for k, v := range h {
			out[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			out[kv[i]] = kv[i+1]
		}
		return out
	}
	type tc struct {
		name, method, path string
		hdr                map[string]string
		want               int
	}
	cases := []tc{
		{"app origin, header token", "GET", "/api/state", with(tok, "Origin", "http://tauri.localhost"), 200},
		{"https app origin", "GET", "/api/state", with(tok, "Origin", "https://tauri.localhost"), 200},
		{"tauri scheme origin", "GET", "/api/state", with(tok, "Origin", "tauri://localhost"), 200},
		{"query token", "GET", "/api/state?token=" + e.srv.Token, map[string]string{"Cookie": "", "Origin": "http://tauri.localhost"}, 200},
		{"app origin, no token", "GET", "/api/state", map[string]string{"Cookie": "", "Origin": "http://tauri.localhost"}, 403},
		{"wrong header token", "GET", "/api/state", map[string]string{"Cookie": "", TokenHeader: "nope"}, 403},
		{"preflight", "OPTIONS", "/api/ask", map[string]string{"Cookie": "", "Origin": "http://tauri.localhost"}, 204},
		{"foreign origin still refused", "GET", "/api/state", with(tok, "Origin", "http://evil.example"), 403},
		{"foreign tauri-like origin", "GET", "/api/state", with(tok, "Origin", "http://tauri.localhost.evil.example"), 403},
		{"query token off /api", "GET", "/?token=" + e.srv.Token, map[string]string{"Cookie": ""}, 303},
	}
	e.srv.cfg.App = true
	for _, c := range cases {
		if got := e.do(t, c.method, c.path, "", c.hdr).StatusCode; got != c.want {
			t.Errorf("app: %s: status %d, want %d", c.name, got, c.want)
		}
	}
	if h := e.do(t, "GET", "/api/state", "", with(tok, "Origin", "http://tauri.localhost")).Header.Get("Access-Control-Allow-Origin"); h != "http://tauri.localhost" {
		t.Errorf("Access-Control-Allow-Origin = %q", h)
	}
	e.srv.cfg.App = false
	for _, c := range cases[:5] {
		if got := e.do(t, c.method, c.path, "", c.hdr).StatusCode; got != 403 {
			t.Errorf("not app: %s: status %d, want 403", c.name, got)
		}
	}
}

func TestNextQuestsCapped(t *testing.T) {
	if got := nextQuests(character.Character{}, character.File{}, nil); len(got) > NextQuests {
		t.Errorf("%d lines", len(got))
	}
}

// TestNoAgent: a companion with no agent still serves the character data, and asking says why it cannot.
func TestNoAgent(t *testing.T) {
	e := start(t, "text", false)
	e.srv.cfg.Ask.Command = nil
	var st State
	if err := json.NewDecoder(e.do(t, "GET", "/api/state", "", nil).Body).Decode(&st); err != nil || len(st.Characters) != 2 {
		t.Fatalf("state: %+v, %v", st, err)
	}
	resp := e.do(t, "POST", "/api/ask", `{"character":"R:New","text":"hi"}`, nil)
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusConflict || strings.TrimSpace(string(body)) != ErrNoAgent {
		t.Errorf("ask: %d %q", resp.StatusCode, body)
	}
}

// digestEnv lays out <game>/WTF/Account/A/SavedVariables/ForeverScribe.lua, with the digest slot
// folder when withAddon is set, and returns a companion (not serving) that writes into it.
func digestEnv(t *testing.T, withAddon, on bool) (*Server, string, string) {
	t.Helper()
	game := t.TempDir()
	sv := filepath.Join(game, "WTF", "Account", "A", "SavedVariables")
	if err := os.MkdirAll(sv, 0o755); err != nil {
		t.Fatal(err)
	}
	slot := filepath.Join(game, "Interface", "AddOns", digest.SlotAddon)
	if withAddon {
		if err := os.MkdirAll(slot, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fs := filepath.Join(sv, "ForeverScribe.lua")
	write(t, fs, capture)
	return New(Config{ForeverScribe: fs, AutoDigest: on}), fs, filepath.Join(slot, "Digest.lua")
}

func TestAutoDigest(t *testing.T) {
	s, fs, out := digestEnv(t, true, true)
	if s.digest.Status != DigestOn {
		t.Fatalf("status %q", s.digest.Status)
	}
	s.writeDigest()
	first, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(first), "New") {
		t.Fatalf("digest not written: %v %q", err, first)
	}
	if s.digest.UpdatedAt == 0 {
		t.Error("no update time")
	}
	// A rebuild of the same advice, even at a later second, leaves the file alone.
	older := regexp.MustCompile(`generated = \d+,`).ReplaceAllString(string(first), "generated = 1,")
	write(t, out, older)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(out, old, old)
	s.writeDigest()
	if st, _ := os.Stat(out); st.ModTime().After(old.Add(time.Minute)) {
		t.Error("identical digest was rewritten")
	}
	// A real change writes.
	write(t, fs, strings.Replace(capture, `name = "New"`, `name = "Newer"`, 1))
	s.writeDigest()
	if got, _ := os.ReadFile(out); string(got) == string(first) {
		t.Error("changed capture did not change the digest")
	}
}

func TestAutoDigestSkips(t *testing.T) {
	s, _, out := digestEnv(t, false, true)
	if s.digest.Status != DigestNoAddon {
		t.Fatalf("status %q, want addon not found", s.digest.Status)
	}
	s.writeDigest()
	if _, err := os.Stat(out); err == nil || s.snapshot().Digest.Status != DigestNoAddon {
		t.Errorf("wrote without the addon (%v) or lost the status %q", err, s.snapshot().Digest.Status)
	}
	s, _, out = digestEnv(t, true, false)
	s.writeDigest()
	if _, err := os.Stat(out); err == nil {
		t.Error("wrote with the setting off")
	}
	s.SetAutoDigest(true)
	s.writeDigest()
	if _, err := os.Stat(out); err != nil {
		t.Errorf("not written after turning it on: %v", err)
	}
}

func TestAutoDigestOnSave(t *testing.T) {
	s, fs, out := digestEnv(t, true, true)
	s.cfg.Poll, s.cfg.Debounce = 20*time.Millisecond, 100*time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.watch(ctx)
	time.Sleep(60 * time.Millisecond)
	write(t, fs, capture+"-- saved\n")
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(out); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("a save did not write the digest")
}
