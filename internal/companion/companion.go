// Package companion is the local web window beside the game: a chat page that streams the agent's
// answer, knows the current character, and refreshes the moment ForeverScribe saves. It reuses the
// ask package's prompt and agent runner, so the agent runs exactly as `ask` runs it.
package companion

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/server"
	"github.com/WoW-Companion/advisor/internal/store"
)

//go:embed page.html
var page []byte

// Limits. History is the context kept from earlier turns of the same character: at most
// HistoryTurns turns and HistoryChars characters, oldest dropped first, each answer cut to
// HistoryAnswerChars.
const (
	MaxBody            = 8 << 10
	MaxQuestion        = 1000
	HistoryTurns       = 4
	HistoryChars       = 4000
	HistoryAnswerChars = 600
	maxEntries         = 200
)

// ErrNoAgent is what asking says when no AI client is set up.
const ErrNoAgent = "No AI client set up yet: install Claude Code, Codex or Gemini CLI, or choose one in settings."

// Config is one companion's settings.
type Config struct {
	ForeverScribe string     // ForeverScribe.lua, polled for game data
	Ask           ask.Config // the agent; SavedVariables and AnswersPath serve in-game questions
	Stream        bool       // the agent command is Claude's and can print stream-json
	LAN           bool       // listening on all interfaces
	App           bool       // the desktop app's webview is the client: its origins pass, and the token may come in a header or query
	Dataset       string     // dataset directory, for each character's next quests (empty: none)
	Poll          time.Duration
	NoUsage       bool          // start with the usage line off
	AutoDigest    bool          // rewrite the in-game digest when ForeverScribe saves
	DigestPath    string        // Digest.lua; empty derives it from ForeverScribe
	Debounce      time.Duration // quiet time after a save before the digest is written (default 2s)
	GameDir       string        // the WoW folder the player chose in the setup checklist (empty: detect)
	HTTP          *http.Client  // for the addon lookup and install (nil: the default client)
	AddonAPI      string        // the addon's latest-release URL (empty: GitHub's)
}

// Digest statuses.
const (
	DigestOn      = "on"
	DigestOff     = "off"
	DigestNoAddon = "addon not found"
	DigestFailed  = "error"
)

// DigestState is the in-game digest's status: whether it is kept up to date, and when it last was.
type DigestState struct {
	Enabled   bool   `json:"enabled"`
	Status    string `json:"status"`
	UpdatedAt int64  `json:"updated_at,omitempty"` // when the file was last written or confirmed current
	Error     string `json:"error,omitempty"`
}

// Entry is one turn of the chat history.
type Entry struct {
	ID        string `json:"id"`
	Source    string `json:"source"` // "web" or "game"
	Character string `json:"character"`
	Question  string `json:"question"`
	Answer    string `json:"answer"`
	Status    string `json:"status"` // "waiting", "done" or "error"
	Streamed  bool   `json:"streamed"`
	Usage     string `json:"usage,omitempty"` // "≈3.1k tokens", when the CLI reported it and the setting is on
	At        int64  `json:"at"`
}

// CharInfo is one captured character.
type CharInfo struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Class      string   `json:"class"`
	Level      int      `json:"level"`
	CapturedAt int64    `json:"captured_at"`
	Next       []string `json:"next,omitempty"` // the top next quests, from the digest
}

// NextQuests is how many next quests each character carries.
const NextQuests = 3

// AppOrigins are the webview origins of the desktop app (Tauri on Windows, and elsewhere).
var AppOrigins = []string{"http://tauri.localhost", "https://tauri.localhost", "tauri://localhost"}

// State is what the page loads and what a "game" event carries.
type State struct {
	Characters []CharInfo  `json:"characters"`
	Default    string      `json:"default"`
	CapturedAt int64       `json:"captured_at"`
	Error      string      `json:"error,omitempty"`
	History    []Entry     `json:"history"`
	Streaming  bool        `json:"streaming"`
	Digest     DigestState `json:"digest"`
	Settings   Settings    `json:"settings"`
}

type event struct{ name, data string }

// turn is one running answer; its events are kept so a late stream replays from the start.
type turn struct {
	mu     sync.Mutex
	events []event
	done   bool
	wake   chan struct{}
}

func (t *turn) emit(name string, v any, done bool) {
	b, _ := json.Marshal(v)
	t.mu.Lock()
	t.events = append(t.events, event{name, string(b)})
	t.done = t.done || done
	close(t.wake)
	t.wake = make(chan struct{})
	t.mu.Unlock()
}

