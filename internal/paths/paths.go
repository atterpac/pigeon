// Package paths resolves Pigeon's on-disk locations following the XDG Base
// Directory spec. All persistent state (the SQLite store and its blob sidecar)
// lives under a single "pigeon" data directory so the desktop app and the CLI
// share one source of truth.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
)

// appName is the per-user subdirectory under the XDG data root.
const appName = "pigeon"

// DataDir returns Pigeon's data directory — $XDG_DATA_HOME/pigeon, or
// ~/.local/share/pigeon when XDG_DATA_HOME is unset — creating it 0700 if
// missing.
func DataDir() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home dir (set XDG_DATA_HOME): %w", err)
		}
		dir = filepath.Join(home, ".local", "share")
	}
	dir = filepath.Join(dir, appName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create data dir: %w", err)
	}
	return dir, nil
}

// DBPath returns the SQLite database path. EMAIL_DB overrides the location
// outright (its parent is created if missing); otherwise the db lives at
// <DataDir>/mail.db. The blob sidecar is a "blobs" directory alongside it.
func DBPath() (string, error) {
	if p := os.Getenv("EMAIL_DB"); p != "" {
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			return "", fmt.Errorf("create db dir: %w", err)
		}
		return p, nil
	}
	dir, err := DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mail.db"), nil
}
