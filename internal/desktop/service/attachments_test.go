package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizeFilename(t *testing.T) {
	sep := string(filepath.Separator)
	cases := []struct {
		name, in, want string
	}{
		{"plain name kept", "report.pdf", "report.pdf"},
		{"surrounding space trimmed", "  photo.jpg  ", "photo.jpg"},
		{"path traversal stripped to base", "../../etc/passwd", "passwd"},
		{"nested path stripped to base", "a/b/c/invoice.txt", "invoice.txt"},
		{"empty becomes placeholder", "", "attachment"},
		{"whitespace-only becomes placeholder", "   ", "attachment"},
		{"dot becomes placeholder", ".", "attachment"},
		{"dotdot becomes placeholder", "..", "attachment"},
		{"bare separator becomes placeholder", sep, "attachment"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sanitizeFilename(c.in); got != c.want {
				t.Fatalf("sanitizeFilename(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()

	// A free path is returned unchanged.
	free := filepath.Join(dir, "doc.txt")
	if got := uniquePath(free); got != free {
		t.Fatalf("uniquePath(free) = %q, want %q", got, free)
	}

	// First collision appends " (1)" before the extension.
	if err := os.WriteFile(free, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	want1 := filepath.Join(dir, "doc (1).txt")
	if got := uniquePath(free); got != want1 {
		t.Fatalf("uniquePath(taken) = %q, want %q", got, want1)
	}

	// With " (1)" also taken, it advances to " (2)".
	if err := os.WriteFile(want1, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	want2 := filepath.Join(dir, "doc (2).txt")
	if got := uniquePath(free); got != want2 {
		t.Fatalf("uniquePath(taken twice) = %q, want %q", got, want2)
	}

	// Extensionless names get the suffix at the end.
	noExt := filepath.Join(dir, "LICENSE")
	if err := os.WriteFile(noExt, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := uniquePath(noExt), filepath.Join(dir, "LICENSE (1)"); got != want {
		t.Fatalf("uniquePath(no ext) = %q, want %q", got, want)
	}
}

func TestDownloadsDir(t *testing.T) {
	t.Run("honors XDG override", func(t *testing.T) {
		t.Setenv("XDG_DOWNLOAD_DIR", "/custom/dl")
		got, err := downloadsDir()
		if err != nil {
			t.Fatal(err)
		}
		if got != "/custom/dl" {
			t.Fatalf("downloadsDir = %q, want /custom/dl", got)
		}
	})

	t.Run("falls back to ~/Downloads", func(t *testing.T) {
		t.Setenv("XDG_DOWNLOAD_DIR", "")
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home dir")
		}
		got, err := downloadsDir()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, "Downloads"); got != want {
			t.Fatalf("downloadsDir = %q, want %q", got, want)
		}
	})
}
