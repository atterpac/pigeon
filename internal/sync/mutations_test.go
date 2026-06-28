package sync

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/store"
)

func TestMutationsOptimisticAndDrained(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	const acct model.AccountID = "a@x.io"
	_ = st.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)})
	m := model.Message{
		ID: "<m1@x.io>", Account: acct, Subject: "hi",
		Flags: []model.Flag{model.FlagSeen}, Labels: []model.LabelID{"INBOX"},
		Date: time.Unix(1, 0),
	}
	if err := st.SaveMessages(ctx, []model.Message{m}); err != nil {
		t.Fatal(err)
	}
	eng := New(st)
	ids := []model.MessageID{m.ID}

	// Mark unread: optimistic local change removes FlagSeen immediately.
	if err := eng.SetFlags(ctx, acct, ids, nil, []model.Flag{model.FlagSeen}); err != nil {
		t.Fatal(err)
	}
	got, _ := st.Message(ctx, acct, m.ID)
	if slices.Contains(got.Flags, model.FlagSeen) {
		t.Fatal("expected Seen removed optimistically before drain")
	}

	// Archive: optimistic INBOX removal.
	if err := eng.ApplyLabels(ctx, acct, ids, nil, []model.LabelID{"INBOX"}); err != nil {
		t.Fatal(err)
	}

	// Drain: provider receives both mutations.
	p := &fakeProvider{}
	if _, err := eng.DrainOutbox(ctx, p, acct); err != nil {
		t.Fatal(err)
	}
	if p.flagCalls != 1 {
		t.Fatalf("expected 1 ApplyFlags call, got %d", p.flagCalls)
	}
	if p.labelCalls != 1 {
		t.Fatalf("expected 1 ApplyLabels call, got %d", p.labelCalls)
	}

	// Delete: optimistic local removal + queued provider delete.
	if err := eng.Delete(ctx, acct, ids); err != nil {
		t.Fatal(err)
	}
	if _, err := st.Message(ctx, acct, m.ID); err == nil {
		t.Fatal("expected message removed optimistically")
	}
	if _, err := eng.DrainOutbox(ctx, p, acct); err != nil {
		t.Fatal(err)
	}
	if p.deleteCalls != 1 {
		t.Fatalf("expected 1 Delete call, got %d", p.deleteCalls)
	}
}

func TestMoveRemovesInboxLocallyAndDrainsProviderMove(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	const acct model.AccountID = "a@x.io"
	_ = st.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)})
	msg := model.Message{
		ID: "m1", Account: acct, Thread: "t1", Subject: "hi",
		Labels: []model.LabelID{"INBOX"}, Date: time.Unix(1, 0),
	}
	if err := st.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatal(err)
	}

	eng := New(st)
	if err := eng.Move(ctx, acct, []model.MessageID{msg.ID}, "Archive"); err != nil {
		t.Fatal(err)
	}
	got, err := st.Message(ctx, acct, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(got.Labels, model.LabelID("INBOX")) {
		t.Fatalf("expected move to remove INBOX locally, got %v", got.Labels)
	}
	if !slices.Contains(got.Labels, model.LabelID("Archive")) {
		t.Fatalf("expected move to add archive label locally, got %v", got.Labels)
	}

	p := &fakeProvider{}
	if _, err := eng.DrainOutbox(ctx, p, acct); err != nil {
		t.Fatal(err)
	}
	if p.moveCalls != 1 {
		t.Fatalf("expected 1 Move call, got %d", p.moveCalls)
	}
}

// Moving a message back INTO the inbox (the undo-archive path) must keep the
// INBOX label. Regression: adds run before removes in ApplyFlagDeltas, so a
// blanket "remove INBOX" cancelled the add and the mail never reappeared.
func TestMoveIntoInboxKeepsInboxLabel(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	const acct model.AccountID = "a@x.io"
	_ = st.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)})
	// Start in Archive (as if previously archived), not in the inbox.
	msg := model.Message{
		ID: "m1", Account: acct, Thread: "t1", Subject: "hi",
		Labels: []model.LabelID{"Archive"}, Date: time.Unix(1, 0),
	}
	if err := st.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatal(err)
	}

	eng := New(st)
	if err := eng.Move(ctx, acct, []model.MessageID{msg.ID}, "INBOX"); err != nil {
		t.Fatal(err)
	}
	got, err := st.Message(ctx, acct, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(got.Labels, model.LabelID("INBOX")) {
		t.Fatalf("expected move into inbox to keep INBOX label, got %v", got.Labels)
	}
}
