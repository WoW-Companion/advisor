package ask

import (
	"os"
	"path/filepath"
	"strings"
)

// Where an agent came from, as the companion's handshake reports it.
const (
	SourceFlag   = "flag"   // --agent
	SourceConfig = "config" // "agent" in the config file
	SourceAuto   = "auto"   // found on this machine
	SourceNone   = "none"   // no agent configured and none found
)

// AutoAgent is the --agent (or config) value that asks for detection even when the config file
// names an agent.
const AutoAgent = "auto"

// DetectOrder is the order the AI CLIs are looked for in; the first one found is used.
var DetectOrder = []string{"claude", "codex", "gemini"}

// Agent is a resolved agent: its argv, its short name and where it came from. A Source of
// SourceNone has no Command.
type Agent struct {
	Command []string
	Name    string
	Source  string
}

// ResolveAuto is ResolveAgent for callers that can run without an agent: with nothing configured,
// or the value "auto", it looks for the first of DetectOrder installed. Nothing found is no error.
func ResolveAuto(flagValue, configPath string) (Agent, error) {
	spec, source, err := agentSpec(flagValue, configPath)
	if err != nil {
		return Agent{}, err
	}
	if spec != "" && spec != AutoAgent {
		argv := expand(spec)
		return Agent{Command: argv, Name: agentName(spec, argv), Source: source}, nil
	}
	name, path, ok := Detect(os.Getenv("PATH"), extraDirs(os.Getenv))
	if !ok {
		return Agent{Source: SourceNone}, nil
	}
	argv := expand(name)
	argv[0] = path
	return Agent{Command: argv, Name: name, Source: SourceAuto}, nil
}

func agentName(spec string, argv []string) string {
	if _, ok := Presets[spec]; ok || len(argv) == 0 {
		return spec
	}
	b := filepath.Base(argv[0])
	return strings.TrimSuffix(b, filepath.Ext(b))
}

// extraDirs is ExtraDirs; a test swaps it so the machine's own installs do not leak in.
var extraDirs = ExtraDirs

// Detect finds the first of DetectOrder in the PATH list, else in the extra directories, and the
// full path to it.
func Detect(pathEnv string, extra []string) (name, path string, ok bool) {
	dirs := append(filepath.SplitList(pathEnv), extra...)
	for _, n := range DetectOrder {
		for _, d := range dirs {
			if d == "" {
				continue
			}
			for _, c := range exeNames(n) {
				if p := filepath.Join(d, c); isExecutable(p) {
					return n, p, true
				}
			}
		}
	}
	return "", "", false
}

// exeNames are the file names an executable may have: on Windows the usual extensions (npm installs
// a .cmd shim), elsewhere the bare name.
func exeNames(name string) []string {
	if os.PathSeparator == '\\' {
		return []string{name + ".exe", name + ".cmd", name + ".bat", name + ".com"}
	}
	return []string{name}
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	return os.PathSeparator == '\\' || st.Mode()&0o111 != 0
}

// ExtraDirs are the places the AI CLIs' installers put them that a desktop app's PATH may lack.
// getenv supplies USERPROFILE, APPDATA, LOCALAPPDATA and ProgramFiles (Windows) or HOME.
func ExtraDirs(getenv func(string) string) []string {
	var dirs []string
	add := func(base string, rest ...string) {
		if base != "" {
			dirs = append(dirs, filepath.Join(append([]string{base}, rest...)...))
		}
	}
	if os.PathSeparator == '\\' {
		home := getenv("USERPROFILE")
		add(home, ".local", "bin")    // Claude Code's native installer
		add(home, ".claude", "local") // Claude Code's local install
		add(getenv("APPDATA"), "npm") // npm install -g (claude, codex, gemini)
		add(getenv("LOCALAPPDATA"), "Programs", "claude")
		add(home, "scoop", "shims") // scoop
		add(getenv("ProgramFiles"), "nodejs")
		return dirs
	}
	home := getenv("HOME")
	add(home, ".local", "bin")
	add(home, ".claude", "local")
	add(home, ".npm-global", "bin")
	dirs = append(dirs, "/opt/homebrew/bin", "/usr/local/bin")
	return dirs
}
