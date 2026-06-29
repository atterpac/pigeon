package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEncryptedRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "creds.enc")
	pass := func() ([]byte, error) { return []byte("correct horse battery staple"), nil }

	s := NewEncrypted(path, pass)
	want := Credential{Password: "app-pass-123"}
	if err := s.Set(ctx, "a@x.io", want); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Fresh instance to prove it decrypts from disk, not memory.
	got, err := NewEncrypted(path, pass).Get(ctx, "a@x.io")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Password != want.Password {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	// Wrong passphrase must fail to decrypt.
	bad := func() ([]byte, error) { return []byte("wrong"), nil }
	if _, err := NewEncrypted(path, bad).Get(ctx, "a@x.io"); err == nil {
		t.Fatal("expected decrypt failure with wrong passphrase")
	}

	if err := s.Delete(ctx, "a@x.io"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, "a@x.io"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// A non-envelope file must surface a parse error rather than silently returning
// no credentials.
func TestEncryptedCorruptFile(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "creds.enc")
	if err := os.WriteFile(path, []byte("not an envelope"), 0o600); err != nil {
		t.Fatal(err)
	}
	pass := func() ([]byte, error) { return []byte("pw"), nil }
	if _, err := NewEncrypted(path, pass).Get(ctx, "a@x.io"); err == nil {
		t.Fatal("expected error reading corrupt credential file")
	}
}

// The passphrase is derived only once: a single instance answering many Gets
// must not re-invoke pass() (and thus the argon2 KDF) after the first load.
func TestEncryptedDerivesOnce(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "creds.enc")
	calls := 0
	pass := func() ([]byte, error) { calls++; return []byte("pw"), nil }

	s := NewEncrypted(path, pass)
	if err := s.Set(ctx, "a@x.io", Credential{Password: "p"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	callsAfterSet := calls
	for range 5 {
		if _, err := s.Get(ctx, "a@x.io"); err != nil {
			t.Fatalf("get: %v", err)
		}
	}
	if calls != callsAfterSet {
		t.Fatalf("pass() called %d times during cached Gets, want 0", calls-callsAfterSet)
	}
}