// Server is the companion.
type Server struct {
	cfg   Config
	Token string
	port  string
	ctx   context.Context

	mu       sync.Mutex
	chars    []CharInfo
	captured int64
	loadErr  string
	web      []Entry
	game     []Entry
	gameDone map[string]Entry // in-game question id -> its answer, kept in memory (no Answers.lua)
	turns    map[string]*turn
	busy     map[string]bool // client address -> an answer is in flight
	seq      int
	subs     map[chan event]bool
	digest   DigestState
	opts     ask.Options
	usageOn  bool
	setup    setupCache
}

// New returns a companion; it does not listen. Serve does that.
func New(cfg Config) *Server {
	if cfg.Poll <= 0 {
		cfg.Poll = time.Second
	}
	b := make([]byte, 16)
	rand.Read(b)
	if cfg.Debounce <= 0 {
		cfg.Debounce = 2 * time.Second
	}
	s := &Server{cfg: cfg, Token: hex.EncodeToString(b), turns: map[string]*turn{}, busy: map[string]bool{}, subs: map[chan event]bool{}}
	s.digest = DigestState{Enabled: cfg.AutoDigest}
	s.opts, s.usageOn = cfg.Ask.Options.Normalize(), !cfg.NoUsage
	s.digest.Status = s.digestStatus(cfg.AutoDigest)
	return s
}

// Serve serves on ln until ctx ends, watching the game files meanwhile.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	s.ctx = ctx
	s.port = strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)
	s.reload()
	go s.watch(ctx)
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Handler is the whole site behind the Host, Origin and token checks.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("POST /api/ask", s.ask)
	mux.HandleFunc("POST /api/auto-digest", s.autoDigest)
	mux.HandleFunc("POST /api/settings", s.setSettings)
	mux.HandleFunc("GET /api/stream/{id}", s.stream)
	mux.HandleFunc("GET /api/setup", s.setupState)
	mux.HandleFunc("POST /api/setup/install-addon", s.setupInstall)
	mux.HandleFunc("POST /api/setup/test-ai", s.setupTestAI)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if msg := s.reject(r); msg != "" {
			http.Error(w, msg, http.StatusForbidden)
			return
		}
		if s.cfg.App && s.cors(w, r) {
			return
		}
		if s.cfg.App && s.appToken(r) {
			mux.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/" && r.URL.Query().Get("token") != "" {
			// The token in the opened URL becomes a cookie, then leaves the address bar.
			if !s.tokenOK(r.URL.Query().Get("token")) {
				http.Error(w, "bad token", http.StatusForbidden)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: s.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		if c, err := r.Cookie(s.cookieName()); err != nil || !s.tokenOK(c.Value) {
			http.Error(w, "missing token: open the URL wowc-advisor companion printed", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// cors answers the app origin's cross-origin rules: it names the origin on every response and
// ends a preflight, which carries no token, there.
func (s *Server) cors(w http.ResponseWriter, r *http.Request) (preflight bool) {
	o := r.Header.Get("Origin")
	if o == "" || !slices.Contains(AppOrigins, o) {
		return false
	}
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", o)
	h.Set("Access-Control-Allow-Headers", "Content-Type, "+TokenHeader)
	h.Set("Access-Control-Allow-Methods", "GET, POST")
	h.Add("Vary", "Origin")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}

// TokenHeader carries the token for the app, whose origin cannot hold the cookie.
const TokenHeader = "X-Wowc-Token"

// appToken is true when an /api request carries the token in the header, or in the query for
// EventSource, which cannot set headers.
func (s *Server) appToken(r *http.Request) bool {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		return false
	}
	v := r.Header.Get(TokenHeader)
	if v == "" {
		v = r.URL.Query().Get("token")
	}
	return s.tokenOK(v)
}

func (s *Server) cookieName() string { return "wowc_token_" + s.port }

func (s *Server) tokenOK(v string) bool {
	return subtle.ConstantTimeCompare([]byte(v), []byte(s.Token)) == 1
}

// reject says why a request is refused before the token is looked at: a Host that is not this
// server's own (DNS rebinding), or an Origin that is not the page's own.
func (s *Server) reject(r *http.Request) string {
	host, port, err := net.SplitHostPort(r.Host)
	if err != nil || port != s.port {
		return "bad Host"
	}
	ok := host == "127.0.0.1" || host == "localhost"
	// A rebinding attack needs a name; on --lan the machine's own address is an IP literal.
	if s.cfg.LAN && net.ParseIP(host) != nil {
		ok = true
	}
	if !ok {
		return "bad Host"
	}
	if o := r.Header.Get("Origin"); o != "" {
		if s.cfg.App && slices.Contains(AppOrigins, o) {
			return ""
		}
		if u, err := url.Parse(o); err != nil || u.Scheme != "http" || u.Host != r.Host {
			return "bad Origin"
		}
	}
	return ""
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'unsafe-inline'; script-src 'unsafe-inline'")
	w.Write(page)
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.snapshot())
}

func (s *Server) snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := State{Characters: s.chars, CapturedAt: s.captured, Error: s.loadErr, Streaming: s.cfg.Stream, Digest: s.digest, Settings: s.settingsLocked()}
	var best int64 = -1
	for _, c := range s.chars {
		if c.CapturedAt > best {
			best, st.Default = c.CapturedAt, c.Key
		}
	}
	st.History = append(append([]Entry{}, s.game...), s.web...)
	return st
}

// reload re-reads ForeverScribe.lua; the previous list stays when it cannot be read.
func (s *Server) reload() {
	f, err := character.LoadFile(s.cfg.ForeverScribe)
	var ds store.DataStore
	if s.cfg.Dataset != "" {
		ds = store.NewDir(s.cfg.Dataset)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.loadErr = err.Error()
		return
	}
	s.loadErr, s.chars, s.captured = "", nil, 0
	for _, c := range f.Characters {
		ci := CharInfo{Key: c.Key, Name: c.Name, Class: c.Class, Next: nextQuests(c, f, ds)}
		if c.Level != nil {
			ci.Level = *c.Level
		}
		if c.CapturedAt != nil {
			ci.CapturedAt = *c.CapturedAt
		}
		s.captured = max(s.captured, ci.CapturedAt)
		s.chars = append(s.chars, ci)
	}
}

// publish sends an event to every connected page.
func (s *Server) publish(name string, v any) {
	b, _ := json.Marshal(v)
	s.mu.Lock()
	defer s.mu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- event{name, string(b)}:
		default: // a stalled page misses events and catches up on its next state load
		}
	}
}

