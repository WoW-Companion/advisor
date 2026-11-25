// Package gamesetup checks the player's setup (game folder, ForeverScribe, the WoW Companion
// addon, display mode, the AI client) and installs or updates the addon on request.
//
// Checks only read. The one write is the addon install, confined to Interface/AddOns, and it
// runs only when the caller asks. Nothing here touches the game process, and WTF/Config.wtf is
// never modified.
package gamesetup

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/dataset"
	"github.com/WoW-Companion/advisor/internal/digest"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/release"
	"github.com/WoW-Companion/advisor/internal/svfind"
)

// Item statuses.
const (
	OK   = "ok"
	Warn = "warn"
	Bad  = "bad"
)

// Item ids.
const (
	IDGame      = "game"
	IDScribe    = "foreverscribe"
	IDData      = "data"
	IDDataset   = "dataset"
	IDAddon     = "addon"
	IDDisplay   = "display"
	IDQuestie   = "questie"
	IDAI        = "ai"
	AddonName   = "WoWCompanion"
	ScribeName  = "ForeverScribe"
	ScribeURL   = "https://github.com/MattMaximo/foreverscribe"
	addonTocFmt = "%s.toc"
)

// Actions an item's button may ask for.
const (
	ActChooseFolder = "choose-folder"
	ActOpenURL      = "open-url"
	ActInstallAddon = "install-addon"
	ActUpdateAddon  = "update-addon"
	ActTestAI       = "test-ai"
	ActWriteData    = "write-data"
)

// Item is one checklist row.
type Item struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   string `json:"status"` // ok, warn or bad
	Detail   string `json:"detail"`
	Optional bool   `json:"optional,omitempty"` // may be hidden with "Don't show again"
	Action   string `json:"action,omitempty"`   // the one button, if any
	Label    string `json:"label,omitempty"`    // the button's text
	URL      string `json:"url,omitempty"`      // for ActOpenURL
}

// Options says where to look. Latest is the newest addon release, nil when not looked up.
type Options struct {
	GameDir        string // the player's chosen WoW folder; empty to detect
	SavedVariables string // the configured ForeverScribe.lua or WoWCompanion.lua; used when GameDir is empty
	Latest         *AddonRelease
	LatestErr      string
	// Pulled are the addon versions the project pulled (updates/updates.json); Fallback is the release
	// of the version to offer instead when the newest, or the installed one, is pulled.
	Pulled   []release.Pull
	Fallback *AddonRelease
}

// Offer is the addon release the player may install, and the plan behind it. The release is nil
// when nothing is offered.
func (o Options) Offer(have string) (AddonOffer, *AddonRelease) {
	latest := ""
	if o.Latest != nil {
		latest = o.Latest.Version
	}
	plan := PlanAddon(have, latest, o.Pulled)
	switch {
	case plan.Version == "":
		return plan, nil
	case o.Latest != nil && plan.Version == o.Latest.Version:
		return plan, o.Latest
	case o.Fallback != nil && o.Fallback.Version == plan.Version:
		return plan, o.Fallback
	}
	return plan, nil
}

var flavorMarkers = []string{"Interface", "WTF"}

// GameFolder finds the game folder that holds Interface and WTF (for example
// C:\Program Files (x86)\World of Warcraft\_forever_). dir may be that folder or the WoW root.
func GameFolder(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	isGame := func(d string) bool {
		for _, m := range flavorMarkers {
			if st, err := os.Stat(filepath.Join(d, m)); err == nil && st.IsDir() {
				return true
			}
		}
		return false
	}
	if isGame(dir) {
		return dir, true
	}
	for _, f := range svfind.Flavors {
		if d := filepath.Join(dir, f); isGame(d) {
			return d, true
		}
	}
	return "", false
}

