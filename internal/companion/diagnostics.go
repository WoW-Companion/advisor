package companion

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/gamesetup"
	"github.com/WoW-Companion/advisor/internal/lua"
	"github.com/WoW-Companion/advisor/internal/route"
	"github.com/WoW-Companion/advisor/internal/server"
)

// The "Copy diagnostics" text: versions, the AI setup (kind and model only), the save's age, the
// addon's last errors, the shell log's tail and the launch-guard record, with the player's names,
// home folder, keys and email addresses taken out. It is built on request, returned to the page for
// the clipboard and never written or sent anywhere.

const (
	diagMaxBytes   = 4000 // the whole block
	diagMaxErrors  = 5
	diagMaxErrText = 300 // one addon error message
	diagLogLines   = 30  // shell log lines kept (the last 20 plus up to 10 earlier records)
	diagLineMax    = 240
)

type diagRequest struct {
	ShellLog string `json:"shell_log"` // the shell log's tail, read by the shell
	Guard    string `json:"guard"`     // launch-guard.json
	OS       string `json:"os"`        // the page's own idea of the OS name and version
}

func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	var req diagRequest
	json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req)
	writeJSON(w, map[string]string{"text": s.diagnosticsText(req)})
}

var (
	reEmail  = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*\.[A-Za-z]{2,}`)
	reBearer = regexp.MustCompile(`(?i)\bBearer\s+\S+`)
	reSK     = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{6,}`)
	reKeyVal = regexp.MustCompile(`(?i)\b((?:api[_-]?)?key|token|secret|password)(["']?\s*[:=]\s*["']?)[^\s"',}]+`)
	reLong   = regexp.MustCompile(`[A-Za-z0-9+=_-]{32,}`)
	reHomes  = regexp.MustCompile(`(?i)(?:[A-Z]:[\\/]+Users[\\/]+|/Users/|/home/)[^\\/\s"']+`)
	reSpace  = regexp.MustCompile(`[ \t]+`)
)

