// Package auth manages the local Kinlyze Dashboard credentials file.
package auth

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// ErrNoCredentials is returned when no token is saved locally.
var ErrNoCredentials = errors.New("not logged in")

func credentialsPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kinlyze", "credentials"), nil
}

// SaveToken writes the token to ~/.kinlyze/credentials with mode 0600.
func SaveToken(token string) error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(token+"\n"), 0600)
}

// LoadToken reads the saved token. It returns ErrNoCredentials if the user
// has never logged in (or has logged out), which callers should treat as
// "run fully local" rather than an error.
func LoadToken() (string, error) {
	path, err := credentialsPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNoCredentials
		}
		return "", err
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", ErrNoCredentials
	}
	return token, nil
}

// DeleteToken removes the credentials file, if present.
func DeleteToken() error {
	path, err := credentialsPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
