// Package doctor checks the whole player setup: the capture, the addons, the dataset cache and
// the client configs. It reads files only; the one thing it writes is a short-lived probe file
// to prove a folder is writable.
package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/ask"
	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/setup"
)

// Statuses of a check. Info never fails the report.
const (
	Pass = "pass"
	Fail = "fail"
	Info = "info"
)

// StaleAfter is how old a capture may be before the doctor asks for a /reload.
const StaleAfter = time.Hour

// Addons are the folders the player installs under Interface/AddOns.
var Addons = []string{"WoWCompanion", digest.SlotAddon, ask.SlotAddon}

const gamePathHint = "use a SavedVariables path like <game>/WTF/Account/<account>/SavedVariables/ForeverScribe.lua"

// Options says what to check; Now and Env make it testable.
type Options struct {
	SavedVariables string
	Cache          string // dataset cache root; empty means the per-user cache
	Now            time.Time
	Env            setup.Env
}

// Check is one line of the report.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status" jsonschema:"pass, fail or info"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty" jsonschema:"one-line fix; set when the check failed"`
}

// Report is the doctor's result.
type Report struct {
	OK     bool    `json:"ok" jsonschema:"true when no check failed"`
	Checks []Check `json:"checks"`
	Text   string  `json:"text" jsonschema:"plain-text rendering, one line per check"`
}

type run struct {
	opt    Options
	checks []Check
}

func (r *run) add(name, status, detail, fix string) {
	r.checks = append(r.checks, Check{name, status, detail, fix})
}

func (r *run) pass(name, detail string)      { r.add(name, Pass, detail, "") }
func (r *run) fail(name, detail, fix string) { r.add(name, Fail, detail, fix) }

// Run performs every check. It never returns an error: a problem is a failed check.
func Run(opt Options) Report {
	r := &run{opt: opt}
	if r.opt.Now.IsZero() {
		r.opt.Now = time.Now()
	}
	file, ok := r.capture()
	client := r.freshness(file, ok)
	r.addons()
	r.dataset(client)
	r.writable()
	r.clients()
	return render(r.checks)
}

func (r *run) capture() (character.File, bool) {
	const name = "SavedVariables"
	sv := r.opt.SavedVariables
	if sv == "" {
		r.fail(name, "no SavedVariables file configured", "pass --savedvariables PATH or set WOWC_SAVEDVARIABLES to your ForeverScribe.lua")
		return character.File{}, false
	}
	f, err := character.LoadFile(sv)
	switch {
	case os.IsNotExist(err):
		r.fail(name, "file not found: "+sv, "log in once with ForeverScribe installed, then log out, so it writes "+filepath.Base(sv))
	case err != nil:
		r.fail(name, "could not read "+sv+": "+err.Error(), "log in and out once so ForeverScribe rewrites the file, or fix the path")
	case len(f.Characters) == 0:
		r.fail(name, sv+" lists no characters", "log in with a character and log out so ForeverScribe captures it")
	default:
		r.pass(name, fmt.Sprintf("%s parses, %d character(s)", sv, len(f.Characters)))
		return f, true
	}
	return f, false
}

// freshness checks the capture's age and returns the client that wrote it.
func (r *run) freshness(f character.File, ok bool) character.Client {
	const name = "Capture age"
	if !ok {
		r.fail(name, "no capture to age", "fix the SavedVariables check first")
		return character.Client{}
	}
	client := f.Client
	for _, c := range f.Characters {
		if client.Empty() {
			client = c.Client
		}
	}
	t := f.CapturedAt()
	if t == nil {
		r.fail(name, "the capture has no time", "/reload in game so ForeverScribe stamps a capture")
		return client
	}
	age := r.opt.Now.Sub(time.Unix(*t, 0)).Round(time.Minute)
	if age > StaleAfter {
		r.fail(name, fmt.Sprintf("captured %s ago", age), "/reload in game")
	} else {
		r.pass(name, fmt.Sprintf("captured %s ago", max(age, 0)))
	}
	return client
}

var tocVersion = regexp.MustCompile(`(?m)^##\s*Version:\s*(\S.*?)\s*$`)

