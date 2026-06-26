package auth

import (
	"context"
	"path/filepath"
	"testing"
)

func TestEncryptedRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "creds.enc")
	pass := func() ([]byte, error) { return []byte("correct horse battery staple"), nil }

	s := NewEncrypted(path, pass)
	want := Credential{RefreshToken: "rt-123", AccessToken: "at-456", TokenType: "Bearer"}
	if err := s.Set(ctx, "a@x.io", want); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Fresh instance to prove it decrypts from disk, not memory.
	got, err := NewEncrypted(path, pass).Get(ctx, "a@x.io")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RefreshToken != want.RefreshToken || got.AccessToken != want.AccessToken {
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
