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
	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/provider"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/release"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/server"
	"github.com/WoW-Companion/advisor/internal/store"
	"github.com/WoW-Companion/advisor/internal/talents"
	"github.com/WoW-Companion/advisor/internal/unlock"
	"github.com/WoW-Companion/advisor/internal/whatsnew"
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
const ErrNoAgent = "No AI client set up yet: install Claude Code, Codex or Antigravity CLI, or choose one in settings."

// Config is one companion's settings.
type Config struct {
	ForeverScribe string        // ForeverScribe.lua, polled for game data
	Ask           ask.Config    // the agent; SavedVariables and AnswersPath serve in-game questions
	Stream        bool          // the agent command is Claude's and can print stream-json
	LAN           bool          // listening on all interfaces
	App           bool          // the desktop app's webview is the client: its origins pass, and the token may come in a header or query
	Dataset       string        // dataset directory, for each character's next quests (empty: none until Setup downloads it)
	Cache         dataset.Store // where the Setup row looks for and downloads the dataset (Root empty: the per-user cache)
	Poll          time.Duration
	NoUsage       bool                                          // start with the usage line off
	AutoDigest    bool                                          // rewrite the in-game digest when ForeverScribe saves
	DigestPath    string                                        // Digest.lua; empty derives it from ForeverScribe
	Debounce      time.Duration                                 // quiet time after a save before the digest is written (default 2s)
	GameDir       string                                        // the WoW folder the player chose in the setup checklist (empty: detect)
	HTTP          *http.Client                                  // for the addon lookup and install (nil: the default client)
	AddonAPI      string                                        // the addon's latest-release URL (empty: GitHub's)
	Update        func(context.Context) (release.Result, error) // looks up the latest release for /api/update; nil answers no notice
	Profiles      string                                        // the play-style store (empty: profile.Path())
	Now           func() time.Time                              // the clock for ask ages (nil: time.Now)
	Providers     string                                        // the saved client list (empty: provider.SettingsPath())
	Keys          provider.KeyStore                             // where API keys are kept (nil: the platform's default)
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
	Status    string `json:"status"` // "waiting", "answering" (text so far in Answer), "done" or "error"
	Streamed  bool   `json:"streamed"`
	UsageTip  string `json:"usage_tip,omitempty"` // the usage line's tooltip: cached tokens are cheaper but may still count
	Usage     string `json:"usage,omitempty"`     // "≈3.1k tokens · haiku": tokens when the CLI reported them, and the model; only with the setting on
	Notice    string `json:"notice,omitempty"`    // a one-line notice, e.g. the reasoning setting was dropped
	At        int64  `json:"at"`
	// Needs is set, with Status "needs", when the question needs a play style the character has not
	// saved: the choices to show as buttons. No AI call was made. Saving the choice (POST /api/profile)
	// asks the question again, once, and says so with a "reask" event.
	Needs *ask.Needs `json:"needs,omitempty"`
}

// CharInfo is one captured character.
type CharInfo struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Class      string   `json:"class"`
	Level      int      `json:"level"`
	CapturedAt int64    `json:"captured_at"`
	Next       []string `json:"next,omitempty"` // the top next quests, from the digest
	// NextLines is the same quests as Next with the digest's structured, untruncated fields, so the
	// overlay can lay them out like the in-game panel. Next stays for older pages.
	NextLines []NextLine `json:"next_lines,omitempty"`
	// Entries is the character's current panel entries in the in-game panel's order, each with its
	// suggested questions (asks) and ref, so the overlay can offer them as one-click questions.
	Entries  []PlanEntry `json:"entries,omitempty"`
	Plan     []string    `json:"plan,omitempty"`      // the route's first stops, in order
	PlanLine string      `json:"plan_line,omitempty"` // "N stops"
	// PlanSteps is the same stops with their lock state, so the overlay can dim a locked one.
	PlanSteps []PlanStep `json:"plan_steps,omitempty"`
	// Style is the saved play style (the overlay's "Play style" row); nil until the player sets one.
	Style *character.Profile `json:"style,omitempty"`
	// Specs is the class's spec names for the Spec dropdown; an old free-text spec in Style is
	// already migrated to the matching one, or "" for "Not sure yet".
	Specs []string `json:"specs,omitempty"`
	// Suggestions is the ranked list of at most three next actions (see suggest.go).
	Suggestions []Suggestion `json:"suggestions,omitempty"`
}

