package ask

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Command is exec.CommandContext for the agent argv. On Windows a .cmd or .bat shim (npm's) runs
// through cmd.exe and a .ps1 script through PowerShell; anything else runs directly.
func Command(ctx context.Context, argv []string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		if exe, line, ok := scriptLine(argv); ok {
			return winCommand(ctx, exe, line)
		}
	}
	return exec.CommandContext(ctx, argv[0], argv[1:]...)
}

// scriptLine is the Windows command line that runs argv when argv[0] is a script: the program to
// start and its complete, already-quoted command line.
func scriptLine(argv []string) (exe, line string, ok bool) {
	switch strings.ToLower(filepath.Ext(argv[0])) {
	case ".cmd", ".bat":
		// cmd /s /c "<line>" strips the outer quotes and runs the rest as typed, so every part
		// is quoted inside them.
		parts := make([]string, len(argv))
		for i, a := range argv {
			parts[i] = cmdQuote(a)
		}
		return "cmd.exe", `cmd.exe /d /s /c "` + strings.Join(parts, " ") + `"`, true
	case ".ps1":
		parts := []string{"powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File"}
		for _, a := range argv {
			parts = append(parts, winQuote(a))
		}
		return "powershell.exe", strings.Join(parts, " "), true
	}
	return "", "", false
}

// cmdQuote quotes one argument for a cmd.exe command line. Inside double quotes cmd treats & | < >
// ^ literally; what is left is the quote itself, %VAR% expansion and a line break (which ends the
// command), so those three are changed: " to ', % to the full-width ％ and a line break to a space.
func cmdQuote(a string) string {
	a = strings.NewReplacer("\"", "'", "%", "％", "\r\n", " ", "\n", " ", "\r", " ", "\x00", "").Replace(a)
	if strings.HasSuffix(a, `\`) {
		a += `\` // a trailing backslash must not escape the closing quote
	}
	return `"` + a + `"`
}

// winQuote quotes one argument by the rules CommandLineToArgv reads (what PowerShell -File uses).
func winQuote(a string) string {
	var b strings.Builder
	b.WriteByte('"')
	slashes := 0
	for _, c := range a {
		switch c {
		case '\\':
			slashes++
			b.WriteRune(c)
			continue
		case '"':
			b.WriteString(strings.Repeat(`\`, slashes+1))
		}
		slashes = 0
		b.WriteRune(c)
	}
	b.WriteString(strings.Repeat(`\`, slashes))
	b.WriteByte('"')
	return b.String()
}
