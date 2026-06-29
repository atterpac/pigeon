package auth

import (
	"context"
	"path/filepath"
	"testing"
)

func TestFileRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "creds.json")

	s := NewFile(path)
	want := Credential{Password: "app-pass-123"}
	if err := s.Set(ctx, "a@x.io", want); err != nil {
		t.Fatalf("set: %v", err)
	}

	// Fresh instance to prove it reads from disk, not memory.
	got, err := NewFile(path).Get(ctx, "a@x.io")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Password != want.Password {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	if _, err := s.Get(ctx, "missing"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for missing account, got %v", err)
	}

	if err := s.Delete(ctx, "a@x.io"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := s.Get(ctx, "a@x.io"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
	if err := s.Delete(ctx, "a@x.io"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound deleting missing, got %v", err)
	}
}