// PlanStep is one route stop for the overlay; After is the step a locked stop waits for.
type PlanStep struct {
	Text   string  `json:"text"` // the action in words, without the place
	X      float64 `json:"x,omitempty"`
	Y      float64 `json:"y,omitempty"`
	Locked bool    `json:"locked,omitempty"`
	After  int     `json:"after,omitempty"`
}

// planSteps is the first limit stops of p, with their lock state.
func planSteps(p route.Plan, limit int) []PlanStep {
	var out []PlanStep
	for i, s := range p.Steps {
		if i >= limit {
			break
		}
		out = append(out, PlanStep{Text: route.StepText(s), X: s.X, Y: s.Y, Locked: s.Locked, After: s.After})
	}
	return out
}

// NextObjective is one objective of a next quest; Have and Need are set only when HasCount.
type NextObjective struct {
	Text     string `json:"text"`
	Have     int    `json:"have,omitempty"`
	Need     int    `json:"need,omitempty"`
	HasCount bool   `json:"has_count,omitempty"`
}

// NextLine is one next-list entry: the digest line's text plus its structured fields, none clipped.
type NextLine struct {
	Text       string          `json:"text"`
	Kind       string          `json:"kind,omitempty"`
	Title      string          `json:"title,omitempty"`
	Level      int             `json:"level,omitempty"`
	Class      bool            `json:"class,omitempty"`
	Elite      bool            `json:"elite,omitempty"`
	NPC        string          `json:"npc,omitempty"`
	Zone       string          `json:"zone,omitempty"`
	Objectives []NextObjective `json:"objectives,omitempty"`
	Points     int             `json:"points,omitempty"`
	Unlocks    []string        `json:"unlocks,omitempty"` // a turn-in's follow-ups: "Quest (from NPC, here)"
}

// NextQuests is how many next quests each character carries.
const NextQuests = 3

// PlanStops is how many route stops each character carries for the overlay.
const PlanStops = 6

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
	DataNotice string      `json:"data_notice,omitempty"` // set once a dataset update has installed
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
	pmu      sync.Mutex // guards the saved client list file
	chars    []CharInfo
	captured int64
	loadErr  string
	web      []Entry
	turns    map[string]*turn
	busy     map[string]bool       // client address -> an answer is in flight
	pending  map[string]askRequest // character -> the question waiting for a play style
	seq      int
	subs     map[chan event]bool
	digest   DigestState
	opts     ask.Options
	usageOn  bool
	setup    setupCache
	data     dataState
	planKeys map[string]map[string]bool // character -> every step of its current plan, for "N new steps"
	refresh  sync.Mutex
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
	s := &Server{cfg: cfg, Token: hex.EncodeToString(b), turns: map[string]*turn{}, busy: map[string]bool{}, pending: map[string]askRequest{}, subs: map[chan event]bool{}}
	s.data.dir = cfg.Dataset
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
	go s.watchDataset(ctx)
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
	mux.HandleFunc("POST /api/profile", s.setProfile)
	mux.HandleFunc("GET /api/suggestions", s.suggestions)
	mux.HandleFunc("POST /api/refresh-plan", s.refreshPlanHandler)
	mux.HandleFunc("GET /api/update", s.update)
	mux.HandleFunc("GET /api/whats-new", s.whatsNew)
	mux.HandleFunc("GET /api/stream/{id}", s.stream)
	mux.HandleFunc("GET /api/setup", s.setupState)
	mux.HandleFunc("POST /api/setup/install-addon", s.setupInstall)
	mux.HandleFunc("POST /api/setup/write-data", s.setupWriteData)
	mux.HandleFunc("POST /api/setup/test-ai", s.setupTestAI)
	mux.HandleFunc("POST /api/setup/download-dataset", s.setupDownloadData)
	mux.HandleFunc("GET /api/providers", s.providersList)
	mux.HandleFunc("POST /api/providers/active", s.setActiveProvider)
	mux.HandleFunc("POST /api/providers/clients", s.setClients)
	mux.HandleFunc("POST /api/providers/key", s.setKey)
	mux.HandleFunc("POST /api/providers/key/delete", s.deleteKey)
	mux.HandleFunc("GET /api/providers/models", s.providerModels)
	mux.HandleFunc("POST /api/providers/test", s.testProvider)
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
	st := State{Characters: s.chars, CapturedAt: s.captured, Error: s.loadErr, Streaming: s.cfg.Stream, Digest: s.digest, Settings: s.settingsLocked(), DataNotice: s.dataNotice()}
	var best int64 = -1
	for _, c := range s.chars {
		if c.CapturedAt > best {
			best, st.Default = c.CapturedAt, c.Key
		}
	}
	st.History = append([]Entry{}, s.web...)
	return st
}

