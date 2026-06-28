package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/atterpac/email/internal/model"
)

func seed(t *testing.T) (*Store, context.Context, model.AccountID) {
	t.Helper()
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	const acct model.AccountID = "a@x.io"
	if err := s.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)}); err != nil {
		t.Fatal(err)
	}
	msgs := []model.Message{
		{ID: "<1@x>", Account: acct, Thread: "t1", Subject: "Invoice March",
			From: []model.Address{{Name: "Alice", Addr: "alice@acme.io"}}, Date: time.Unix(1_700_000_100, 0),
			Flags: []model.Flag{model.FlagSeen}, Labels: []model.LabelID{"INBOX", "WORK"}, HasAttachments: true},
		{ID: "<2@x>", Account: acct, Thread: "t1", Subject: "Re: Invoice March",
			From: []model.Address{{Name: "Bob", Addr: "bob@acme.io"}}, Date: time.Unix(1_700_000_200, 0),
			Flags: nil /* unread */, Labels: []model.LabelID{"INBOX"}},
		{ID: "<3@x>", Account: acct, Thread: "t2", Subject: "Lunch?",
			From: []model.Address{{Addr: "carol@other.io"}}, Date: time.Unix(1_700_000_300, 0),
			Flags: []model.Flag{model.FlagSeen}, Labels: []model.LabelID{"INBOX"}},
	}
	if err := s.SaveMessages(ctx, msgs); err != nil {
		t.Fatal(err)
	}
	return s, ctx, acct
}

func TestSearchOperators(t *testing.T) {
	s, ctx, acct := seed(t)
	defer s.Close()

	cases := []struct {
		q    string
		want int
	}{
		{"invoice", 2},             // FTS body/subject
		{"from:bob", 1},            // sender filter
		{"is:unread", 1},           // <2@x>
		{"has:attachment", 1},      // <1@x>
		{"label:WORK", 1},          // <1@x>
		{"after:2023-11-14", 3},    // all (epoch ~2023-11-14/15)
		{"invoice is:unread", 1},   // combine FTS + filter
		{"from:acme is:unread", 1}, // <2@x>
	}
	for _, c := range cases {
		got, err := s.Search(ctx, acct, c.q, 50)
		if err != nil {
			t.Fatalf("%q: %v", c.q, err)
		}
		if len(got) != c.want {
			t.Errorf("%q: want %d, got %d", c.q, c.want, len(got))
		}
	}
}

func TestSearchMatchesTokenPrefix(t *testing.T) {
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
		ID: "m1", Account: acct, Thread: "t1", Subject: "Baboon report",
		From: []model.Address{{Addr: "sender@example.com"}}, Date: time.Unix(1_700_000_000, 0),
		Labels: []model.LabelID{"INBOX"},
	}
	if err := s.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Search(ctx, acct, "Baboo", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != msg.ID {
		t.Fatalf("prefix search should match Baboon, got %#v", got)
	}
}

func TestThreadListItems(t *testing.T) {
	s, ctx, acct := seed(t)
	defer s.Close()

	items, err := s.ThreadListItems(ctx, acct, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 threads, got %d", len(items))
	}
	// Newest thread (t2) first by last activity.
	byID := map[model.ThreadID]model.ThreadListItem{}
	for _, it := range items {
		byID[it.ID] = it
	}
	t1 := byID["t1"]
	if t1.Count != 2 {
		t.Errorf("t1 count: want 2, got %d", t1.Count)
	}
	if len(t1.Participants) != 2 {
		t.Errorf("t1 participants: want 2 (Alice, Bob), got %d", len(t1.Participants))
	}
	if t1.LatestSender.Addr != "bob@acme.io" {
		t.Errorf("t1 latest sender: want bob, got %q", t1.LatestSender.Addr)
	}
	if !t1.Unread {
		t.Errorf("t1 should be unread (msg 2)")
	}
	if !t1.HasAttachments {
		t.Errorf("t1 should have attachments (msg 1)")
	}
	// Labels union across the thread.
	hasWork, hasInbox := false, false
	for _, l := range t1.Labels {
		if l == "WORK" {
			hasWork = true
		}
		if l == "INBOX" {
			hasInbox = true
		}
	}
	if !hasWork || !hasInbox {
		t.Errorf("t1 labels union missing: %v", t1.Labels)
	}
}

func TestLabelsOnReads(t *testing.T) {
	s, ctx, acct := seed(t)
	defer s.Close()

	m, err := s.Message(ctx, acct, "<1@x>")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Labels) != 2 {
		t.Fatalf("want 2 labels on read, got %v", m.Labels)
	}
}
