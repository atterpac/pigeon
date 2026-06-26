package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// File is a CredentialStore backed by a single JSON file with 0600 perms. Use
// it on hosts without an OS Secret Service (the keyring's "name is not
// activatable" error). Tokens are stored in plaintext, so the file must live on
// an adequately protected filesystem; treat it like an SSH private key.
type File struct {
	path string
	mu   sync.Mutex
}

// NewFile returns a file-backed store at path. The parent directory is created
// with 0700 on first write.
func NewFile(path string) *File { return &File{path: path} }

// DefaultFilePath is $XDG_CONFIG_HOME/email/credentials.json (or ~/.config/...).
func DefaultFilePath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "email", "credentials.json")
}

func (f *File) load() (map[string]Credential, error) {
	b, err := os.ReadFile(f.path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]Credential{}, nil
	}
	if err != nil {
		return nil, err
	}
	m := map[string]Credential{}
	if len(b) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse credential file: %w", err)
	}
	return m, nil
}

func (f *File) save(m map[string]Credential) error {
	if err := os.MkdirAll(filepath.Dir(f.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	// Write to a temp file then rename for atomicity; 0600 perms.
	tmp := f.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, f.path)
}

func (f *File) Get(_ context.Context, account string) (Credential, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return Credential{}, err
	}
	c, ok := m[account]
	if !ok {
		return Credential{}, ErrNotFound
	}
	return c, nil
}

func (f *File) Set(_ context.Context, account string, c Credential) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	m[account] = c
	return f.save(m)
}

func (f *File) Delete(_ context.Context, account string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m, err := f.load()
	if err != nil {
		return err
	}
	if _, ok := m[account]; !ok {
		return ErrNotFound
	}
	delete(m, account)
	return f.save(m)
}
