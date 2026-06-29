package migrations

import (
	"io/fs"
	"testing"
)

// The //go:embed directive fails to compile if the directory is missing, but an
// empty glob match would still build and silently run zero migrations. Pin the
// invariant that at least one migration is embedded.
func TestEmbeddedMigrations(t *testing.T) {
	entries, err := fs.Glob(FS, "*.sql")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no migrations embedded")
	}
}
