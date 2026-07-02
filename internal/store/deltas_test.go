package store

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/atterpac/pigeon/internal/model"
	gen "github.com/atterpac/pigeon/internal/store/db"
)

func gmGetMessageParams(acct model.AccountID, id model.MessageID) gen.GetMessageParams {
	return gen.GetMessageParams{Account: string(acct), ID: string(id)}
}

func TestApplyFlagDeltasAndDelete(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const acct model.AccountID = "a@x.io"
	if err := s.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)}); err != nil {
		t.Fatal(err)
	}
	m := model.Message{
		ID: "<m1@x.io>", Account: acct, Thread: "<m1@x.io>", Subject: "hi",
		Flags: []model.Flag{model.FlagSeen}, Labels: []model.LabelID{"INBOX"},
		Date: time.Unix(1_700_000_000, 0),
	}
	if err := s.SaveMessages(ctx, []model.Message{m}); err != nil {
		t.Fatal(err)
	}

	// Mark unread (remove Seen) and add a label.
	if err := s.ApplyFlagDeltas(ctx, acct, []FlagDelta{{
		ID:          m.ID,
		RemoveFlags: []model.Flag{model.FlagSeen},
		AddFlags:    []model.Flag{model.FlagFlagged},
		AddLabels:   []model.LabelID{"IMPORTANT"},
	}}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Queries().GetMessage(ctx, gmGetMessageParams(acct, m.ID))
	if err != nil {
		t.Fatal(err)
	}
	flags := splitFlags(got.Flags)
	if slices.Contains(flags, model.FlagSeen) {
		t.Fatalf("expected Seen removed, got %v", flags)
	}
	if !slices.Contains(flags, model.FlagFlagged) {
		t.Fatalf("expected Flagged added, got %v", flags)
	}
	items, err := s.ThreadListItems(ctx, acct, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Unread {
		t.Fatalf("thread list should reflect unread mutation, got %#v", items)
	}

	// Mark read again; the denormalized thread row must be recalculated so a
	// reload does not show the conversation as unread.
	if err := s.ApplyFlagDeltas(ctx, acct, []FlagDelta{{ID: m.ID, AddFlags: []model.Flag{model.FlagSeen}}}); err != nil {
		t.Fatal(err)
	}
	items, err = s.ThreadListItems(ctx, acct, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Unread {
		t.Fatalf("thread list should be read after Seen is restored, got %#v", items)
	}

	// Archiving removes INBOX; the inbox conversation list should omit it on
	// the next read from SQLite.
	if err := s.ApplyFlagDeltas(ctx, acct, []FlagDelta{{ID: m.ID, RemoveLabels: []model.LabelID{"INBOX"}}}); err != nil {
		t.Fatal(err)
	}
	items, err = s.ThreadListItems(ctx, acct, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("archived thread should be omitted from inbox list, got %#v", items)
	}

	// Unknown message id is silently skipped.
	if err := s.ApplyFlagDeltas(ctx, acct, []FlagDelta{{ID: "<nope@x.io>", AddFlags: []model.Flag{model.FlagSeen}}}); err != nil {
		t.Fatalf("expected skip, got %v", err)
	}

	// Delete removes the row.
	if err := s.DeleteMessages(ctx, acct, []model.MessageID{m.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Queries().GetMessage(ctx, gmGetMessageParams(acct, m.ID)); err == nil {
		t.Fatal("expected message gone after delete")
	}
}

func TestSaveMessagesRecalculatesThreadUnread(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const acct model.AccountID = "a@x.io"
	if err := s.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)}); err != nil {
		t.Fatal(err)
	}
	msg := model.Message{
		ID: "m1", Account: acct, Thread: "t1", Subject: "hi",
		Labels: []model.LabelID{"INBOX"}, Date: time.Unix(1, 0),
	}
	if err := s.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatal(err)
	}
	items, err := s.ThreadListItems(ctx, acct, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Unread {
		t.Fatalf("initial message should be unread, got %#v", items)
	}

	msg.Flags = []model.Flag{model.FlagSeen}
	if err := s.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatal(err)
	}
	items, err = s.ThreadListItems(ctx, acct, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Unread {
		t.Fatalf("thread unread should recalculate after full upsert, got %#v", items)
	}
}

func TestThreadListUnreadOnlyCountsVisibleInboxMessages(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const acct model.AccountID = "a@x.io"
	if err := s.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)}); err != nil {
		t.Fatal(err)
	}
	msgs := []model.Message{
		{
			ID: "inbox-read", Account: acct, Thread: "t1", Subject: "hi",
			Flags: []model.Flag{model.FlagSeen}, Labels: []model.LabelID{"INBOX"},
			Date: time.Unix(1, 0),
		},
		{
			ID: "archived-unread", Account: acct, Thread: "t1", Subject: "hi",
			Labels: []model.LabelID{"Archive"}, Date: time.Unix(2, 0),
		},
	}
	if err := s.SaveMessages(ctx, msgs); err != nil {
		t.Fatal(err)
	}
	items, err := s.ThreadListItems(ctx, acct, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected visible inbox thread, got %#v", items)
	}
	if items[0].Unread {
		t.Fatalf("inbox row should be read; archived unread messages must not leak into inbox unread state: %#v", items[0])
	}
}
