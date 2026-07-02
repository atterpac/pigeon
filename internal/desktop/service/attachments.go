package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/atterpac/pigeon/internal/email"
)

// SaveAttachment writes the index-th attachment of a message to disk. When
// prompt is true it asks the user where via a native "Save as" dialog;
// otherwise it drops the file straight into the user's Downloads directory,
// de-duplicating the filename. Returns the written path, or "" if the user
// cancels the dialog. The index matches the order of Attachments (parts with
// Disposition == "attachment"), which the frontend mirrors 1:1.
func (m *Messages) SaveAttachment(ctx context.Context, acct email.Account, id email.MessageID, index int, prompt bool) (string, error) {
	atts, err := m.client.Attachments(ctx, acct, id)
	if err != nil {
		return "", err
	}
	if index < 0 || index >= len(atts) {
		return "", fmt.Errorf("attachment index %d out of range (%d available)", index, len(atts))
	}
	att := atts[index]
	content, err := m.client.PartContent(ctx, att)
	if err != nil {
		return "", err
	}
	if len(content) == 0 {
		return "", fmt.Errorf("attachment %q has no cached content", att.Filename)
	}
	name := sanitizeFilename(att.Filename)

	if prompt {
		dest, err := application.Get().Dialog.SaveFile().SetFilename(name).PromptForSingleSelection()
		if err != nil {
			return "", err
		}
		if dest == "" {
			return "", nil // user cancelled
		}
		// The user picked an explicit path, so honor it even if it overwrites.
		if err := os.WriteFile(dest, content, 0o644); err != nil {
			return "", fmt.Errorf("write attachment: %w", err)
		}
		return dest, nil
	}

	dir, err := downloadsDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	// Create the chosen name exclusively: if a concurrent save claims it between
	// uniquePath's check and the write, fail loudly instead of clobbering a file.
	dest := uniquePath(filepath.Join(dir, name))
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("write attachment: %w", err)
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return "", fmt.Errorf("write attachment: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("write attachment: %w", err)
	}
	return dest, nil
}

// downloadsDir resolves the user's Downloads directory, honoring the XDG
// override (Linux) and falling back to ~/Downloads on other platforms.
func downloadsDir() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("XDG_DOWNLOAD_DIR")); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Downloads"), nil
}

// sanitizeFilename reduces an attachment name to a single safe path element so a
// crafted name (e.g. "../../etc/passwd") can't escape the target directory.
func sanitizeFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	switch name {
	case "", ".", "..", string(filepath.Separator):
		return "attachment"
	}
	return name
}

// uniquePath returns path if free, otherwise appends " (n)" before the
// extension until it finds an unused name. Any stat error (not just
// "not exist") stops probing and yields that name, so an unreadable directory
// surfaces as a write error rather than an infinite loop.
func uniquePath(path string) string {
	if _, err := os.Stat(path); err != nil {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		candidate := fmt.Sprintf("%s (%d)%s", base, i, ext)
		if _, err := os.Stat(candidate); err != nil {
			return candidate
		}
	}
}
