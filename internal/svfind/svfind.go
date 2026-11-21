// Package svfind finds ForeverScribe.lua in the standard WoW: Forever install folders.
package svfind

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

// Flavors are the game folders searched under a WoW root, in preference order.
var Flavors = []string{"_forever_", "_classic_era_", "_retail_"}

// Roots lists the WoW install folders to search, in order, for an OS. driveExists says whether a
// Windows drive root such as `D:\` is present; it is not used on other systems.
func Roots(goos string, driveExists func(root string) bool) []string {
	switch goos {
	case "windows":
		var roots []string
		add := func(drive string) {
			roots = append(roots, drive+`\Program Files (x86)\World of Warcraft`, drive+`\Program Files\World of Warcraft`)
		}
		add("C:")
		for d := 'A'; d <= 'Z'; d++ {
			if d != 'C' && driveExists(string(d)+`:\`) {
				add(string(d) + ":")
			}
		}
		return roots
	case "darwin":
		return []string{"/Applications/World of Warcraft"}
	}
	return nil
}

// Result is the outcome of a search.
type Result struct {
	Path   string   // the file picked; empty when none was found
	Others []string // the other matches in the same game folder, newest first
	Tried  []string // the WoW roots that were searched
}

// Find looks under each root, game folder by game folder in Flavors order, for
// WTF/Account/*/SavedVariables/ForeverScribe.lua. The first game folder with a match wins, and its
// most recently modified file is the pick.
func Find(roots []string) Result {
	res := Result{Tried: roots}
	for _, root := range roots {
		for _, flavor := range Flavors {
			pattern := filepath.Join(root, flavor, "WTF", "Account", "*", "SavedVariables", "ForeverScribe.lua")
			matches, _ := filepath.Glob(pattern)
			type cand struct {
				path string
				mod  time.Time
			}
			var cands []cand
			for _, m := range matches {
				if st, err := os.Stat(m); err == nil && st.Mode().IsRegular() {
					cands = append(cands, cand{m, st.ModTime()})
				}
			}
			if len(cands) == 0 {
				continue
			}
			sort.SliceStable(cands, func(i, j int) bool {
				if !cands[i].mod.Equal(cands[j].mod) {
					return cands[i].mod.After(cands[j].mod)
				}
				return cands[i].path < cands[j].path
			})
			res.Path = cands[0].path
			for _, c := range cands[1:] {
				res.Others = append(res.Others, c.path)
			}
			return res
		}
	}
	return res
}

// Detect searches the standard install folders of the running system.
func Detect() Result {
	return Find(Roots(runtime.GOOS, func(root string) bool {
		_, err := os.Stat(root)
		return err == nil
	}))
}

// Note is the one line saying which file was picked and how to override it; empty when none was.
func (r Result) Note(envName string) string {
	if r.Path == "" {
		return ""
	}
	s := "wowc-advisor: using " + r.Path + " (auto-detected"
	if len(r.Others) > 0 {
		s += "; other accounts, older: " + strings.Join(r.Others, ", ")
	}
	return s + "; override with --savedvariables FILE or " + envName + ")"
}

// NotFound is the error text for an empty search, naming the folders searched.
func (r Result) NotFound(envName string) string {
	s := "no SavedVariables file configured (set " + envName + ")"
	if len(r.Tried) == 0 {
		return s + "; auto-detect knows no standard WoW folder on this system"
	}
	return fmt.Sprintf("%s; no ForeverScribe.lua found under %s", s, strings.Join(r.Tried, ", "))
}
