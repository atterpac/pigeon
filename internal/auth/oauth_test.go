package auth

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoadGoogleConfig validates a real client_secret.json if present. It is
// skipped when the file is absent so CI stays hermetic.
func TestLoadGoogleConfig(t *testing.T) {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, "creds", "google", "birdie", "client_secret.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("no client secret at %s", path)
	}
	cfg, err := LoadGoogleConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		t.Fatal("missing client id/secret")
	}
	if len(cfg.Scopes) != 1 || cfg.Scopes[0] != GmailScope {
		t.Fatalf("unexpected scopes: %v", cfg.Scopes)
	}
	if cfg.Endpoint.AuthURL == "" {
		t.Fatal("missing auth endpoint")
	}
}