// Resolve picks the game folder: the chosen one, else the one the configured SavedVariables
// path sits in, else a standard install.
func Resolve(o Options) (string, bool) {
	if o.GameDir != "" {
		return GameFolder(o.GameDir)
	}
	if o.SavedVariables != "" {
		// <game>/Interface/AddOns/X/f derived from the SavedVariables path; two levels up is the game folder.
		if p, ok := digest.SlotPath(o.SavedVariables, "x", "f"); ok {
			game := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filepath.FromSlash(strings.ReplaceAll(p, `\`, "/"))))))
			if g, ok := GameFolder(game); ok {
				return g, true
			}
		}
	}
	for _, root := range svfind.Roots(goos, driveExists) {
		if g, ok := GameFolder(root); ok {
			return g, true
		}
	}
	return "", false
}

// Check runs every check except the AI client's, which needs the agent (see AIItem).
func Check(o Options) []Item {
	game, ok := Resolve(o)
	items := []Item{gameItem(game, ok)}
	if !ok {
		return append(items,
			Item{ID: IDScribe, Title: "ForeverScribe (optional)", Status: Warn, Optional: true, Detail: "Needs the game folder first."},
			Item{ID: IDData, Title: "Characters captured", Status: Bad, Detail: "Needs the game folder first."},
			Item{ID: IDAddon, Title: "WoW Companion addon", Status: Warn, Detail: "Needs the game folder first."},
			Item{ID: IDDisplay, Title: "Display mode", Status: Warn, Detail: "Needs the game folder first."},
			Item{ID: IDQuestie, Title: "Questie (optional)", Status: Warn, Optional: true, Detail: "Needs the game folder first."})
	}
	items = append(items, scribeItem(game), dataItem(game), addonItem(game, o), displayItem(game), questieItem(game))
	return items
}

func gameItem(game string, ok bool) Item {
	it := Item{ID: IDGame, Title: "WoW: Forever found"}
	if ok {
		it.Status, it.Detail = OK, game
		return it
	}
	it.Status, it.Detail = Bad, "No World of Warcraft folder found in the usual places. Choose the folder that holds _forever_."
	it.Action, it.Label = ActChooseFolder, "Choose folder…"
	return it
}

// RemoveScribeNote is the one line said when ForeverScribe is the only capture source.
const RemoveScribeNote = "You can remove ForeverScribe once WoW Companion 0.6.N+ is installed."

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

// scribeItem: ForeverScribe is optional; the WoW Companion addon records the characters itself.
func scribeItem(game string) Item {
	it := Item{ID: IDScribe, Title: "ForeverScribe (optional)", Optional: true}
	scribe := dirExists(filepath.Join(game, "Interface", "AddOns", ScribeName))
	_, addon := InstalledAddonVersion(game)
	switch {
	case scribe && addon:
		it.Status, it.Detail = OK, "Installed, but no longer needed: the WoW Companion addon records your characters. You can remove it."
	case scribe:
		it.Status, it.Detail = Warn, RemoveScribeNote
	case addon:
		it.Status, it.Detail = OK, "Not needed: the WoW Companion addon records your characters."
	default:
		it.Status, it.Detail = OK, "Not needed. Install the WoW Companion addon below, which records your characters."
	}
	return it
}

// svFiles lists the character files (ForeverScribe.lua and WoWCompanion.lua) under
// <game>/WTF/Account/*/SavedVariables.
func svFiles(game string) []string {
	var out []string
	for _, n := range []string{character.WoWCFile, character.FSFile} {
		m, _ := filepath.Glob(filepath.Join(game, "WTF", "Account", "*", "SavedVariables", n))
		out = append(out, m...)
	}
	return out
}

func dataItem(game string) Item {
	it := Item{ID: IDData, Title: "Characters captured"}
	const hint = "Log in to a character and type /reload once."
	files := svFiles(game)
	if len(files) == 0 {
		it.Status, it.Detail = Bad, "No character saved yet. "+hint
		return it
	}
	for _, f := range files {
		if c, err := character.LoadFile(f); err == nil && len(c.Characters) > 0 {
			it.Status, it.Detail = OK, fmt.Sprintf("%d character(s) captured%s.", len(c.Characters), sourceSummary(c))
			return it
		}
	}
	it.Status, it.Detail = Bad, "No character captured yet. "+hint
	return it
}

// sourceSummary says, per source, how many characters came from it.
func sourceSummary(f character.File) string {
	n := map[string]int{}
	var order []string
	for _, c := range f.Characters {
		if n[c.Source] == 0 {
			order = append(order, c.Source)
		}
		n[c.Source]++
	}
	var parts []string
	for _, s := range order {
		parts = append(parts, fmt.Sprintf("%d from %s", n[s], s))
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

var tocVersion = regexp.MustCompile(`(?m)^##\s*Version:\s*(\S.*?)\s*$`)

// InstalledAddonVersion is the installed WoWCompanion.toc version; ok is false when not installed.
func InstalledAddonVersion(game string) (string, bool) {
	data, err := os.ReadFile(filepath.Join(game, "Interface", "AddOns", AddonName, AddonName+".toc"))
	if err != nil {
		return "", false
	}
	if m := tocVersion.FindSubmatch(data); m != nil {
		return string(m[1]), true
	}
	return "unknown", true
}

// DataFiles are the files the digest addon's TOC lists, written by the advisor.
var DataFiles = []string{"Digest.lua", "Plan.lua"}

// missingData lists the data files absent from the digest addon's folder, when that addon is installed.
func missingData(game string) []string {
	dir := filepath.Join(game, "Interface", "AddOns", digest.SlotAddon)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return nil
	}
	var out []string
	for _, f := range DataFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			out = append(out, f)
		}
	}
	return out
}

func addonItem(game string, o Options) Item {
	it := Item{ID: IDAddon, Title: "WoW Companion addon"}
	have, installed := InstalledAddonVersion(game)
	if !installed {
		have = ""
	}
	plan, rel := o.Offer(have)
	latest := ""
	if rel != nil {
		latest = rel.Version
	}
	_, installedPulled := pullFor(o.Pulled, have)
	if !installed {
		installedPulled = false
	}
	switch {
	case installed && installedPulled && latest != "":
		it.Status, it.Detail = Warn, "Installed "+have+", which has a known problem"+reasonSuffix(plan.Reason)+". Version "+latest+" is safe to go back to."
		it.Action, it.Label = ActUpdateAddon, "Reinstall "+latest
	case installed && installedPulled:
		it.Status, it.Detail = Warn, "Installed "+have+", which has a known problem"+reasonSuffix(plan.Reason)+". No safe version is available to go back to yet."
	case installed && latest == "" && len(missingData(game)) > 0:
		it.Status, it.Detail = Warn, "Installed, version "+have+", but "+strings.Join(missingData(game), " and ")+" missing: the game will warn at login."
		it.Action, it.Label = ActWriteData, "Write now"
	case installed && latest == "":
		it.Status, it.Detail = OK, "Installed, version "+have+"."
		if o.Latest == nil && o.LatestErr != "" {
			it.Detail += " Could not check for updates: " + o.LatestErr
		}
	case installed:
		it.Status, it.Detail = Warn, "Installed "+have+", latest "+latest+"."
		it.Action, it.Label = ActUpdateAddon, "Update"
	case latest != "":
		it.Status, it.Detail = Warn, "Not installed. Latest is "+latest+". Restart the game fully after the first install."
		it.Action, it.Label = ActInstallAddon, "Install"
	case o.Latest != nil:
		it.Status, it.Detail = Warn, "Not installed; the newest release has a known problem and no safe version is available yet."
	default:
		it.Status, it.Detail = Warn, "Not installed; could not look up the latest release: "+o.LatestErr
		it.Action, it.Label = ActInstallAddon, "Install"
	}
	return it
}

func reasonSuffix(r string) string {
	if r == "" {
		return ""
	}
	return " (" + r + ")"
}

// Newer reports whether version a is newer than b, by dotted numbers; an unreadable b counts as older.
func Newer(a, b string) bool {
	pa, pb := nums(a), nums(b)
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			return x > y
		}
	}
	return false
}