// reload re-reads ForeverScribe.lua; the previous list stays when it cannot be read.
func (s *Server) reload() {
	f, err := character.LoadFile(s.cfg.ForeverScribe)
	var ds store.DataStore
	dsDir := s.datasetDir()
	if dsDir != "" {
		ds = store.NewDir(dsDir)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.loadErr = err.Error()
		return
	}
	s.loadErr, s.chars, s.captured = "", nil, 0
	s.planKeys = map[string]map[string]bool{}
	for _, c := range f.Characters {
		c = character.AttachTotals(unlock.AddonSV(s.cfg.Ask.SavedVariables), profile.Attach(s.profilesPath(), c))
		ci := CharInfo{Key: c.Key, Name: c.Name, Class: c.Class, Next: []string{NoDataMessage}, Style: c.Profile}
		ci.Specs = talents.Specs(ds, c)
		if c.Profile != nil && len(ci.Specs) > 0 {
			p := *c.Profile
			p.Spec = talents.MatchSpec(ci.Specs, p.Spec)
			ci.Style = &p
		}
		if ds != nil {
			qx := questie.ForSavedVariables(s.cfg.Ask.SavedVariables)
			var plan route.Plan
			if qx != nil {
				plan = route.BuildWith(c, qx, route.OptionsFor(s.cfg.Ask.SavedVariables, ds))
			}
			d := digest.Build(c, ds, digest.Options{Now: time.Now(), Client: f.ClientOf(c), Questie: qx, Unlocks: route.UnlockNotes(plan)})
			ci.NextLines = nextFrom(d)
			ci.Entries = entriesOf(d)
			ci.Next = nextTexts(ci.NextLines)
			if qx != nil {
				ci.Plan, ci.PlanLine = route.Lines(plan, PlanStops)
				ci.PlanSteps = planSteps(plan, PlanStops)
				keys := map[string]bool{}
				for _, st := range plan.Steps {
					keys[fmt.Sprintf("%s|%d", st.Label, st.QuestID)] = true
				}
				s.planKeys[c.Key] = keys
			}
		}
		if c.Level != nil {
			ci.Level = *c.Level
		}
		if c.CapturedAt != nil {
			ci.CapturedAt = *c.CapturedAt
		}
		s.captured = max(s.captured, ci.CapturedAt)
		ci.Suggestions = suggest(suggestInput{HasStyle: c.Profile != nil, Entries: ci.Entries, PlanStale: s.planStaleFor(ci.CapturedAt), HasPlanSrc: ds != nil && s.planKeys[c.Key] != nil})
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

// watch polls ForeverScribe.lua for new game data. Questions are asked in the overlay only.
func (s *Server) watch(ctx context.Context) {
	last := ask.Stamp(s.cfg.ForeverScribe)
	var due time.Time // when the pending digest write falls due; zero when none
	// Write at once, not on the first save: an upgraded addon lists Plan.lua before any save happens.
	s.writeDigest()
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
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.Poll):
		}
	}
}

func (s *Server) now() time.Time {
	if s.cfg.Now != nil {
		return s.cfg.Now()
	}
	return time.Now()
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
	// Ref is the optional subject of the question (an entry's ref from the digest, see docs/digest.md):
	// the advisor expands it into the entry's facts. A question that exactly matches one of the
	// character's suggested asks gets that entry's ref without it.
	Ref string `json:"ref,omitempty"`
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
	if !s.askCfg().Usable() {
		http.Error(w, ErrNoAgent, http.StatusConflict)
		return
	}
	client, _, _ := net.SplitHostPort(r.RemoteAddr)
	id, ok := s.startAsk(client, req)
	if !ok {
		http.Error(w, "an answer is already being written", http.StatusTooManyRequests)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"id": id})
}

