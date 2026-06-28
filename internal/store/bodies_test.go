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

func TestSaveMessagesPreservesLoadedBody(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	const acct model.AccountID = "a1"
	if err := s.UpsertAccount(ctx, model.Account{ID: acct, Email: "a1@example.com"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	msg := model.Message{
		ID: "m1", Account: acct, Thread: "t1", Subject: "hello",
		From: []model.Address{{Addr: "sender@example.com"}},
		Date: time.Unix(100, 0), Labels: []model.LabelID{"INBOX"},
	}
	if err := s.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatalf("save envelope: %v", err)
	}
	if err := s.SaveBody(ctx, acct, msg.ID, []model.Part{{ContentType: "text/plain", Content: []byte("cached body")}}, "cached body", ""); err != nil {
		t.Fatalf("save body: %v", err)
	}

	msg.Snippet = "new envelope snippet"
	msg.BodyLoaded = false
	if err := s.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatalf("resave envelope: %v", err)
	}
	if loaded, err := s.IsBodyLoaded(ctx, acct, msg.ID); err != nil || !loaded {
		t.Fatalf("body_loaded should survive envelope upsert, loaded=%v err=%v", loaded, err)
	}
}

func TestPruneBodiesDropsBodyCacheOnly(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	const acct model.AccountID = "a1"
	if err := s.UpsertAccount(ctx, model.Account{ID: acct, Email: "a1@example.com"}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}
	msg := model.Message{
		ID: "m1", Account: acct, Thread: "t1", Subject: "plain subject",
		From: []model.Address{{Addr: "sender@example.com"}},
		Date: time.Unix(100, 0), Flags: []model.Flag{model.FlagSeen}, Labels: []model.LabelID{"INBOX"},
	}
	if err := s.SaveMessages(ctx, []model.Message{msg}); err != nil {
		t.Fatalf("save envelope: %v", err)
	}
	if err := s.SaveBody(ctx, acct, msg.ID, []model.Part{{ContentType: "text/plain", Size: 24, Content: []byte("needle-body-term")}}, "needle-body-term", ""); err != nil {
		t.Fatalf("save body: %v", err)
	}
	old := time.Unix(10, 0).Unix()
	if _, err := s.DB().ExecContext(ctx, `UPDATE messages SET body_cached_at = ? WHERE account = ? AND id = ?`, old, string(acct), string(msg.ID)); err != nil {
		t.Fatalf("age message body cache: %v", err)
	}
	if _, err := s.DB().ExecContext(ctx, `UPDATE parts SET cached_at = ? WHERE account = ? AND message = ?`, old, string(acct), string(msg.ID)); err != nil {
		t.Fatalf("age parts body cache: %v", err)
	}

	hits, err := s.Search(ctx, acct, "needle-body-term", 10)
	if err != nil {
		t.Fatalf("search before prune: %v", err)
	}
	if len(hits) != 1 || hits[0].ID != msg.ID {
		t.Fatalf("expected body search hit before prune, got %+v", hits)
	}

	result, err := s.PruneBodies(ctx, acct, BodyRetentionPolicy{
		MaxAge:      time.Hour,
		KeepUnread:  true,
		KeepStarred: true,
	}, time.Unix(10_000, 0))
	if err != nil {
		t.Fatalf("prune bodies: %v", err)
	}
	if result.Messages != 1 || result.Bytes == 0 {
		t.Fatalf("unexpected prune result: %+v", result)
	}
	stored, err := s.Message(ctx, acct, msg.ID)
	if err != nil {
		t.Fatalf("message after prune: %v", err)
	}
	if stored.BodyLoaded {
		t.Fatal("expected body_loaded=false after prune")
	}
	parts, err := s.Parts(ctx, acct, msg.ID)
	if err != nil {
		t.Fatalf("parts after prune: %v", err)
	}
	if len(parts) != 0 {
		t.Fatalf("expected body parts pruned, got %+v", parts)
	}
	hits, err = s.Search(ctx, acct, "needle-body-term", 10)
	if err != nil {
		t.Fatalf("search after prune: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("expected body-only search term to disappear after prune, got %+v", hits)
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
