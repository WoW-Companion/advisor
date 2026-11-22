package digest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SlotAddon is the data-only addon the digest is written into.
const SlotAddon = "WoWCompanion_Digest"

// DigestPath derives the digest file from a SavedVariables path:
// <game>/WTF/Account/<acct>/SavedVariables/<file> gives <game>/Interface/AddOns/WoWCompanion_Digest/Digest.lua.
// Both Windows and Unix paths are accepted, and the result keeps the input's separator.
func DigestPath(savedVariables string) (string, error) {
	if p, ok := SlotPath(savedVariables, SlotAddon, "Digest.lua"); ok {
		return p, nil
	}
	return "", fmt.Errorf("cannot derive the digest path from %q: expected <game>/WTF/Account/<account>/SavedVariables/<file>; use --digest-path", savedVariables)
}

// SlotPath derives <game>/Interface/AddOns/<addon>/<file> from a SavedVariables path; ok is false
// when the path is not <game>/WTF/Account/<account>/SavedVariables/<file>.
func SlotPath(savedVariables, addon, file string) (path string, ok bool) {
	sep := "/"
	if strings.Contains(savedVariables, `\`) && !strings.Contains(savedVariables, "/") {
		sep = `\`
	}
	segs := strings.Split(strings.ReplaceAll(savedVariables, `\`, "/"), "/")
	// The file is the last segment: WTF, Account, <acct>, SavedVariables, <file>.
	if i := len(segs) - 5; i >= 0 && strings.EqualFold(segs[i], "WTF") && strings.EqualFold(segs[i+1], "Account") &&
		segs[i+2] != "" && strings.EqualFold(segs[i+3], "SavedVariables") && segs[i+4] != "" {
		game := strings.Join(segs[:i], sep)
		return strings.Join([]string{game, "Interface", "AddOns", addon, file}, sep), true
	}
	return "", false
}

// CheckInstalled is an error when the slot addon's folder is not there. Nothing is created.
func CheckInstalled(digestPath string) error {
	dir := filepath.Dir(digestPath)
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return fmt.Errorf("the %s addon is not installed (no folder %s); install it from the WoW Companion addon repo", SlotAddon, dir)
	}
	return nil
}

// Write replaces path with the envelope atomically: a temp file in the same folder, then a rename.
// The folder must exist.
func Write(path string, d Envelope) error { return WriteFile(path, d.Lua()) }

// WriteFile replaces path with text atomically, as Write does.
func WriteFile(path, text string) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".Digest-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.WriteString(text)
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

var generatedRE = regexp.MustCompile(`generated = \d+,`)

// Same reports whether the file at path already holds the envelope, ignoring the generated times
// (they change on every build, so a rebuild of the same advice would never match byte for byte).
func Same(path string, d Envelope) bool { return SameText(path, d.Lua()) }

// SameText is Same for any data-only Lua text carrying a `generated = N,` line.
func SameText(path, text string) bool {
	old, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	norm := func(s string) string { return generatedRE.ReplaceAllString(s, "generated = 0,") }
	return norm(string(old)) == norm(text)
}

// SiblingPath is the file named file in the folder of the digest file at path, keeping its separator.
func SiblingPath(path, file string) string {
	i := strings.LastIndexAny(path, `/\`)
	if i < 0 {
		return file
	}
	return path[:i+1] + file
}