// watch polls the two SavedVariables files: ForeverScribe.lua for new game data, and the addon's
// WoWCompanion.lua for questions asked in game, which it answers into Answers.lua like ask --watch.
func (s *Server) watch(ctx context.Context) {
	last, lastAsk := ask.Stamp(s.cfg.ForeverScribe), ""
	var asking sync.Mutex
	var due time.Time // when the pending digest write falls due; zero when none
	for {
		if cur := ask.Stamp(s.cfg.ForeverScribe); cur != last {
			last = cur
			s.reload()
			s.publish("game", s.snapshot())
			due = time.Now().Add(s.cfg.Debounce) // each further change pushes it back
		}
		if !due.IsZero() && !time.Now().Before(due) {
			due = time.Time{}
			s.writeDigest()
		}
		if cur := ask.Stamp(s.cfg.Ask.SavedVariables); cur != lastAsk && cur != "" && asking.TryLock() {
			lastAsk = cur
			go func() {
				defer asking.Unlock()
				s.gameQuestions(ctx)
			}()
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.Poll):
		}
	}
}

// gameQuestions shows the in-game questions as waiting, answers them (writing Answers.lua), and
// shows the answers.
func (s *Server) gameQuestions(ctx context.Context) {
	s.showGame()
	if len(s.cfg.Ask.Command) == 0 {
		return
	}
	if s.cfg.Ask.AnswersPath != "" { // the deprecated --answers-file mode: Answers.lua too
		if res, err := ask.Run(ctx, s.askCfg()); err == nil && res.Answered > 0 {
			s.showGame()
		}
		return
	}
	s.answerGame(ctx)
}

