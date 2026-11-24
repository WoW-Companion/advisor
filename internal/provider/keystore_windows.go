package provider

import (
	"errors"

	"github.com/danieljoos/wincred"
)

// credStore keeps keys in Windows Credential Manager, one generic credential per provider named
// wowc-advisor:<provider>. wincred is pure Go (no cgo; it calls advapi32 through x/sys), so the
// Windows release stays a cgo-free build.
type credStore struct{}

// DefaultKeyStore is Windows Credential Manager.
func DefaultKeyStore() KeyStore { return credStore{} }

func (credStore) Get(provider string) (string, error) {
	if err := checkID(provider); err != nil {
		return "", err
	}
	c, err := wincred.GetGenericCredential(KeyTarget(provider))
	if errors.Is(err, wincred.ErrElementNotFound) {
		return "", ErrNoKey
	}
	if err != nil {
		return "", errors.New("couldn't read the saved key from Credential Manager")
	}
	return string(c.CredentialBlob), nil
}

func (credStore) Set(provider, key string) error {
	if err := checkID(provider); err != nil {
		return err
	}
	c := wincred.NewGenericCredential(KeyTarget(provider))
	c.CredentialBlob = []byte(key)
	c.Persist = wincred.PersistLocalMachine
	if err := c.Write(); err != nil {
		return errors.New("couldn't save the key in Credential Manager")
	}
	return nil
}

func (credStore) Delete(provider string) error {
	if err := checkID(provider); err != nil {
		return err
	}
	c, err := wincred.GetGenericCredential(KeyTarget(provider))
	if errors.Is(err, wincred.ErrElementNotFound) {
		return nil
	}
	if err != nil {
		return errors.New("couldn't read Credential Manager")
	}
	if err := c.Delete(); err != nil {
		return errors.New("couldn't delete the key from Credential Manager")
	}
	return nil
}
