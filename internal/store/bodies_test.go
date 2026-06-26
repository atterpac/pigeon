package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/atterpac/email/internal/events"
	"github.com/atterpac/email/internal/mime"
	"github.com/atterpac/email/internal/model"
)

func TestSaveBodyParsesAttachmentsAndIndexes(t *testing.T) {
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
	msg := model.Message{ID: "<m1@x.io>", Account: acct, Thread: "<m1@x.io>", Subject: "report", Date: time.Unix(1_700_000_000, 0)}
	if err := s.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatal(err)
	}

	// Build a real message with a body and an attachment, then parse it.
	raw, err := mime.Build(model.Outgoing{
		From: model.Address{Addr: "a@x.io"}, To: []model.Address{{Addr: "b@y.io"}},
		Subject: "report", Text: "quarterly figures attached",
		Attachments: []model.Outfile{{Filename: "q3.csv", ContentType: "text/csv", Content: []byte("a,b,c\n1,2,3")}},
	}, time.Unix(1_700_000_000, 0), "<m1@x.io>")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mime.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.SaveBody(ctx, acct, msg.ID, parsed.Parts, parsed.Text, ""); err != nil {
		t.Fatal(err)
	}

	parts, err := s.Parts(ctx, acct, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	var gotAttachment bool
	for _, p := range parts {
		if p.Disposition == "attachment" && p.Filename == "q3.csv" {
			gotAttachment = true
			if string(p.Content) != "a,b,c\n1,2,3" {
				t.Fatalf("attachment content mismatch: %q", p.Content)
			}
		}
	}
	if !gotAttachment {
		t.Fatalf("expected q3.csv attachment, got parts %+v", parts)
	}

	// Body text is now searchable.
	res, err := s.Search(ctx, acct, "quarterly", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].ID != msg.ID {
		t.Fatalf("expected body-text search hit, got %+v", res)
	}

	if loaded, _ := s.IsBodyLoaded(ctx, acct, msg.ID); !loaded {
		t.Fatal("expected body_loaded=true")
	}
}

func TestChangefeedPublishes(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	const acct model.AccountID = "a@x.io"
	_ = s.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)})

	ch, cancel := s.Events().Subscribe()
	defer cancel()

	if err := s.SaveMessages(ctx, []model.Message{{ID: "<m1@x.io>", Account: acct, Date: time.Unix(1, 0)}}); err != nil {
		t.Fatal(err)
	}
	select {
	case e := <-ch:
		if e.Kind != events.KindUpsert || len(e.IDs) != 1 {
			t.Fatalf("unexpected event: %+v", e)
		}
	case <-time.After(time.Second):
		t.Fatal("no event received")
	}
}

func TestSaveMessagesAndBodySetCategory(t *testing.T) {
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
		ID:      "<m1@x.io>",
		Account: acct,
		Thread:  "<m1@x.io>",
		Subject: "Lunch tomorrow?",
		From:    []model.Address{{Name: "Jane", Addr: "jane@example.com"}},
		Date:    time.Unix(1, 0),
	}
	if err := s.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatal(err)
	}
	stored, err := s.Message(ctx, acct, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Category != model.CategoryPrimary {
		t.Fatalf("expected primary after envelope save, got %q", stored.Category)
	}

	if err := s.SaveBody(ctx, acct, msg.ID, nil, "shop now before this sale ends tomorrow", model.CategoryPromotions); err != nil {
		t.Fatal(err)
	}
	stored, err = s.Message(ctx, acct, msg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Category != model.CategoryPromotions {
		t.Fatalf("expected promotions after body save, got %q", stored.Category)
	}
}
