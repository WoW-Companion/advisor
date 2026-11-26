package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/WoW-Companion/advisor/internal/whatsnew"
)

// versionFiles are the files whose version must equal VERSION, each with the pattern that finds it.
var versionFiles = []struct {
	path string
	re   *regexp.Regexp
}{
	{"overlay/src-tauri/tauri.conf.json", regexp.MustCompile(`("version":\s*")[^"]+(")`)},
	{"overlay/src-tauri/Cargo.toml", regexp.MustCompile(`(?m)(^version = ")[^"]+(")`)},
	{"overlay/src-tauri/Cargo.lock", regexp.MustCompile(`(name = "wow-companion"\nversion = ")[^"]+(")`)},
}

// releaseFiles are all the paths a release-prep commit may change besides the fragments.
func releaseFiles() map[string]bool {
	m := map[string]bool{"VERSION": true, "CHANGELOG.md": true}
	for _, f := range versionFiles {
		m[f.path] = true
	}
	return m
}

func nextVersion(cur, bump string) (string, error) {
	p := strings.Split(strings.TrimSpace(cur), ".")
	if len(p) != 3 {
		return "", fmt.Errorf("VERSION %q is not major.minor.patch", cur)
	}
	n := make([]int, 3)
	for i := range p {
		v, err := strconv.Atoi(p[i])
		if err != nil {
			return "", fmt.Errorf("VERSION %q is not major.minor.patch", cur)
		}
		n[i] = v
	}
	switch bump {
	case "", "patch":
		n[2]++
	case "minor":
		n[1]++
		n[2] = 0
	default:
		return "", fmt.Errorf("BUMP must be patch or minor, not %q", bump)
	}
	return fmt.Sprintf("%d.%d.%d", n[0], n[1], n[2]), nil
}

// prepare reads changes/*.md under root in file-name order, writes the new CHANGELOG section, bumps
// VERSION and the version fields, and deletes the fragments. It returns the new version.
func prepare(root, bump string) (string, error) {
	names, _ := filepath.Glob(filepath.Join(root, "changes", "*.md"))
	sort.Strings(names)
	var bullets []string
	var used []string
	for _, n := range names {
		if filepath.Base(n) == "README.md" {
			continue
		}
		b, err := os.ReadFile(n)
		if err != nil {
			return "", err
		}
		if p := whatsnew.LintFragment("changes/"+filepath.Base(n), string(b)); len(p) > 0 {
			return "", fmt.Errorf("a fragment breaks the note rule (see docs/releasing.md):\n  %s", strings.Join(p, "\n  "))
		}
		got := false
		for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n") {
			if strings.HasPrefix(l, "- ") {
				bullets = append(bullets, strings.TrimRight(l, " "))
				got = true
			}
		}
		if !got {
			return "", fmt.Errorf("%s has no '- ' bullet lines", filepath.Base(n))
		}
		used = append(used, n)
	}
	if len(used) == 0 {
		return "", fmt.Errorf("no fragments in changes/: nothing to release (add changes/<ID>.md)")
	}
	vb, err := os.ReadFile(filepath.Join(root, "VERSION"))
	if err != nil {
		return "", err
	}
	next, err := nextVersion(string(vb), bump)
	if err != nil {
		return "", err
	}
	cl := filepath.Join(root, "CHANGELOG.md")
	old, err := os.ReadFile(cl)
	if err != nil {
		return "", err
	}
	const head = "# Changelog\n\n"
	if !strings.HasPrefix(string(old), head) {
		return "", fmt.Errorf("CHANGELOG.md does not start with %q", strings.TrimSpace(head))
	}
	section := "## " + next + "\n\n" + strings.Join(bullets, "\n") + "\n\n"
	// Check every version file before writing any.
	type edit struct {
		path string
		data []byte
	}
	var edits []edit
	for _, f := range versionFiles {
		p := filepath.Join(root, f.path)
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		if !f.re.Match(b) {
			return "", fmt.Errorf("no version field found in %s", f.path)
		}
		done := false
		out := f.re.ReplaceAllFunc(b, func(m []byte) []byte {
			if done {
				return m
			}
			done = true
			return []byte(f.re.ReplaceAllString(string(m), "${1}"+next+"${2}"))
		})
		edits = append(edits, edit{p, out})
	}
	edits = append(edits,
		edit{cl, []byte(head + section + strings.TrimPrefix(string(old), head))},
		edit{filepath.Join(root, "VERSION"), []byte(next + "\n")})
	for _, e := range edits {
		if err := os.WriteFile(e.path, e.data, 0o644); err != nil {
			return "", err
		}
	}
	for _, n := range used {
		if err := os.Remove(n); err != nil {
			return "", err
		}
	}
	return next, nil
}

func git(dir string, args ...string) (string, error) {
	c := exec.Command("git", args...)
	c.Dir = dir
	out, err := c.Output()
	return string(out), err
}

// violation returns the message for a commit that changed a guarded file, or "" when it is fine. files are
// the commit's changed paths.
func violation(files []string) string {
	rel := releaseFiles()
	var ver, log bool
	for _, f := range files {
		ver = ver || f == "VERSION"
		log = log || f == "CHANGELOG.md"
	}
	if !ver && !log {
		return ""
	}
	if !(ver && log) {
		return "changes only one of VERSION and CHANGELOG.md"
	}
	for _, f := range files {
		if !rel[f] && !(strings.HasPrefix(f, "changes/") && strings.HasSuffix(f, ".md")) {
			return "changes VERSION or CHANGELOG.md along with " + f
		}
	}
	return ""
}

// checkBranch fails when a commit between base and HEAD edits VERSION or CHANGELOG.md without being a
// release-prep commit (only VERSION, CHANGELOG.md, the version fields and changes/ fragments). It does
// nothing when base does not exist (a shallow or fresh clone).
func checkBranch(dir, base string) error {
	if _, err := git(dir, "rev-parse", "--verify", "-q", base); err != nil {
		fmt.Println("release-prep: no", base, "to compare with; guard skipped")
		return nil
	}
	mb, err := git(dir, "merge-base", base, "HEAD")
	if err != nil {
		fmt.Println("release-prep: no merge base with", base, "; guard skipped")
		return nil
	}
	log, err := git(dir, "log", "--no-merges", "--format=%x00%h", "--name-only", strings.TrimSpace(mb)+"..HEAD")
	if err != nil {
		return err
	}
	var bad []string
	for _, blk := range strings.Split(log, "\x00") {
		lines := strings.Fields(blk)
		if len(lines) == 0 {
			continue
		}
		if why := violation(lines[1:]); why != "" {
			bad = append(bad, lines[0]+": "+why)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("a normal change must not edit VERSION or CHANGELOG.md: use changes/<ID>.md; `make release-prep` does the release\n  %s", strings.Join(bad, "\n  "))
	}
	return nil
}

// checkFragments fails when a line of any changes/*.md under root breaks the note rule, naming the file.
func checkFragments(root string) error {
	names, _ := filepath.Glob(filepath.Join(root, "changes", "*.md"))
	sort.Strings(names)
	var probs []string
	for _, n := range names {
		if filepath.Base(n) == "README.md" {
			continue
		}
		b, err := os.ReadFile(n)
		if err != nil {
			return err
		}
		probs = append(probs, whatsnew.LintFragment("changes/"+filepath.Base(n), string(b))...)
	}
	if len(probs) > 0 {
		return fmt.Errorf("a fragment breaks the note rule (see docs/releasing.md):\n  %s", strings.Join(probs, "\n  "))
	}
	return nil
}
