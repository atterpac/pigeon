// Package blob stores large MIME part bodies outside the SQLite row,
// content-addressed by SHA-256. The store keeps the database lean and lets
// callers load attachment bytes lazily, only when actually opened.
package blob

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const refPrefix = "sha256:"

// Info describes one stored blob, used by sweeps to reclaim orphans.
type Info struct {
	Ref     string
	ModTime time.Time
	Size    int64
}

// Store persists opaque blobs addressed by a content ref ("sha256:<hex>").
// Implementations must be safe for concurrent use.
type Store interface {
	// Put stores r's bytes and returns their content ref. Idempotent: identical
	// content yields the same ref and leaves any existing blob untouched.
	Put(ctx context.Context, r io.Reader) (ref string, err error)
	// Open returns a reader over the blob's bytes; the caller closes it.
	Open(ctx context.Context, ref string) (io.ReadCloser, error)
	// Delete removes a blob. Removing a missing blob is not an error.
	Delete(ctx context.Context, ref string) error
	// List enumerates every stored blob.
	List(ctx context.Context) ([]Info, error)
}

// FS is a filesystem-backed Store laid out as <root>/<ab>/<sha256hex>, sharded
// by the first hash byte so no directory grows unbounded.
type FS struct{ root string }

// NewFS returns a Store rooted at dir. The directory is created lazily on first
// Put.
func NewFS(dir string) *FS { return &FS{root: dir} }

func (f *FS) path(ref string) (string, error) {
	h, ok := strings.CutPrefix(ref, refPrefix)
	if !ok || len(h) != 64 {
		return "", fmt.Errorf("blob: invalid ref %q", ref)
	}
	return filepath.Join(f.root, h[:2], h), nil
}

// Put streams r to a temp file while hashing, then atomically renames it into
// place under its content ref. The bytes never need to fit in memory.
func (f *FS) Put(ctx context.Context, r io.Reader) (string, error) {
	if err := os.MkdirAll(f.root, 0o700); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(f.root, ".tmp-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op once renamed away

	h := sha256.New()
	if _, err := io.Copy(tmp, io.TeeReader(r, h)); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}

	ref := refPrefix + hex.EncodeToString(h.Sum(nil))
	dst, err := f.path(ref)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return "", err
	}
	if _, err := os.Stat(dst); err == nil {
		return ref, nil // already stored — content-addressed dedup
	}
	if err := os.Rename(tmpName, dst); err != nil {
		return "", err
	}
	return ref, nil
}

func (f *FS) Open(ctx context.Context, ref string) (io.ReadCloser, error) {
	p, err := f.path(ref)
	if err != nil {
		return nil, err
	}
	return os.Open(p)
}

func (f *FS) Delete(ctx context.Context, ref string) error {
	p, err := f.path(ref)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// List walks the shard directories and returns one Info per stored blob.
// In-flight temp files (".tmp-*") and stray non-hash entries are skipped.
func (f *FS) List(ctx context.Context) ([]Info, error) {
	shards, err := os.ReadDir(f.root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Info
	for _, shard := range shards {
		if !shard.IsDir() || len(shard.Name()) != 2 {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(f.root, shard.Name()))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			name := e.Name()
			if len(name) != 64 || !strings.HasPrefix(name, shard.Name()) {
				continue
			}
			fi, err := e.Info()
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue // raced with a Delete
				}
				return nil, err
			}
			out = append(out, Info{Ref: refPrefix + name, ModTime: fi.ModTime(), Size: fi.Size()})
		}
	}
	return out, nil
}
