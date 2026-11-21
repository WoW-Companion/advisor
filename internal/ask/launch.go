package ask

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// Registry reads the PATH the machine's settings hold, as opposed to the one this process was
// started with. A fake stands in for it in tests so they run on any OS.
type Registry interface {
	// Paths are the user's and the system's PATH values, %VAR% references already expanded.
	Paths(getenv func(string) string) []string
}

type sysRegistry struct{}

func (sysRegistry) Paths(getenv func(string) string) []string { return registryPaths(getenv) }

// registry is read on every detection and every launch, never cached: a client installed while
// the overlay runs shows up in the registry but not in this process's environment.
var registry Registry = sysRegistry{}

// dedupPaths joins PATH lists, dropping empty and repeated entries (case-insensitively on Windows).
func dedupPaths(lists ...string) string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lists {
		for _, d := range filepath.SplitList(l) {
			k := d
			if winMode {
				k = strings.ToLower(d)
			}
			if d != "" && !seen[k] {
				seen[k] = true
				out = append(out, d)
			}
		}
	}
	return strings.Join(out, string(os.PathListSeparator))
}

// freshPath is the PATH for looking up and running an agent: on Windows the registry's, merged with
// this process's own so tools the agent spawns are found too.
func freshPath() string {
	if !winMode {
		return os.Getenv("PATH")
	}
	return dedupPaths(append(registry.Paths(os.Getenv), os.Getenv("PATH"))...)
}

// LookPath resolves an agent command to an absolute file, by the rules detection uses: PATH from
// freshPath, PATHEXT, then the known install folders. A name with a directory is used as given
// (with an extension added on Windows). A .ps1 script is swapped for a sibling .cmd or .exe.
func LookPath(name string) (string, error) {
	exts := pathExts(os.Getenv("PATHEXT"))
	var found string
	if strings.ContainsAny(name, `/\`) {
		for _, c := range exeNames(name, exts) {
			if isExecutable(c) {
				found = c
				break
			}
		}
		if !winMode && found == "" && isExecutable(name) {
			found = name
		}
	} else {
		found = findExe(name, append(filepath.SplitList(freshPath()), extraDirs(os.Getenv)...), exts)
	}
	if found == "" {
		return "", fmt.Errorf("Couldn't find %s: it is not on PATH or in the usual install folders", displayName(name))
	}
	return preferSibling(found), nil
}

// findExe is the first file called name (with a PATHEXT extension on Windows) in dirs.
func findExe(name string, dirs, exts []string) string {
	for _, d := range dirs {
		if d == "" {
			continue
		}
		for _, c := range exeNames(name, exts) {
			if p := filepath.Join(d, c); isExecutable(p) {
				return p
			}
		}
	}
	return ""
}

// preferSibling swaps a .ps1 for a .exe or .cmd next to it: those start without PowerShell's
// execution policy in the way.
func preferSibling(p string) string {
	if !strings.EqualFold(filepath.Ext(p), ".ps1") {
		return p
	}
	base := strings.TrimSuffix(p, filepath.Ext(p))
	for _, e := range []string{".exe", ".cmd"} {
		if isExecutable(base + e) {
			return base + e
		}
	}
	return p
}

// withEnv returns the process environment with PATH replaced by freshPath and extra added.
func withEnv(extra ...string) []string {
	env := make([]string, 0, len(os.Environ())+len(extra)+1)
	for _, kv := range os.Environ() {
		if k, _, _ := strings.Cut(kv, "="); strings.EqualFold(k, "PATH") && winMode || k == "PATH" {
			continue
		}
		env = append(env, kv)
	}
	return append(append(env, "PATH="+freshPath()), extra...)
}

// Launch builds the command for an agent argv: argv[0] is resolved to an absolute path, scripts
// are wrapped (see Command) and the environment carries a fresh PATH. The error is fit to show.
func Launch(ctx context.Context, argv []string, extraEnv ...string) (*exec.Cmd, error) {
	if len(argv) == 0 {
		return nil, errors.New("no agent command is configured")
	}
	p, err := LookPath(argv[0])
	if err != nil {
		return nil, err
	}
	cmd := Command(ctx, append([]string{p}, argv[1:]...))
	cmd.Env = withEnv(extraEnv...)
	return cmd, nil
}

// displayName is an agent's name as a person writes it: "codex.cmd" is "Codex".
func displayName(argv0 string) string {
	b := filepath.Base(strings.ReplaceAll(argv0, `\`, "/"))
	b = strings.TrimSuffix(b, filepath.Ext(b))
	if b == "" {
		return "the agent"
	}
	r := []rune(b)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// startError is the one line shown when the agent process could not be started.
func startError(path string, err error) error {
	reason := err.Error()
	var pe *fs.PathError
	if errors.As(err, &pe) {
		reason = pe.Err.Error()
	}
	return fmt.Errorf("Couldn't start %s (%s): %s", displayName(path), path, reason)
}

// exitError is the line shown when the agent started and exited non-zero: its first stderr line
// when it wrote no answer, else the status.
func exitError(ee *exec.ExitError, wroteOutput bool, stderr string) error {
	if l := firstLine(stderr); l != "" && !wroteOutput {
		return errors.New(l)
	}
	return fmt.Errorf("the agent exited with status %d", ee.ExitCode())
}

// counter counts the bytes written through it.
type counter struct {
	w io.Writer
	n int
}

func (c *counter) Write(p []byte) (int, error) {
	c.n += len(p)
	return c.w.Write(p)
}
