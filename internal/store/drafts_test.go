package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/atterpac/pigeon/internal/model"
)

func TestDraftsRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const acct model.AccountID = "a@x.io"
	_ = s.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)})

	id, err := s.SaveDraft(ctx, acct, "", model.Outgoing{Subject: "wip", Text: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		t.Fatal("expected generated id")
	}
	// Autosave update under the same id.
	if _, err := s.SaveDraft(ctx, acct, id, model.Outgoing{Subject: "wip", Text: "hello world"}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetDraft(ctx, acct, id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Message.Text != "hello world" {
		t.Fatalf("draft not updated: %q", got.Message.Text)
	}
	list, _ := s.ListDrafts(ctx, acct)
	if len(list) != 1 {
		t.Fatalf("want 1 draft, got %d", len(list))
	}
	if err := s.DeleteDraft(ctx, acct, id); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListDrafts(ctx, acct); len(list) != 0 {
		t.Fatalf("expected drafts cleared, got %d", len(list))
	}
}

func TestSnoozeAndDone(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	const acct model.AccountID = "a@x.io"
	_ = s.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)})

	ids := []model.MessageID{"<1@x>", "<2@x>"}
	future := time.Now().Add(time.Hour)
	if err := s.Snooze(ctx, acct, ids, future); err != nil {
		t.Fatal(err)
	}
	// Nothing due yet.
	if due, _ := s.DueSnoozes(ctx, acct, time.Now()); len(due) != 0 {
		t.Fatalf("expected nothing due, got %d", len(due))
	}
	// Due after the window.
	due, _ := s.DueSnoozes(ctx, acct, future.Add(time.Minute))
	if len(due) != 2 {
		t.Fatalf("expected 2 due, got %d", len(due))
	}
	if snz, _ := s.ListSnoozes(ctx, acct); len(snz) != 2 {
		t.Fatalf("expected 2 snoozes listed, got %d", len(snz))
	}
	if err := s.Unsnooze(ctx, acct, ids); err != nil {
		t.Fatal(err)
	}
	if snz, _ := s.ListSnoozes(ctx, acct); len(snz) != 0 {
		t.Fatalf("expected snoozes cleared, got %d", len(snz))
	}

	// Done metric.
	midnight := time.Now().Truncate(24 * time.Hour)
	if err := s.RecordDone(ctx, acct, ids, time.Now()); err != nil {
		t.Fatal(err)
	}
	n, err := s.DoneSince(ctx, acct, midnight)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 done, got %d", n)
	}
}
