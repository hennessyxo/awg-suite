package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/zalando/go-keyring"
)

// keyringService is the namespace under which the SSH password is stored in the
// OS secret store (macOS Keychain / Windows Credential Manager / libsecret).
const keyringService = "awg-gui-ssh"

// diskPrefs holds ONLY non-secret connection fields persisted to a config file.
// The password is deliberately absent here so it can never be written to disk.
type diskPrefs struct {
	Host         string `json:"host"`
	User         string `json:"user"`
	AuthMode     string `json:"authMode"` // "password" | "key"
	IdentityPath string `json:"identityPath"`
	Remember     bool   `json:"remember"`
}

// Prefs is what the frontend receives: the saved non-secret fields plus the
// password pulled from the OS secret store (only when Remember is set).
type Prefs struct {
	Host         string `json:"host"`
	User         string `json:"user"`
	AuthMode     string `json:"authMode"`
	IdentityPath string `json:"identityPath"`
	Remember     bool   `json:"remember"`
	Password     string `json:"password"`
}

// prefsPath returns the config file path (e.g. ~/Library/Application Support/
// awg-gui/config.json), creating the directory if needed.
func prefsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir = filepath.Join(dir, "awg-gui")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

// keyringAccount keys the stored secret per user@host.
func keyringAccount(user, host string) string { return user + "@" + host }

// loadDiskPrefs reads the non-secret config; a missing file yields zero prefs.
func loadDiskPrefs() (diskPrefs, error) {
	var p diskPrefs
	path, err := prefsPath()
	if err != nil {
		return p, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	_ = json.Unmarshal(b, &p)
	return p, nil
}

// saveDiskPrefs writes the non-secret config (0600, never contains a password).
func saveDiskPrefs(p diskPrefs) error {
	path, err := prefsPath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

// rememberPassword stores the password in the OS secret store.
func rememberPassword(user, host, password string) error {
	return keyring.Set(keyringService, keyringAccount(user, host), password)
}

// loadPassword fetches a stored password; a missing entry returns "".
func loadPassword(user, host string) string {
	pw, err := keyring.Get(keyringService, keyringAccount(user, host))
	if err != nil {
		return ""
	}
	return pw
}

// forgetPassword removes any stored password (ignoring "not found").
func forgetPassword(user, host string) {
	_ = keyring.Delete(keyringService, keyringAccount(user, host))
}
