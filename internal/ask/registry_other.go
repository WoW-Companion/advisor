//go:build !windows

package ask

// registryPaths is Windows only.
func registryPaths(func(string) string) []string { return nil }
