// Package config resolves the default on-disk locations for the application.
package config

import (
	"os"
	"path/filepath"
)

// AppName is the folder created under the OS config directory.
const AppName = "electric-hamster"

// Dir returns the per-user configuration directory, creating it if needed.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, AppName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// DefaultAppDB returns the path of the application database.
func DefaultAppDB() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "app.db"), nil
}

// DefaultVault returns the path of the encrypted vault.
func DefaultVault() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vault.db"), nil
}