func nums(v string) []int {
	var out []int
	for _, p := range strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".") {
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	return out
}

// Display is what Config.wtf says about the window mode.
type Display struct {
	Known      bool
	Exclusive  bool // exclusive fullscreen: the overlay cannot draw over it
	Mode       string
	Unreadable error
}

// ReadDisplay reads <game>/WTF/Config.wtf, read-only. The client writes lines like
// `SET gxWindow "1"` and `SET gxMaximize "1"`: gxWindow 0 is exclusive fullscreen; gxWindow 1 is
// a window, borderless ("Windowed (Fullscreen)") when gxMaximize is 1.
func ReadDisplay(game string) Display {
	f, err := os.Open(filepath.Join(game, "WTF", "Config.wtf"))
	if err != nil {
		return Display{Unreadable: err}
	}
	defer f.Close()
	set := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fl := strings.Fields(sc.Text())
		if len(fl) >= 3 && strings.EqualFold(fl[0], "SET") {
			set[strings.ToLower(fl[1])] = strings.Trim(strings.Join(fl[2:], " "), `"`)
		}
	}
	win, ok := set["gxwindow"]
	if !ok {
		return Display{}
	}
	switch {
	case win == "0":
		return Display{Known: true, Exclusive: true, Mode: "Fullscreen"}
	case set["gxmaximize"] == "1":
		return Display{Known: true, Mode: "Windowed (Fullscreen)"}
	}
	return Display{Known: true, Mode: "Windowed"}
}

