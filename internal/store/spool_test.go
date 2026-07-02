package store

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/atterpac/pigeon/internal/blob"
	"github.com/atterpac/pigeon/internal/model"
)

// seedBody opens a blob-backed store with one account and one message ready for
// SaveBody. Returns the store, account, and message id.
func seedBody(t *testing.T) (*Store, model.AccountID, model.MessageID) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(ctx, filepath.Join(dir, "t.db"), WithBlobStore(blob.NewFS(filepath.Join(dir, "blobs"))))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	const acct model.AccountID = "a@x.io"
	if err := s.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)}); err != nil {
		t.Fatal(err)
	}
	msg := model.Message{ID: "<m1@x.io>", Account: acct, Thread: "<m1@x.io>", Subject: "report", Date: time.Unix(1_700_000_000, 0)}
	if err := s.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatal(err)
	}
	return s, acct, msg.ID
}

func TestSaveBodySpoolsLargeAttachment(t *testing.T) {
	ctx := context.Background()
	s, acct, id := seedBody(t)

	big := bytes.Repeat([]byte("Z"), spoolThreshold+1)
	parts := []model.Part{
		{ContentType: "text/plain", Disposition: "inline", Size: 5, Content: []byte("hello")},
		{ContentType: "application/pdf", Disposition: "attachment", Filename: "big.pdf", Size: int64(len(big)), Content: big},
	}
	if err := s.SaveBody(ctx, acct, id, parts, "hello", ""); err != nil {
		t.Fatal(err)
	}

	got, err := s.Parts(ctx, acct, id)
	if err != nil {
		t.Fatal(err)
	}
	var inline, attach model.Part
	for _, p := range got {
		switch p.Disposition {
		case "inline":
			inline = p
		case "attachment":
			attach = p
		}
	}

	// Inline part stays in the row.
	if string(inline.Content) != "hello" || inline.BlobRef != "" {
		t.Fatalf("inline part should stay inline, got content=%q ref=%q", inline.Content, inline.BlobRef)
	}
	// Attachment is spooled: empty Content, a ref, and Size preserved.
	if len(attach.Content) != 0 {
		t.Fatalf("spooled attachment should have empty Content, got %d bytes", len(attach.Content))
	}
	if attach.BlobRef == "" {
		t.Fatal("spooled attachment missing BlobRef")
	}
	if attach.Size != int64(len(big)) {
		t.Fatalf("spooled attachment Size = %d, want %d", attach.Size, len(big))
	}

	// Bytes round-trip through the blob store.
	rc, err := s.BlobContent(ctx, attach.BlobRef)
	if err != nil {
		t.Fatalf("BlobContent: %v", err)
	}
	defer func() { _ = rc.Close() }()
	hydrated, err := io.ReadAll(rc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(hydrated, big) {
		t.Fatalf("hydrated bytes mismatch: %d vs %d", len(hydrated), len(big))
	}
}

func TestSaveBodyKeepsSmallAttachmentInline(t *testing.T) {
	ctx := context.Background()
	s, acct, id := seedBody(t)

	small := []byte("a,b,c\n1,2,3")
	parts := []model.Part{
		{ContentType: "text/csv", Disposition: "attachment", Filename: "q3.csv", Size: int64(len(small)), Content: small},
	}
	if err := s.SaveBody(ctx, acct, id, parts, "", ""); err != nil {
		t.Fatal(err)
	}
	got, err := s.Parts(ctx, acct, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || string(got[0].Content) != string(small) || got[0].BlobRef != "" {
		t.Fatalf("small attachment should stay inline, got %+v", got)
	}
}

func TestSweepBlobsReclaimsOrphans(t *testing.T) {
	ctx := context.Background()
	s, acct, id := seedBody(t)

	big := bytes.Repeat([]byte("Z"), spoolThreshold+1)
	parts := []model.Part{{ContentType: "application/pdf", Disposition: "attachment", Filename: "big.pdf", Size: int64(len(big)), Content: big}}
	if err := s.SaveBody(ctx, acct, id, parts, "", ""); err != nil {
		t.Fatal(err)
	}
	ref := mustRef(t, s, ctx, acct, id)

	now := time.Now()
	// Still referenced → not swept, regardless of age.
	if n, err := s.SweepBlobs(ctx, now.Add(48*time.Hour), 0); err != nil || n != 0 {
		t.Fatalf("referenced blob swept: n=%d err=%v", n, err)
	}

	// Orphan it by replacing the body with no parts.
	if err := s.SaveBody(ctx, acct, id, nil, "", ""); err != nil {
		t.Fatal(err)
	}

	// Young orphan is kept by the grace window.
	if n, err := s.SweepBlobs(ctx, now, time.Hour); err != nil || n != 0 {
		t.Fatalf("young orphan swept: n=%d err=%v", n, err)
	}
	// Past the grace window it's reclaimed.
	if n, err := s.SweepBlobs(ctx, now.Add(2*time.Hour), time.Hour); err != nil || n != 1 {
		t.Fatalf("expected 1 orphan reclaimed, got n=%d err=%v", n, err)
	}
	// And the bytes are gone.
	if _, err := s.BlobContent(ctx, ref); err == nil {
		t.Fatal("expected BlobContent to fail after sweep")
	}
}

func mustRef(t *testing.T, s *Store, ctx context.Context, acct model.AccountID, id model.MessageID) string {
	t.Helper()
	parts, err := s.Parts(ctx, acct, id)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range parts {
		if p.BlobRef != "" {
			return p.BlobRef
		}
	}
	t.Fatal("no spooled part found")
	return ""
}
