//go:build !windows

package provider

import "path/filepath"

// DefaultKeyStore is a private file in the data folder: macOS and Linux have no small pure-Go
// keychain binding (the macOS one shells out to `security`, which would put the key in a process
// argument list), so the key sits in <data folder>/keys/<provider>.key, mode 0600 in a 0700 folder.
func DefaultKeyStore() KeyStore { return FileKeyStore{Dir: filepath.Join(DataDir(), "keys")} }