// slot is <game>/Interface/AddOns/<addon>/<file> for the configured SavedVariables, as a native path.
func (r *run) slot(addon, file string) (string, bool) {
	p, ok := digest.SlotPath(r.opt.SavedVariables, addon, file)
	return filepath.FromSlash(strings.ReplaceAll(p, `\`, "/")), ok
}

func (r *run) addons() {
	for _, a := range Addons {
		name := "Addon " + a
		toc, ok := r.slot(a, a+".toc")
		if !ok {
			r.fail(name, "cannot find Interface/AddOns from the SavedVariables path", gamePathHint)
			continue
		}
		data, err := os.ReadFile(toc)
		if err != nil {
			r.fail(name, "not installed: no "+toc, "install "+a+" from the WoW Companion addon repo into "+filepath.Dir(filepath.Dir(toc)))
			continue
		}
		v := "unknown"
		if m := tocVersion.FindSubmatch(data); m != nil {
			v = string(m[1])
		}
		r.pass(name, "installed, version "+v)
	}
}

func (r *run) dataset(client character.Client) {
	const name, match = "Dataset cache", "Dataset build"
	root := r.opt.Cache
	if root == "" {
		var err error
		if root, err = dataset.DefaultRoot(); err != nil {
			r.fail(name, "no cache directory: "+err.Error(), "pass --cache DIR")
			return
		}
	}
	good, bad := dataset.Store{Root: root}.Cached()
	for b, err := range bad {
		r.fail(name, fmt.Sprintf("build %s failed verification: %v", b, err), "run `wowc-advisor dataset update --build "+b+"`")
	}
	if len(good) == 0 {
		if len(bad) == 0 {
			r.fail(name, "no dataset cached in "+root, "run `wowc-advisor dataset update`")
		}
		r.fail(match, "no verified dataset to compare", "fix the dataset cache first")
		return
	}
	r.pass(name, fmt.Sprintf("%d verified build(s) in %s, newest %s", len(good), root, good[0]))
	if client.Build == "" {
		r.fail(match, "the capture names no client build to match", "/reload in game with a current ForeverScribe")
		return
	}
	for _, b := range good {
		if dataset.Matches(b, client.Version, client.Build) {
			r.pass(match, "dataset "+b+" matches client build "+client.Build)
			return
		}
	}
	r.fail(match, fmt.Sprintf("no cached dataset matches client build %s (have %s)", client.Build, strings.Join(good, ", ")), "run `wowc-advisor dataset update`")
}

// canWrite proves a folder takes new files without touching the files the advisor owns.
func canWrite(dir string) error {
	f, err := os.CreateTemp(dir, ".doctor-*.tmp")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

func (r *run) writable() {
	for _, w := range []struct{ addon, file string }{{digest.SlotAddon, "Digest.lua"}, {ask.SlotAddon, "Answers.lua"}} {
		name := w.file + " writable"
		p, ok := r.slot(w.addon, w.file)
		if !ok {
			r.fail(name, "cannot derive the path from the SavedVariables path", gamePathHint)
			continue
		}
		if err := canWrite(filepath.Dir(p)); err != nil {
			r.fail(name, "cannot write in "+filepath.Dir(p)+": "+err.Error(), "install "+w.addon+" there and make the folder writable")
		} else if st, err := os.Stat(p); err == nil && st.Mode().Perm()&0o200 == 0 {
			r.fail(name, p+" is read-only", "make "+p+" writable")
		} else {
			r.pass(name, p)
		}
	}
}

// clients is informational: a client may simply not be installed.
func (r *run) clients() {
	for _, c := range setup.Clients {
		name := "Client " + c
		p, err := setup.Path(c, r.opt.Env)
		if err != nil {
			r.add(name, Info, err.Error(), "")
			continue
		}
		data, err := os.ReadFile(p)
		switch {
		case err != nil:
			r.add(name, Info, "no config at "+p+" (fine if you do not use it)", "wowc-advisor setup "+c+" --write")
		case strings.Contains(string(data), setup.ServerName):
			r.add(name, Info, p+" mentions "+setup.ServerName, "")
		default:
			r.add(name, Info, p+" does not mention "+setup.ServerName, "wowc-advisor setup "+c+" --write")
		}
	}
}

func render(checks []Check) Report {
	rep := Report{OK: true, Checks: checks}
	var b strings.Builder
	for _, c := range checks {
		fmt.Fprintf(&b, "%s  %s: %s\n", strings.ToUpper(c.Status), c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(&b, "      fix: %s\n", c.Fix)
		}
		if c.Status == Fail {
			rep.OK = false
		}
	}
	if rep.OK {
		b.WriteString("All checks passed.\n")
	} else {
		b.WriteString("Some checks failed; apply the fixes above.\n")
	}
	rep.Text = b.String()
	return rep
}