func displayItem(game string) Item {
	it := Item{ID: IDDisplay, Title: "Display mode"}
	d := ReadDisplay(game)
	switch {
	case d.Unreadable != nil:
		it.Status, it.Detail = Warn, "Could not read WTF/Config.wtf yet (it appears after the game has run once)."
	case d.Exclusive:
		it.Status = Warn
		it.Detail = "The overlay can't draw over exclusive fullscreen. In WoW: System › Graphics › Display Mode › Windowed (Fullscreen)."
	case !d.Known:
		it.Status, it.Detail = OK, "No fullscreen setting found; the game defaults to a window."
	default:
		it.Status, it.Detail = OK, d.Mode+"."
	}
	return it
}

// ActDownloadData asks the sidecar to fetch the game dataset (the one write besides the addon install).
const ActDownloadData = "download-dataset"

// DatasetState is what the Game data row needs: the capture's client, the verified cached builds
// (newest first), the last failed download and whether one is running.
type DatasetState struct {
	Client character.Client
	Cached []string
	Failed string
	Busy   bool
	// Revision is the day the matching cached dataset was published (empty when unknown).
	Revision string
}

// DatasetItem is the "Game data" row. The dataset downloads by itself on first need; this shows
// whether it has and lets the player retry.
func DatasetItem(st DatasetState) Item {
	it := Item{ID: IDDataset, Title: "Game data"}
	game := st.Client.Build
	if game != "" && st.Client.Version != "" {
		game = st.Client.Version + "." + st.Client.Build
	}
	var match string
	for _, b := range st.Cached {
		if game == "" || dataset.Matches(b, st.Client.Version, st.Client.Build) {
			match = b
			break
		}
	}
	rev := ""
	if st.Revision != "" {
		rev = ", updated " + st.Revision
	}
	switch {
	case st.Busy:
		it.Status, it.Detail = Warn, "Downloading the game data (under 1 MB)…"
	case match != "" && game == "":
		it.Status, it.Detail = OK, "Downloaded (build "+match+rev+")."
	case match != "":
		it.Status, it.Detail = OK, "Downloaded (build "+match+", matches your game"+rev+")."
	case len(st.Cached) > 0:
		it.Status = Warn
		it.Detail = "Downloaded for build " + st.Cached[0] + "; your game is build " + game + "; a newer dataset will download when it's published."
	case st.Failed != "":
		it.Status, it.Detail = Bad, "Download failed: "+oneLine(st.Failed)
		it.Action, it.Label = ActDownloadData, "Retry"
	default:
		it.Status, it.Detail = Bad, "Not downloaded yet. It downloads by itself the first time the advisor needs it."
		it.Action, it.Label = ActDownloadData, "Download now"
	}
	return it
}

// oneLine keeps a failure reason to one short line.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 160 {
		s = string(r[:157]) + "..."
	}
	return s
}

// questieItem reports the player's own Questie, which the advisor reads when present. It is never
// an error: without it, quest suggestions come from ForeverScribe's saved data alone.
func questieItem(game string) Item {
	it := Item{ID: IDQuestie, Title: "Questie (optional)", Optional: true}
	ok, detail := questie.Probe(game).Describe()
	it.Detail = detail
	if ok {
		it.Status = OK
	} else {
		it.Status = Warn
	}
	return it
}
