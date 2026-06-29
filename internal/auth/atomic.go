package auth

import (
	"os"
	"path/filepath"
)

// writeFileAtomic writes data to path by first writing a uniquely-named temp
// file in the same directory and renaming it into place, so a reader never
// observes a partial file. The parent directory is created 0700 and the temp
// file is created 0600 by os.CreateTemp. A failed write leaves no temp file
// behind. The unique name (vs a fixed ".tmp") keeps concurrent writers from
// corrupting each other's temp file.
func writeFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".cred-*.tmp")
	if err != nil {
		return err
	}
	// Drop the temp file unless the rename below promotes it.
	defer func() {
		if err != nil {
			_ = os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	err = os.Rename(tmp.Name(), path)
	return err
}
