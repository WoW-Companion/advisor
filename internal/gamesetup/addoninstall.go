package gamesetup

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/release"
)

// KeepBackups is how many previous addon versions are kept for going back.
const KeepBackups = 3

// MinAddonVersion is the oldest addon this app works with. An installed addon older than this gets a
// plain notice and an install offer that skips the settle delay.
const MinAddonVersion = "0.6.0"

// NoGameMessage is what every addon update action says when the game folder is not known.
const NoGameMessage = "The WoW folder was not found. Choose it in the setup list first; nothing was changed."

// isDataFile is true for a file the app itself writes into the addon folders (as a slash path
// relative to AddOns). An update never replaces or removes these.
func isDataFile(rel string) bool {
	top, name, ok := strings.Cut(path.Clean(rel), "/")
	if !ok || !strings.EqualFold(top, "WoWCompanion_Digest") {
		return false
	}
	for _, d := range DataFiles {
		if strings.EqualFold(name, d) {
			return true
		}
	}
	return false
}

func haveVersion(game string) string {
	v, ok := InstalledAddonVersion(game)
	if !ok || v == "unknown" {
		return ""
	}
	return v
}

type installOpts struct {
	Have    string // the installed version, "" when none: what a backup is named
	Backups string // where backups live; "" keeps none
}

// InstallZip puts a verified release zip in <game>/Interface/AddOns: it unpacks to a staging folder,
// keeps a copy of the version being replaced, then replaces the addon folders file by file. Files
// the new version no longer ships are removed; the app's data files are never touched. Only the
// AddOns folder changes (the backups and the staging folder live under opts.Backups).
func InstallZip(zipb []byte, game string, o installOpts) (InstallResult, error) {
	addons := filepath.Join(game, "Interface", "AddOns")
	if st, err := os.Stat(game); err != nil || !st.IsDir() {
		return InstallResult{}, errors.New(NoGameMessage)
	}
	stageRoot := o.Backups
	if stageRoot == "" {
		stageRoot = os.TempDir()
	}
	if err := os.MkdirAll(stageRoot, 0o755); err != nil {
		return InstallResult{}, err
	}
	stage, err := os.MkdirTemp(stageRoot, "staging-")
	if err != nil {
		return InstallResult{}, err
	}
	defer os.RemoveAll(stage)
	if err := extractZip(zipb, stage, true); err != nil {
		return InstallResult{}, err
	}
	before, berr := snapshotAddons(addons)
	if err := backupCurrent(addons, o); err != nil {
		return InstallResult{}, err
	}
	if err := syncAddons(stage, addons); err != nil {
		return InstallResult{}, err
	}
	return resultFor(before, berr, addons), nil
}

// topFolders lists the addon folders (WoWCompanion*) directly under dir.
func topFolders(dir string) []string {
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		if e.IsDir() && allowedTop(e.Name()) {
			out = append(out, e.Name())
		}
	}
	return out
}

