package digest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SlotAddon is the data-only addon the digest is written into.
const SlotAddon = "WoWCompanion_Digest"

// DigestPath derives the digest file from a SavedVariables path:
// <game>/WTF/Account/<acct>/SavedVariables/<file> gives <game>/Interface/AddOns/WoWCompanion_Digest/Digest.lua.
// Both Windows and Unix paths are accepted, and the result keeps the input's separator.
func DigestPath(savedVariables string) (string, error) {
	sep := "/"
	if strings.Contains(savedVariables, `\`) && !strings.Contains(savedVariables, "/") {
		sep = `\`
	}
	segs := strings.Split(strings.ReplaceAll(savedVariables, `\`, "/"), "/")
	// The file is the last segment: WTF, Account, <acct>, SavedVariables, <file>.
	if i := len(segs) - 5; i >= 0 && strings.EqualFold(segs[i], "WTF") && strings.EqualFold(segs[i+1], "Account") &&
		segs[i+2] != "" && strings.EqualFold(segs[i+3], "SavedVariables") && segs[i+4] != "" {
		game := strings.Join(segs[:i], sep)
		return strings.Join([]string{game, "Interface", "AddOns", SlotAddon, "Digest.lua"}, sep), nil
	}
	return "", fmt.Errorf("cannot derive the digest path from %q: expected <game>/WTF/Account/<account>/SavedVariables/<file>; use --digest-path", savedVariables)
}

// CheckInstalled is an error when the slot addon's folder is not there. Nothing is created.
func CheckInstalled(digestPath string) error {
	dir := filepath.Dir(digestPath)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("the %s addon is not installed (no folder %s); install it from the WoW Companion addon repo", SlotAddon, dir)
	}
	return nil
}

// Write replaces path with the digest atomically: a temp file in the same folder, then a rename.
// The folder must exist.
func Write(path string, d Digest) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".Digest-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(d.Lua())
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr, os.Chmod(tmp.Name(), 0o644)); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
