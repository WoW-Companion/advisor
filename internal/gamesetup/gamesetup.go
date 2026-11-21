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
	SavedVariables string // the configured ForeverScribe.lua; used when GameDir is empty
	Latest         *AddonRelease
	LatestErr      string
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
			Item{ID: IDScribe, Title: "ForeverScribe installed", Status: Bad, Detail: "Needs the game folder first."},
			Item{ID: IDData, Title: "ForeverScribe has data", Status: Bad, Detail: "Needs the game folder first."},
			Item{ID: IDAddon, Title: "WoW Companion addon (optional)", Status: Warn, Optional: true, Detail: "Needs the game folder first."},
			Item{ID: IDDisplay, Title: "Display mode", Status: Warn, Detail: "Needs the game folder first."})
	}
	items = append(items, scribeItem(game), dataItem(game), addonItem(game, o), displayItem(game))
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

func scribeItem(game string) Item {
	it := Item{ID: IDScribe, Title: "ForeverScribe installed"}
	if st, err := os.Stat(filepath.Join(game, "Interface", "AddOns", ScribeName)); err == nil && st.IsDir() {
		it.Status, it.Detail = OK, "Found in Interface/AddOns."
		return it
	}
	// ForeverScribe publishes no release zip, so there is nothing to fetch: link to its page.
	it.Status, it.Detail = Bad, "ForeverScribe is not in Interface/AddOns. Install it from its project page, then restart the game."
	it.Action, it.Label, it.URL = ActOpenURL, "Open ForeverScribe page", ScribeURL
	return it
}

// svFiles lists ForeverScribe.lua under <game>/WTF/Account/*/SavedVariables.
func svFiles(game string) []string {
	m, _ := filepath.Glob(filepath.Join(game, "WTF", "Account", "*", "SavedVariables", "ForeverScribe.lua"))
	return m
}

func dataItem(game string) Item {
	it := Item{ID: IDData, Title: "ForeverScribe has data"}
	const hint = "Log in to a character and type /reload once."
	files := svFiles(game)
	if len(files) == 0 {
		it.Status, it.Detail = Bad, "No ForeverScribe.lua saved yet. "+hint
		return it
	}
	for _, f := range files {
		if c, err := character.LoadFile(f); err == nil && len(c.Characters) > 0 {
			it.Status, it.Detail = OK, fmt.Sprintf("%d character(s) captured.", len(c.Characters))
			return it
		}
	}
	it.Status, it.Detail = Bad, "ForeverScribe.lua holds no character yet. "+hint
	return it
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

func addonItem(game string, o Options) Item {
	it := Item{ID: IDAddon, Title: "WoW Companion addon (optional)", Optional: true}
	have, installed := InstalledAddonVersion(game)
	latest := ""
	if o.Latest != nil {
		latest = o.Latest.Version
	}
	switch {
	case installed && (latest == "" || !Newer(latest, have)):
		it.Status, it.Detail = OK, "Installed, version "+have+"."
		if latest == "" && o.LatestErr != "" {
			it.Detail += " Could not check for updates: " + o.LatestErr
		}
	case installed:
		it.Status, it.Detail = Warn, "Installed "+have+", latest "+latest+"."
		it.Action, it.Label = ActUpdateAddon, "Update"
	case latest != "":
		it.Status, it.Detail = Warn, "Not installed. Latest is "+latest+". Restart the game fully after the first install."
		it.Action, it.Label = ActInstallAddon, "Install"
	default:
		it.Status, it.Detail = Warn, "Not installed; could not look up the latest release: "+o.LatestErr
		it.Action, it.Label = ActInstallAddon, "Install"
	}
	return it
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
	switch {
	case st.Busy:
		it.Status, it.Detail = Warn, "Downloading the game data (under 1 MB)…"
	case match != "" && game == "":
		it.Status, it.Detail = OK, "Downloaded (build "+match+")."
	case match != "":
		it.Status, it.Detail = OK, "Downloaded (build "+match+", matches your game)."
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
