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

// DetectOrder is the order the AI CLIs are looked for in; the first one found is used. Gemini CLI is
// legacy (API keys only since 2026-06-18), so it is chosen only when nothing else is installed.
var DetectOrder = []string{"claude", "codex", "agy", "gemini"}

// Agent is a resolved agent: its argv, its short name and where it came from. A Source of
// SourceNone has no Command.
type Agent struct {
	Command []string
	Name    string
	Source  string
}

// DetectedPath is the executable a detected agent runs ("" for any other source).
func DetectedPath(source string, command []string) string {
	if source == SourceAuto && len(command) > 0 {
		return command[0]
	}
	return ""
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
	name, path, ok := Detect(searchPath(), extraDirs(os.Getenv))
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

// winMode is true where executables are found by extension (Windows); a test sets it to check
// that lookup on any machine.
var winMode = os.PathSeparator == '\\'

// searchPath is the PATH to look in: the process's own and, on Windows, the user's and the
// system's from the registry. A desktop app started from the Start menu may have a minimal PATH,
// and a program installed after this one started is only in the registry.
var searchPath = func() string {
	return freshPath()
}

// Detect finds the first of DetectOrder in the PATH list, else in the extra directories, and the
// full path to it.
func Detect(pathEnv string, extra []string) (name, path string, ok bool) {
	dirs := append(filepath.SplitList(pathEnv), extra...)
	exts := pathExts(os.Getenv("PATHEXT"))
	for _, n := range DetectOrder {
		for _, d := range dirs {
			if d == "" {
				continue
			}
			for _, c := range exeNames(n, exts) {
				if p := filepath.Join(d, c); isExecutable(p) {
					return n, p, true
				}
			}
		}
	}
	return "", "", false
}

// scriptExts are run through cmd.exe or PowerShell, see Command. Always looked for on Windows,
// whatever PATHEXT says: PowerShell's .ps1 is not in the default PATHEXT.
var scriptExts = []string{".cmd", ".bat", ".ps1"}

// pathExts is PATHEXT as lower-case extensions: .exe and .com first (a real program beats a shim),
// then the other PATHEXT entries, then the script kinds.
func pathExts(pathext string) []string {
	if pathext == "" {
		pathext = ".COM;.EXE;.BAT;.CMD"
	}
	seen := map[string]bool{}
	var out []string
	add := func(e string) {
		e = strings.ToLower(strings.TrimSpace(e))
		if e != "" && !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	add(".exe")
	add(".com")
	for _, e := range strings.Split(pathext, ";") {
		add(e)
	}
	for _, e := range scriptExts {
		add(e)
	}
	return out
}

// exeNames are the file names an executable may have: on Windows the PATHEXT extensions (npm
// installs a .cmd shim), elsewhere the bare name.
func exeNames(name string, exts []string) []string {
	if !winMode {
		return []string{name}
	}
	out := make([]string, len(exts))
	for i, e := range exts {
		out[i] = name + e
	}
	return out
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	return winMode || st.Mode()&0o111 != 0
}

// ExtraDirs are the places the AI CLIs' installers put them that a desktop app's PATH may lack.
// getenv supplies USERPROFILE, APPDATA, LOCALAPPDATA and ProgramFiles (Windows) or HOME.
//
// On Windows:
//
//	%USERPROFILE%\.local\bin                       Claude Code's native installer (claude.exe)
//	%USERPROFILE%\.claude\local                    Claude Code's local install
//	%LOCALAPPDATA%\agy\bin                        Antigravity CLI's installer (agy.exe; install.ps1, https://antigravity.google/docs/getting-started)
//	%APPDATA%\npm                                  npm install -g shims (claude.cmd, codex.cmd, gemini.cmd)
//	%LOCALAPPDATA%\Microsoft\WinGet\Links          WinGet's command links
//	%LOCALAPPDATA%\Programs\claude
//	%USERPROFILE%\scoop\shims                      Scoop shims
//	%ProgramFiles%\nodejs                          a system-wide npm prefix
func ExtraDirs(getenv func(string) string) []string {
	var dirs []string
	add := func(base string, rest ...string) {
		if base != "" {
			dirs = append(dirs, filepath.Join(append([]string{base}, rest...)...))
		}
	}
	if winMode {
		home, local := getenv("USERPROFILE"), getenv("LOCALAPPDATA")
		add(home, ".local", "bin")
		add(home, ".claude", "local")
		add(getenv("APPDATA"), "npm")
		add(local, "Microsoft", "WinGet", "Links")
		add(local, "Programs", "claude")
		add(local, "agy", "bin")
		add(home, "scoop", "shims")
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

// expandPercent expands %NAME% references the way the registry's REG_EXPAND_SZ values use them;
// an unknown name is left as written.
func expandPercent(s string, getenv func(string) string) string {
	var b strings.Builder
	for {
		i := strings.IndexByte(s, '%')
		if i < 0 {
			break
		}
		j := strings.IndexByte(s[i+1:], '%')
		if j < 0 {
			break
		}
		name := s[i+1 : i+1+j]
		b.WriteString(s[:i])
		if v := getenv(name); v != "" && name != "" {
			b.WriteString(v)
		} else {
			b.WriteString("%" + name + "%")
		}
		s = s[i+j+2:]
	}
	b.WriteString(s)
	return b.String()
}

// parseRegPath pulls the Path value out of `reg query <key> /v Path` output, whose line reads
// "    Path    REG_EXPAND_SZ    C:\a;%USERPROFILE%\b".
func parseRegPath(out string, getenv func(string) string) string {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && strings.EqualFold(f[0], "Path") && strings.HasPrefix(f[1], "REG_") {
			i := strings.Index(line, f[1]) + len(f[1])
			return expandPercent(strings.TrimSpace(line[i:]), getenv)
		}
	}
	return ""
}
