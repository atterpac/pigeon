package blob

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

func TestFSPutOpenRoundTrip(t *testing.T) {
	fs := NewFS(t.TempDir())
	ctx := context.Background()
	want := []byte("attachment bytes")

	ref, err := fs.Put(ctx, bytes.NewReader(want))
	if err != nil {
		t.Fatal(err)
	}
	rc, err := fs.Open(ctx, ref)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip mismatch: %q != %q", got, want)
	}
}

func TestFSPutIsContentAddressedAndIdempotent(t *testing.T) {
	fs := NewFS(t.TempDir())
	ctx := context.Background()
	a, err := fs.Put(ctx, bytes.NewReader([]byte("same")))
	if err != nil {
		t.Fatal(err)
	}
	b, err := fs.Put(ctx, bytes.NewReader([]byte("same")))
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("identical content yielded different refs: %q vs %q", a, b)
	}
	list, err := fs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 stored blob after duplicate Put, got %d", len(list))
	}
}

func TestFSDeleteIsIdempotent(t *testing.T) {
	fs := NewFS(t.TempDir())
	ctx := context.Background()
	ref, err := fs.Put(ctx, bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Delete(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := fs.Delete(ctx, ref); err != nil {
		t.Fatalf("second Delete should be a no-op, got %v", err)
	}
	if _, err := fs.Open(ctx, ref); err == nil {
		t.Fatal("expected Open to fail after Delete")
	}
}

func TestFSListReportsModTime(t *testing.T) {
	fs := NewFS(t.TempDir())
	ctx := context.Background()
	if _, err := fs.Put(ctx, bytes.NewReader([]byte("y"))); err != nil {
		t.Fatal(err)
	}
	list, err := fs.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 blob, got %d", len(list))
	}
	if time.Since(list[0].ModTime) > time.Hour {
		t.Fatalf("unexpected mod time: %v", list[0].ModTime)
	}
	if list[0].Size != 1 {
		t.Fatalf("expected size 1, got %d", list[0].Size)
	}
}

func TestFSRejectsBadRef(t *testing.T) {
	fs := NewFS(t.TempDir())
	ctx := context.Background()
	for _, ref := range []string{"", "nope", "sha256:short", "md5:" + string(make([]byte, 64))} {
		if _, err := fs.Open(ctx, ref); err == nil {
			t.Fatalf("expected error opening bad ref %q", ref)
		}
	}
}

// List on a never-written store returns nothing rather than erroring.
func TestFSListEmpty(t *testing.T) {
	fs := NewFS(t.TempDir() + "/never-created")
	list, err := fs.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d", len(list))
	}
}