// startAsk begins an answer for req in the background and returns its turn id; ok is false when this
// client already has one in flight.
func (s *Server) startAsk(client string, req askRequest) (id string, ok bool) {
	s.mu.Lock()
	if s.busy[client] {
		s.mu.Unlock()
		return "", false
	}
	s.busy[client] = true
	s.seq++
	id = fmt.Sprintf("t%d", s.seq)
	t := &turn{wake: make(chan struct{})}
	s.turns[id] = t
	delete(s.turns, fmt.Sprintf("t%d", s.seq-50)) // finished streams are not kept for long
	history := s.historyFor(req.Character)
	s.mu.Unlock()
	go s.answer(id, client, t, req, history)
	return id, true
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
	e := Entry{ID: id, Source: "web", Character: req.Character, Question: req.Text, At: time.Now().Unix()}
	ac := s.askCfg()
	plain := ask.Question{Character: req.Character, Text: req.Text, Ref: req.Ref}
	plain.Ref = ask.ResolveRef(ac, plain)
	if n, needs := ask.PlayStyleNeeded(ac, plain); needs {
		// A question that needs a play style is not worth an AI call: offer the choices, and ask it again
		// once the choice is saved (setProfile).
		e.Status, e.Needs = "needs", &n
		s.mu.Lock()
		s.pending[req.Character] = askRequest{Character: req.Character, Text: req.Text, Ref: plain.Ref}
		delete(s.busy, client)
		s.mu.Unlock()
		t.emit("done", e, true)
		return
	}
	q := ask.Question{ID: id, Character: req.Character, Text: history + req.Text, Ref: plain.Ref}
	text, streamed, usage, err := ask.RespondUsage(s.ctx, ac, q, ac.Prompt(q), s.cfg.Stream, func(chunk string) {
		t.emit("chunk", map[string]string{"text": chunk}, false)
	})
	e.Answer, e.Streamed, e.Status = text, streamed, "done"
	if s.settings().Usage {
		e.Usage = usage.Line()
		if usage.Cached > 0 {
			e.UsageTip = ask.UsageTip
		}
	}
	e.Notice = usage.Notice
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
func nextQuests(c character.Character, f character.File, ds store.DataStore, qx *questie.Index, unlocks map[int][]string) []NextLine {
	return nextFrom(digest.Build(c, ds, digest.Options{Now: time.Now(), Client: f.ClientOf(c), Questie: qx, Unlocks: unlocks}))
}

// nextFrom is the top NextQuests lines of d's "next quests" section.
func nextFrom(d digest.Digest) []NextLine {
	for _, sec := range d.Sections {
		if sec.ID != digest.NextQuests {
			continue
		}
		var out []NextLine
		for _, l := range sec.Lines[:min(len(sec.Lines), NextQuests)] {
			n := NextLine{Text: l.Text, Kind: l.Kind, Title: l.Title, Level: l.Level, Class: l.Class, Elite: l.Elite, NPC: l.NPC, Zone: l.Zone, Points: l.Points, Unlocks: l.Unlocks}
			for _, o := range l.Objectives {
				n.Objectives = append(n.Objectives, NextObjective{Text: o.Text, Have: o.Have, Need: o.Need, HasCount: o.HasCount})
			}
			out = append(out, n)
		}
		return out
	}
	return nil
}

// nextTexts is the plain text of each line.
func nextTexts(ls []NextLine) []string {
	var out []string
	for _, l := range ls {
		out = append(out, l.Text)
	}
	return out
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
	// Missing data files get an empty stub whatever the setting says, so the game's TOC never lists a missing file.
	server.EnsureDataFiles(server.Config{SavedVariables: s.cfg.ForeverScribe, DigestPath: s.cfg.DigestPath})
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
		SavedVariables: s.cfg.ForeverScribe, Dataset: s.datasetDir(), DigestPath: s.cfg.DigestPath, SkipUnchanged: true,
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
	if req.On {
		s.writeDigest()
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.snapshot().Digest)
}

// update answers the overlay's version line: one unauthenticated release lookup, made only when the
// page asks (it asks when "Check for updates" is on). Failure is an empty notice, never an error.
func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	var n release.Notice
	if s.cfg.Update != nil {
		n = release.NoticeFor(s.cfg.Update(r.Context()))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(n)
}

// planStaleFor reports whether the written Plan.lua is older than a capture or the dataset. The
// caller holds s.mu; with no addon folder there is no file to be stale and it says false.
func (s *Server) planStaleFor(capturedAt int64) bool {
	p, err := s.digestPath()
	if err != nil || digest.CheckInstalled(p) != nil {
		return false
	}
	return planStale(digest.SiblingPath(p, route.File), capturedAt, s.cfg.Dataset)
}

// whatsNew answers the overlay's Updates section: the last few versions' changelog, from the
// CHANGELOG.md bundled into the binary (no network).
func (s *Server) whatsNew(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(whatsnew.Bundled())
}
