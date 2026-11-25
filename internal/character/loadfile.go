package character

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/WoW-Companion/advisor/internal/lua"
)

// File names beside each other in one SavedVariables folder.
const (
	FSFile   = "ForeverScribe.lua"
	WoWCFile = "WoWCompanion.lua"
)

// Siblings returns the ForeverScribe and the WoW Companion file in the folder of path. A path
// named WoWCompanion.lua is the addon's file; any other name is taken as ForeverScribe's.
func Siblings(path string) (fsPath, wowcPath string) {
	dir := filepath.Dir(path)
	if strings.EqualFold(filepath.Base(path), WoWCFile) {
		return filepath.Join(dir, FSFile), path
	}
	return path, filepath.Join(dir, WoWCFile)
}

// readVars parses one file; ok is false when it is missing or malformed (err says which).
func readVars(path string) (vars map[string]any, ok bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false, err
	}
	vars, err = lua.Parse(string(data))
	return vars, err == nil, err
}

// LoadFile reads the character captures beside path: the WoW Companion addon's
// (WoWCompanionDB.characters in WoWCompanion.lua) and ForeverScribe's. A character in both is
// merged: the newer capture leads and the other fills the parts it lacks. A file that is missing
// or unreadable is skipped when the other gives characters; otherwise the error is the one for
// path. A malformed file is a *lua.ParseError.
func LoadFile(path string) (File, error) {
	fsPath, wowcPath := Siblings(path)
	var out File
	var errs = map[string]error{}
	if vars, ok, err := readVars(wowcPath); ok {
		if f, lerr := LoadWoWC(vars); lerr == nil {
			out, out.HasWoWC = f, true
		} else {
			errs[wowcPath] = lerr
		}
	} else {
		errs[wowcPath] = err
	}
	if vars, ok, err := readVars(fsPath); ok {
		if f, lerr := Load(vars); lerr == nil {
			f.HasFS = true
			out = mergeFiles(out, f)
		} else {
			errs[fsPath] = lerr
		}
	} else {
		errs[fsPath] = err
	}
	if out.HasWoWC || out.HasFS {
		return out, nil
	}
	// Report path's own error; a missing sibling is no news.
	if err := errs[path]; err != nil {
		return File{}, err
	}
	for _, err := range errs {
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return File{}, err
		}
	}
	return File{}, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
}