// answerGame answers each unanswered in-game question once, in the overlay only: nothing is written
// to the game folder. Answered ids are kept in memory for the life of the companion.
func (s *Server) answerGame(ctx context.Context) {
	raw, err := readFile(s.cfg.Ask.SavedVariables)
	if err != nil {
		return
	}
	qs, err := ask.Questions(raw)
	if err != nil {
		return
	}
	for _, q := range qs {
		s.mu.Lock()
		_, done := s.gameDone[q.ID]
		s.mu.Unlock()
		if done || q.Answered {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		ac := s.askCfg()
		text, _, err := ask.Respond(ctx, ac, ask.Question{ID: q.ID, Character: q.Character, Text: q.Text}, ask.PromptFor(q, ac.Options.Length), s.cfg.Stream, nil)
		e := Entry{Answer: text, Status: "done"}
		if err != nil {
			e.Answer, e.Status = "Could not answer: "+err.Error(), "error"
		}
		s.mu.Lock()
		if s.gameDone == nil {
			s.gameDone = map[string]Entry{}
		}
		s.gameDone[q.ID] = e
		s.mu.Unlock()
		s.showGame()
	}
}

func (s *Server) showGame() {
	raw, err := readFile(s.cfg.Ask.SavedVariables)
	if err != nil {
		return
	}
	qs, err := ask.Questions(raw)
	if err != nil {
		return
	}
	have := map[string]ask.Answer{}
	if s.cfg.Ask.AnswersPath != "" {
		have = ask.ReadAnswers(s.cfg.Ask.AnswersPath)
	}
	var list []Entry
	for _, q := range qs {
		e := Entry{ID: "game-" + q.ID, Source: "game", Character: q.Character, Question: q.Text, Status: "waiting", At: q.AskedAt}
		if a, ok := have[q.ID]; ok {
			e.Answer, e.Status = a.Text, "done"
			if strings.HasPrefix(a.Text, "Could not answer") {
				e.Status = "error"
			}
		}
		s.mu.Lock()
		if d, ok := s.gameDone[q.ID]; ok {
			e.Answer, e.Status = d.Answer, d.Status
		}
		s.mu.Unlock()
		list = append(list, e)
	}
	if len(list) > maxEntries {
		list = list[len(list)-maxEntries:]
	}
	s.mu.Lock()
	s.game = list
	s.mu.Unlock()
	s.publish("history", s.snapshot().History)
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	ch := make(chan event, 16)
	s.mu.Lock()
	s.subs[ch] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()
	sseHeaders(w)
	rc := http.NewResponseController(w)
	fmt.Fprint(w, ": connected\n\n")
	rc.Flush()
	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-ch:
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.name, ev.data)
			rc.Flush()
		}
	}
}

func sseHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
}

type askRequest struct {
	Character string `json:"character"`
	Text      string `json:"text"`
}

func (s *Server) ask(w http.ResponseWriter, r *http.Request) {
	var req askRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request body (at most 8 KB of JSON)", http.StatusBadRequest)
		return
	}
	req.Text = strings.TrimSpace(req.Text)
	switch {
	case req.Text == "":
		http.Error(w, "empty question", http.StatusBadRequest)
		return
	case utf8.RuneCountInString(req.Text) > MaxQuestion:
		http.Error(w, fmt.Sprintf("question longer than %d characters", MaxQuestion), http.StatusRequestEntityTooLarge)
		return
	}
	if len(s.cfg.Ask.Command) == 0 {
		http.Error(w, ErrNoAgent, http.StatusConflict)
		return
	}
	client, _, _ := net.SplitHostPort(r.RemoteAddr)
	s.mu.Lock()
	if s.busy[client] {
		s.mu.Unlock()
		http.Error(w, "an answer is already being written", http.StatusTooManyRequests)
		return
	}
	s.busy[client] = true
	s.seq++
	id := fmt.Sprintf("t%d", s.seq)
	t := &turn{wake: make(chan struct{})}
	s.turns[id] = t
	delete(s.turns, fmt.Sprintf("t%d", s.seq-50)) // finished streams are not kept for long
	history := s.historyFor(req.Character)
	s.mu.Unlock()
	go s.answer(id, client, t, req, history)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// historyFor is the earlier web turns of one character as a block of prompt text, within the caps.