// stripper returns a function that removes the names, the home folder and anything key-like.
// names maps a name to its placeholder; home is the user's home folder ("" when unknown).
func stripper(names map[string]string, home string) func(string) string {
	var olds []string
	for n := range names {
		olds = append(olds, n)
	}
	sort.Slice(olds, func(i, j int) bool { return len(olds[i]) > len(olds[j]) })
	var nameRE *regexp.Regexp
	if len(olds) > 0 {
		q := make([]string, len(olds))
		for i, n := range olds {
			q[i] = regexp.QuoteMeta(n)
		}
		nameRE = regexp.MustCompile(`(?i)` + strings.Join(q, "|"))
	}
	var homeRE *regexp.Regexp
	if home = strings.TrimRight(home, `\/`); len(home) > 3 {
		homeRE = regexp.MustCompile(`(?i)` + strings.ReplaceAll(regexp.QuoteMeta(strings.ReplaceAll(home, `\`, "/")), "/", `[\\/]+`))
	}
	return func(t string) string {
		t = reEmail.ReplaceAllString(t, "<email>")
		t = reBearer.ReplaceAllString(t, "Bearer <key>")
		t = reSK.ReplaceAllString(t, "<key>")
		t = reKeyVal.ReplaceAllString(t, "$1$2<key>")
		if homeRE != nil {
			t = homeRE.ReplaceAllString(t, "~")
		}
		t = reHomes.ReplaceAllString(t, "~")
		if nameRE != nil {
			t = nameRE.ReplaceAllStringFunc(t, func(m string) string {
				for n, p := range names {
					if strings.EqualFold(n, m) {
						return p
					}
				}
				return "<name>"
			})
		}
		return reLong.ReplaceAllString(t, "<key>")
	}
}

// saveNames collects the character, realm and guild names in the save files beside the configured
// path, as name -> placeholder (Char1, Realm1, Guild1) in a stable order.
func saveNames(path string) (names map[string]string, errs []diagErr) {
	names = map[string]string{}
	counts := map[string]int{}
	add := func(kind, v string) {
		v = strings.TrimSpace(v)
		if len(v) < 2 {
			return
		}
		for _, form := range []string{v, strings.ReplaceAll(v, " ", "")} {
			if _, ok := names[form]; !ok {
				if form == v {
					counts[kind]++
				}
				names[form] = fmt.Sprintf("%s%d", kind, counts[kind])
			}
		}
	}
	if path == "" {
		return names, nil
	}
	if f, err := character.LoadFile(path); err == nil {
		for _, c := range f.Characters {
			add("Char", c.Name)
			add("Realm", c.Realm)
		}
	}
	fsPath, wowcPath := character.Siblings(path)
	for _, p := range []string{wowcPath, fsPath} {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		vars, err := lua.Parse(string(b))
		if err != nil {
			continue
		}
		walkGuilds(vars, func(g string) { add("Guild", g) })
		if p == wowcPath {
			errs = addonErrors(vars)
		}
	}
	return names, errs
}

func walkGuilds(v any, f func(string)) {
	switch t := v.(type) {
	case map[string]any:
		for k, x := range t {
			walkNamed(k, x, f)
		}
	case lua.Table:
		for k, x := range t {
			if ks, ok := k.(string); ok {
				walkNamed(ks, x, f)
			} else {
				walkGuilds(x, f)
			}
		}
	}
}

func walkNamed(k string, x any, f func(string)) {
	if s, ok := x.(string); ok {
		if l := strings.ToLower(k); l == "guild" || l == "guildname" || l == "guild_name" {
			f(s)
		}
		return
	}
	walkGuilds(x, f)
}

type diagErr struct{ kind, at, msg string }

// addonErrors reads the addon's error log: WoWCompanionDB.raw.errors (or .errors), newest last.
func addonErrors(vars map[string]any) []diagErr {
	db, _ := vars[character.WoWCVariable].(lua.Table)
	var list lua.Table
	if raw, ok := db["raw"].(lua.Table); ok {
		list, _ = raw["errors"].(lua.Table)
	}
	if list == nil {
		list, _ = db["errors"].(lua.Table)
	}
	var out []diagErr
	for i := 1; i <= 100000; i++ {
		e, ok := list[int64(i)]
		if !ok {
			break
		}
		out = append(out, oneErr(e))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func str(t lua.Table, keys ...string) string {
	for _, k := range keys {
		switch v := t[k].(type) {
		case string:
			return v
		case float64:
			return fmt.Sprint(int64(v))
		case int64:
			return fmt.Sprint(v)
		}
	}
	return ""
}

func oneErr(e any) diagErr {
	if s, ok := e.(string); ok {
		return diagErr{msg: s}
	}
	t, _ := e.(lua.Table)
	return diagErr{kind: str(t, "kind", "type", "k"), at: str(t, "time", "at", "t", "ts"), msg: str(t, "message", "msg", "text", "err", "error")}
}

func oneLine(s string, max int) string {
	s = reSpace.ReplaceAllString(strings.NewReplacer("\r", " ", "\n", " ⏎ ").Replace(s), " ")
	if r := []rune(s); len(r) > max {
		s = string(r[:max]) + "…"
	}
	return s
}

func ageText(secs int64) string {
	switch {
	case secs < 0:
		return "in the future"
	case secs < 120:
		return fmt.Sprintf("%ds", secs)
	case secs < 7200:
		return fmt.Sprintf("%dm", secs/60)
	case secs < 2*86400:
		return fmt.Sprintf("%dh", secs/3600)
	}
	return fmt.Sprintf("%dd", secs/86400)
}

// wroteAfter says whether the file at p was written after the save was.
func wroteAfter(p string, saved int64) string {
	fi, err := os.Stat(p)
	switch {
	case p == "" || err != nil:
		return "missing"
	case saved == 0:
		return "present (no save to compare)"
	case fi.ModTime().Unix() >= saved:
		return "yes"
	}
	return "no (older than the save)"
}

func (s *Server) datasetVersion() string {
	dir := s.datasetDir()
	if dir == "" {
		return "none"
	}
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return "unknown"
	}
	m, err := dataset.ParseManifest(b)
	if err != nil || m.DatasetVersion == "" {
		return "unknown"
	}
	return m.DatasetVersion
}

func (s *Server) diagnosticsText(req diagRequest) string {
	now := s.now()
	home, _ := os.UserHomeDir()
	names, errs := saveNames(s.cfg.ForeverScribe)
	strip := stripper(names, home)
	s.mu.Lock()
	gameAddon, savedAt := s.gameAddon, s.savedAt
	s.mu.Unlock()
	running := server.Version
	if s.cfg.Version != "" {
		running = s.cfg.Version
	}
	addon := "not installed"
	if game, ok := gamesetup.Resolve(gamesetup.Options{GameDir: s.cfg.GameDir, SavedVariables: s.cfg.ForeverScribe}); ok {
		if v, ok := gamesetup.InstalledAddonVersion(game); ok {
			addon = v
		}
	}
	if gameAddon == "" {
		gameAddon = "none recorded"
	}
	ai := "default agent (no saved client)"
	if cl := s.providerSettings().Clients; len(cl) > 0 {
		ai = cl[0].Kind
		if cl[0].Model != "" {
			ai += ", model " + cl[0].Model
		} else {
			ai += ", default model"
		}
	}
	digestPath, _ := s.digestPath()
	planPath := ""
	if digestPath != "" {
		planPath = digest.SiblingPath(digestPath, route.File)
	}
	save := "no save read yet"
	if savedAt > 0 {
		save = ageText(now.Unix()-savedAt) + " old"
	}

	h := diagHead{now, running, addon, gameAddon, ai, save, wroteAfter(digestPath, savedAt), wroteAfter(planPath, savedAt), oneLine(req.OS, 120)}
	ds := s.datasetVersion()
	logLines := lastLines(req.ShellLog, diagLogLines)
	for {
		text := diagBlock(h, ds, errs, logLines, req.Guard, strip)
		if len(text) <= diagMaxBytes || len(logLines) == 0 {
			return capText(text, diagMaxBytes)
		}
		logLines = logLines[1:] // the oldest log line goes first
	}
}

type diagHead struct {
	now                                           time.Time
	app, addon, saved, ai, save, digest, plan, os string
}

func diagBlock(h diagHead, ds string, errs []diagErr, logLines []string, guard string, strip func(string) string) string {
	line := func(format string, a ...any) string { return strip(oneLine(fmt.Sprintf(format, a...), diagLineMax)) }
	var b strings.Builder
	w := func(l string) { b.WriteString(l + "\n") }
	w("WoW Companion diagnostics, " + h.now.UTC().Format(time.RFC3339))
	w("== Versions ==")
	w(line("app %s, addon installed %s, addon in the latest save %s, dataset %s", h.app, h.addon, h.saved, ds))
	w(line("OS %s", orDash(h.os)))
	w("== AI ==")
	w(line("%s", h.ai))
	w("== Game save ==")
	w(line("save %s; Digest.lua written after it: %s; Plan.lua written after it: %s", h.save, h.digest, h.plan))
	w("== Addon errors (last 5) ==")
	if len(errs) > diagMaxErrors {
		errs = errs[len(errs)-diagMaxErrors:]
	}
	if len(errs) == 0 {
		w("none")
	}
	for _, e := range errs {
		w(line("%s | %s | %s", orDash(e.kind), orDash(e.at), oneLine(e.msg, diagMaxErrText)))
	}
	w("== App log ==")
	if len(logLines) == 0 {
		w("none")
	}
	for _, l := range logLines {
		w(line("%s", l))
	}
	w("== Launch guard ==")
	w(orDash(strip(oneLine(strings.TrimSpace(guard), 600))))
	return b.String()
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func lastLines(text string, n int) []string {
	var out []string
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

// capText cuts text to at most max bytes at a line end, with a note when it did.
func capText(t string, max int) string {
	if len(t) <= max {
		return t
	}
	note := "\n[cut to fit]\n"
	t = t[:max-len(note)]
	for len(t) > 0 && !strings.HasSuffix(t, "\n") && t[len(t)-1] >= 0x80 && t[len(t)-1] < 0xC0 {
		t = t[:len(t)-1] // do not end inside a multi-byte character
	}
	if i := strings.LastIndex(t, "\n"); i > 0 {
		t = t[:i]
	}
	return t + note
}