func backupCurrent(addons string, o installOpts) error {
	if o.Backups == "" || o.Have == "" || strings.ContainsAny(o.Have, `/\`) || strings.Contains(o.Have, "..") {
		return nil
	}
	dest := filepath.Join(o.Backups, o.Have)
	tmp := dest + ".part"
	os.RemoveAll(tmp)
	for _, top := range topFolders(addons) {
		if err := copyTree(filepath.Join(addons, top), filepath.Join(tmp, top), top); err != nil {
			os.RemoveAll(tmp)
			return fmt.Errorf("could not keep a copy of the current addon: %w", err)
		}
	}
	if _, err := os.Stat(tmp); err != nil {
		return nil // nothing installed to keep
	}
	os.RemoveAll(dest)
	if err := os.Rename(tmp, dest); err != nil {
		os.RemoveAll(tmp)
		return err
	}
	return pruneBackups(o.Backups)
}

// copyTree copies src to dst, leaving the data files out. top is src's folder name under AddOns.
func copyTree(src, dst, top string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		if !d.Type().IsRegular() || isDataFile(top+"/"+filepath.ToSlash(rel)) {
			return nil
		}
		return copyFile(p, filepath.Join(dst, rel))
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".install-*.tmp")
	if err != nil {
		return err
	}
	_, cerr := io.Copy(tmp, in)
	if e := tmp.Close(); cerr == nil {
		cerr = e
	}
	if cerr == nil {
		cerr = os.Chmod(tmp.Name(), 0o644)
	}
	if cerr == nil {
		cerr = os.Rename(tmp.Name(), dst)
	}
	if cerr != nil {
		os.Remove(tmp.Name())
	}
	return cerr
}

// syncAddons makes each addon folder under src (a staging or backup folder) what is in addons: its
// files are copied in, and files it does not hold are removed, data files excepted.
func syncAddons(src, addons string) error {
	for _, top := range topFolders(src) {
		keep := map[string]bool{}
		root := filepath.Join(src, top)
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !d.Type().IsRegular() {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			keep[filepath.ToSlash(rel)] = true
			return copyFile(p, filepath.Join(addons, top, rel))
		})
		if err != nil {
			return err
		}
		dest := filepath.Join(addons, top)
		var dirs []string
		filepath.WalkDir(dest, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(dest, p)
			rel = filepath.ToSlash(rel)
			switch {
			case rel == ".":
			case d.IsDir():
				dirs = append(dirs, p)
			case !keep[rel] && !isDataFile(top+"/"+rel) && !strings.HasPrefix(d.Name(), ".install-"):
				os.Remove(p)
			}
			return nil
		})
		for i := len(dirs) - 1; i >= 0; i-- {
			os.Remove(dirs[i]) // only succeeds when empty
		}
	}
	return nil
}

// Backups lists the kept versions, newest first.
func Backups(dir string) []string {
	ents, _ := os.ReadDir(dir)
	var out []string
	for _, e := range ents {
		if _, ok := release.Compare(e.Name(), "0"); e.IsDir() && ok && !strings.HasSuffix(e.Name(), ".part") {
			out = append(out, e.Name())
		}
	}
	sort.Slice(out, func(i, j int) bool { c, _ := release.Compare(out[i], out[j]); return c > 0 })
	return out
}

func pruneBackups(dir string) error {
	b := Backups(dir)
	for i := KeepBackups; i < len(b); i++ {
		if err := os.RemoveAll(filepath.Join(dir, b[i])); err != nil {
			return err
		}
	}
	return nil
}

// GoBack restores a kept version. The version being left is kept in its place.
func GoBack(game, backups, version string) (InstallResult, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if st, err := os.Stat(game); err != nil || !st.IsDir() {
		return InstallResult{}, errors.New(NoGameMessage)
	}
	if version == "" || strings.ContainsAny(version, `/\`) || strings.Contains(version, "..") {
		return InstallResult{}, errors.New("that version is not one of the kept ones")
	}
	src := filepath.Join(backups, version)
	if st, err := os.Stat(src); err != nil || !st.IsDir() {
		return InstallResult{}, fmt.Errorf("version %s is not kept on this computer", version)
	}
	addons := filepath.Join(game, "Interface", "AddOns")
	have := haveVersion(game)
	if have == version {
		return InstallResult{}, fmt.Errorf("version %s is already installed", version)
	}
	before, berr := snapshotAddons(addons)
	if err := backupCurrent(addons, installOpts{Have: have, Backups: backups}); err != nil {
		return InstallResult{}, err
	}
	if err := syncAddons(src, addons); err != nil {
		return InstallResult{}, err
	}
	return resultFor(before, berr, addons), nil
}

// AddonStatus is what the update notice and Settings need about the installed addon.
type AddonStatus struct {
	Game       string   `json:"-"`
	Message    string   `json:"message,omitempty"`   // plain words when there is nothing to check (no game folder, not installed)
	Installed  string   `json:"installed,omitempty"` // "" when the addon is not installed
	Available  bool     `json:"available"`           // Latest is offered now (the settle delay is over, or it is needed)
	Latest     string   `json:"latest,omitempty"`
	Published  string   `json:"published,omitempty"`
	Reinstall  bool     `json:"reinstall,omitempty"` // the installed version is pulled; Latest is the one to go back to
	Reason     string   `json:"reason,omitempty"`
	NeedsNewer bool     `json:"needs_newer,omitempty"` // the app needs a newer addon than the installed one
	Min        string   `json:"min,omitempty"`
	Notes      string   `json:"notes,omitempty"`
	Back       []string `json:"back,omitempty"` // kept older versions to go back to
	Auto       string   `json:"auto,omitempty"` // what the automatic installer should do: "install" or "goback"
}

// Due decides whether an offered release is shown now. A pulled-version go-back and a version the app
// needs come at once; anything else waits SettleDelay after its release, unless the player asked
// for updates right away.
func Due(offer AddonOffer, rel *AddonRelease, needsNewer, immediate bool, now time.Time) bool {
	if offer.Version == "" || rel == nil {
		return false
	}
	if offer.Reinstall || needsNewer || immediate {
		return true
	}
	return !rel.Published.IsZero() && now.Sub(rel.Published) >= release.SettleDelay
}

// StatusFor builds the addon status from the installed folder, the lookups in o and the kept versions.
func StatusFor(o Options, backups string, immediate bool, now time.Time) AddonStatus {
	game, ok := Resolve(o)
	if !ok {
		return AddonStatus{Message: NoGameMessage}
	}
	st := AddonStatus{Game: game}
	have, installed := InstalledAddonVersion(game)
	if !installed {
		st.Message = "The addon is not installed yet. Use the setup list to install it."
		return st
	}
	if have == "unknown" {
		st.Installed = have
		return st
	}
	st.Installed, st.Min = have, MinAddonVersion
	if c, ok := release.Compare(have, MinAddonVersion); ok && c < 0 {
		st.NeedsNewer = true
	}
	offer, rel := o.Offer(have)
	if rel != nil && Due(offer, rel, st.NeedsNewer, immediate, now) {
		st.Available, st.Latest, st.Reinstall, st.Reason = true, rel.Version, offer.Reinstall, offer.Reason
		if !rel.Published.IsZero() {
			st.Published = rel.Published.UTC().Format(time.RFC3339)
		}
		st.Auto = "install"
		if offer.Reinstall {
			st.Auto = "goback"
		}
	}
	for _, v := range Backups(backups) {
		if c, ok := release.Compare(v, have); ok && c < 0 {
			st.Back = append(st.Back, v)
		}
	}
	return st
}

// tocState is what the game reads at start from one TOC: its file entries and the metadata lines that
// shape loading (## Interface, ## SavedVariables*, ## Dependencies).
type tocState struct {
	files map[string]bool
	meta  map[string]bool
}

// addonState maps addon folder -> TOC file name -> state.
type addonState map[string]map[string]tocState

// snapshotAddons reads every WoWCompanion* folder's TOC files under addons. A missing AddOns folder is
// an empty state; any other read failure is an error.
func snapshotAddons(addons string) (addonState, error) {
	st := addonState{}
	ents, err := os.ReadDir(addons)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	for _, e := range ents {
		if !e.IsDir() || !allowedTop(e.Name()) {
			continue
		}
		files, err := os.ReadDir(filepath.Join(addons, e.Name()))
		if err != nil {
			return nil, err
		}
		tocs := map[string]tocState{}
		for _, f := range files {
			if f.IsDir() || !strings.EqualFold(filepath.Ext(f.Name()), ".toc") {
				continue
			}
			b, err := os.ReadFile(filepath.Join(addons, e.Name(), f.Name()))
			if err != nil {
				return nil, err
			}
			tocs[f.Name()] = parseTOC(b)
		}
		st[e.Name()] = tocs
	}
	return st, nil
}

func parseTOC(b []byte) tocState {
	t := tocState{files: map[string]bool{}, meta: map[string]bool{}}
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(sc.Text(), "\xef\xbb\xbf"))
		switch {
		case line == "":
		case strings.HasPrefix(line, "##"):
			k, v, _ := strings.Cut(strings.TrimSpace(strings.TrimPrefix(line, "##")), ":")
			k = strings.ToLower(strings.TrimSpace(k))
			if k == "interface" || k == "dependencies" || strings.HasPrefix(k, "savedvariables") {
				t.meta[k+": "+strings.Join(strings.Fields(v), " ")] = true
			}
		case strings.HasPrefix(line, "#"):
		default:
			t.files[strings.ToLower(strings.ReplaceAll(line, "\\", "/"))] = true
		}
	}
	return t
}

// needsRestart says whether going from before to after adds anything /reload cannot load: a folder, a
// TOC, a file entry or a metadata line that was not there. Removals and content changes do not count.
func needsRestart(before, after addonState) bool {
	for folder, tocs := range after {
		bt, ok := before[folder]
		if !ok {
			return true
		}
		for name, t := range tocs {
			b, ok := bt[name]
			if !ok {
				return true
			}
			for f := range t.files {
				if !b.files[f] {
					return true
				}
			}
			for m := range t.meta {
				if !b.meta[m] {
					return true
				}
			}
		}
	}
	return false
}

// resultFor judges an install that has just synced into addons. An unreadable before or after state
// counts as a restart, the safe side.
func resultFor(before addonState, berr error, addons string) InstallResult {
	after, aerr := snapshotAddons(addons)
	restart := berr != nil || aerr != nil || needsRestart(before, after)
	return InstallResult{Note: ReloadNote(restart), Restart: restart}
}
