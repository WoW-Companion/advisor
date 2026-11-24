package provider

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// KeyStore keeps the player's API keys, one per provider. Keys never go in the config file or a log.
type KeyStore interface {
	// Get returns the saved key, or ErrNoKey.
	Get(provider string) (string, error)
	Set(provider, key string) error
	// Delete removes the key; deleting a key that is not there is not an error.
	Delete(provider string) error
}

// ErrNoKey says no key is saved for the provider.
var ErrNoKey = errors.New("no key saved")

var providerID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

func checkID(provider string) error {
	if !providerID.MatchString(provider) {
		return fmt.Errorf("%q is not a provider id", provider)
	}
	return nil
}

// CleanKey trims a pasted key; it must be one line of printable characters.
func CleanKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 4096 || strings.ContainsAny(key, "\r\n\x00") {
		return "", errors.New("that doesn't look like an API key")
	}
	return key, nil
}

// FileKeyStore keeps each key in its own file in Dir, readable only by the user (mode 0600, in a
// 0700 folder). It is the store wherever the platform has no small pure-Go keychain binding.
type FileKeyStore struct{ Dir string }

func (f FileKeyStore) path(provider string) (string, error) {
	if err := checkID(provider); err != nil {
		return "", err
	}
	return filepath.Join(f.Dir, provider+".key"), nil
}

// Get reads the key.
func (f FileKeyStore) Get(provider string) (string, error) {
	p, err := f.path(provider)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNoKey
	}
	if err != nil {
		return "", errors.New("couldn't read the saved key")
	}
	return strings.TrimSpace(string(b)), nil
}

// Set writes the key, replacing any earlier one.
func (f FileKeyStore) Set(provider, key string) error {
	p, err := f.path(provider)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return errors.New("couldn't create the folder for keys")
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, []byte(key), 0o600); err != nil {
		return errors.New("couldn't save the key")
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return errors.New("couldn't save the key")
	}
	return nil
}

// Delete removes the key.
func (f FileKeyStore) Delete(provider string) error {
	p, err := f.path(provider)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.New("couldn't delete the key")
	}
	return nil
}

// DataDir is the advisor's data folder: <user config dir>/wowc-advisor.
func DataDir() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "wowc-advisor")
}
