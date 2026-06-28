package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/atterpac/email/internal/model"
)

func TestContactsHarvestAndSearch(t *testing.T) {
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

	// Alice appears in three messages (highest frequency); Bob once. Carol is a
	// recipient. The blank/garbage addresses must be skipped entirely.
	msgs := []model.Message{
		{ID: "<1@x>", Account: acct, Thread: "t1", Date: time.Unix(1_700_000_100, 0),
			From: []model.Address{{Name: "Alice", Addr: "alice@acme.io"}},
			To:   []model.Address{{Name: "Carol", Addr: "carol@other.io"}, {Addr: "not-an-address"}}},
		{ID: "<2@x>", Account: acct, Thread: "t1", Date: time.Unix(1_700_000_300, 0),
			From: []model.Address{{Name: "Alice Smith", Addr: "ALICE@acme.io"}}}, // case-folds to same key, newer name
		{ID: "<3@x>", Account: acct, Thread: "t2", Date: time.Unix(1_700_000_200, 0),
			From: []model.Address{{Name: "Alice", Addr: "alice@acme.io"}},
			Cc:   []model.Address{{Name: "Bob", Addr: "bob@acme.io"}, {Addr: ""}}},
	}
	if err := s.SaveMessages(ctx, msgs); err != nil {
		t.Fatal(err)
	}

	// Substring on the address domain: the two @acme.io contacts (Alice, Bob),
	// Alice first (freq 3).
	got, err := s.SearchContacts(ctx, acct, "acme", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 acme contacts, got %d: %+v", len(got), got)
	}
	if got[0].Addr != "alice@acme.io" || got[0].Freq != 3 {
		t.Fatalf("expected alice ranked first with freq 3, got %+v", got[0])
	}
	if got[0].Name != "Alice Smith" {
		t.Fatalf("expected most-recent display name 'Alice Smith', got %q", got[0].Name)
	}
	if got[0].LastSeen.Unix() != 1_700_000_300 {
		t.Fatalf("expected last_seen to track the newest message, got %d", got[0].LastSeen.Unix())
	}

	// Match on display name, and confirm the garbage address never landed.
	byName, err := s.SearchContacts(ctx, acct, "carol", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(byName) != 1 || byName[0].Addr != "carol@other.io" {
		t.Fatalf("expected single carol match, got %+v", byName)
	}
	all, err := s.SearchContacts(ctx, acct, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 harvested contacts (no blanks/garbage), got %d: %+v", len(all), all)
	}
}