// The caller holds s.mu.
func (s *Server) historyFor(char string) string {
	var parts []string
	for i := len(s.web) - 1; i >= 0 && len(parts) < HistoryTurns; i-- {
		e := s.web[i]
		if e.Character != char || e.Status != "done" {
			continue
		}
		a := []rune(e.Answer)
		if len(a) > HistoryAnswerChars {
			a = append(a[:HistoryAnswerChars], '.', '.', '.')
		}
		parts = append([]string{"Q: " + e.Question + "\nA: " + string(a)}, parts...)
	}
	for total := 0; len(parts) > 0; parts = parts[1:] {
		total = 0
		for _, p := range parts {
			total += utf8.RuneCountInString(p)
		}
		if total <= HistoryChars {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Earlier in this conversation, for context only:\n" + strings.Join(parts, "\n") + "\n\nCurrent question: "
}

func (s *Server) answer(id, client string, t *turn, req askRequest, history string) {
	q := ask.Question{ID: id, Character: req.Character, Text: history + req.Text}
	e := Entry{ID: id, Source: "web", Character: req.Character, Question: req.Text, At: time.Now().Unix()}
	ac := s.askCfg()
	text, streamed, usage, err := ask.RespondUsage(s.ctx, ac, q, ask.PromptFor(q, ac.Options.Length), s.cfg.Stream, func(chunk string) {
		t.emit("chunk", map[string]string{"text": chunk}, false)
	})
	e.Answer, e.Streamed, e.Status = text, streamed, "done"
	if s.settings().Usage {
		e.Usage = usage.Line()
	}
	if err != nil {
		e.Answer, e.Status = err.Error(), "error"
	}
	s.mu.Lock()
	s.web = append(s.web, e)
	if len(s.web) > maxEntries {
		s.web = s.web[len(s.web)-maxEntries:]
	}
	delete(s.busy, client)
	s.mu.Unlock()
	t.emit("done", e, true)
}

// stream sends a turn's events as Server-Sent Events: chunk events as text arrives, then one done.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	t := s.turns[r.PathValue("id")]
	s.mu.Unlock()
	if t == nil {
		http.NotFound(w, r)
		return
	}
	sseHeaders(w)
	rc := http.NewResponseController(w)
	for sent := 0; ; {
		t.mu.Lock()
		batch, done, wake := t.events[sent:], t.done, t.wake
		t.mu.Unlock()
		for _, ev := range batch {
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.name, ev.data)
		}
		rc.Flush()
		sent += len(batch)
		if done && len(batch) == 0 {
			return
		}
		if len(batch) == 0 {
			select {
			case <-wake:
			case <-r.Context().Done():
				return
			}
		}
	}
}

func readFile(p string) (string, error) {
	b, err := os.ReadFile(p)
	return string(b), err
}

// nextQuests is the top NextQuests lines of the character's digest "next quests" section.
func nextQuests(c character.Character, f character.File, ds store.DataStore) []string {
	d := digest.Build(c, ds, digest.Options{Now: time.Now(), Client: f.ClientOf(c)})
	for _, sec := range d.Sections {
		if sec.ID != digest.NextQuests {
			continue
		}
		var out []string
		for _, l := range sec.Lines[:min(len(sec.Lines), NextQuests)] {
			out = append(out, l.Text)
		}
		return out
	}
	return nil
}

// digestPath is the Digest.lua to write.
func (s *Server) digestPath() (string, error) {
	if s.cfg.DigestPath != "" {
		return s.cfg.DigestPath, nil
	}
	return digest.DigestPath(s.cfg.ForeverScribe)
}

// digestStatus is the status when nothing has gone wrong: off, addon not found, or on.
func (s *Server) digestStatus(enabled bool) string {
	if p, err := s.digestPath(); err != nil || digest.CheckInstalled(p) != nil {
		return DigestNoAddon
	}
	if !enabled {
		return DigestOff
	}
	return DigestOn
}

// SetAutoDigest turns the automatic digest on or off and republishes the status.
func (s *Server) SetAutoDigest(on bool) {
	st := s.digestStatus(on)
	s.mu.Lock()
	s.digest.Enabled, s.digest.Status, s.digest.Error = on, st, ""
	s.mu.Unlock()
	s.publish("game", s.snapshot())
}

// writeDigest writes the digest of every captured character, as `wowc-advisor digest` does, unless
// the setting is off or the addon folder is missing. A file that already holds the same advice is
// left untouched.
func (s *Server) writeDigest() {
	s.mu.Lock()
	on := s.digest.Enabled
	s.mu.Unlock()
	st := s.digestStatus(on)
	if !on || st == DigestNoAddon {
		s.mu.Lock()
		s.digest.Status = st
		s.mu.Unlock()
		s.publish("game", s.snapshot())
		return
	}
	_, err := server.WriteDigest(server.Config{
		SavedVariables: s.cfg.ForeverScribe, Dataset: s.cfg.Dataset, DigestPath: s.cfg.DigestPath, SkipUnchanged: true,
	}, "")
	s.mu.Lock()
	if err != nil {
		s.digest.Status, s.digest.Error = DigestFailed, err.Error()
	} else {
		s.digest.Status, s.digest.Error, s.digest.UpdatedAt = DigestOn, "", time.Now().Unix()
	}
	s.mu.Unlock()
	s.publish("game", s.snapshot())
}

// autoDigest is the settings switch: {"on": true|false}.
func (s *Server) autoDigest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		On bool `json:"on"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	s.SetAutoDigest(req.On)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.snapshot().Digest)
}
