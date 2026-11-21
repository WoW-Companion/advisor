package ask

import (
	"context"
	"os/exec"
	"time"
)

// registryPaths are the user's and the system's PATH as the registry holds them
// (HKCU\Environment and HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment).
func registryPaths(getenv func(string) string) []string {
	var out []string
	for _, key := range []string{
		`HKCU\Environment`,
		`HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Environment`,
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		b, err := exec.CommandContext(ctx, "reg.exe", "query", key, "/v", "Path").Output()
		cancel()
		if err == nil {
			out = append(out, parseRegPath(string(b), getenv))
		}
	}
	return out
}
