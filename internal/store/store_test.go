package store

import (
	"context"
	"path/filepath"
	"testing"

	gen "github.com/atterpac/pigeon/internal/store/db"
)

func TestOpenMigrateRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "test.db")

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer s.Close()

	q := s.Queries()
	if err := q.UpsertAccount(ctx, gen.UpsertAccountParams{
		ID: "a1", Kind: 1, Email: "michael@getgalaxy.io", Name: "Michael",
	}); err != nil {
		t.Fatalf("upsert account: %v", err)
	}

	got, err := q.GetAccount(ctx, "a1")
	if err != nil {
		t.Fatalf("get account: %v", err)
	}
	if got.Email != "michael@getgalaxy.io" || got.Kind != 1 {
		t.Fatalf("round-trip mismatch: %+v", got)
	}

	if err := q.UpsertMessage(ctx, gen.UpsertMessageParams{
		ID: "m1", Account: "a1", Thread: "t1", Subject: "hello",
	}); err != nil {
		t.Fatalf("upsert message: %v", err)
	}
	msgs, err := q.ListThreadMessages(ctx, gen.ListThreadMessagesParams{Account: "a1", Thread: "t1"})
	if err != nil {
		t.Fatalf("list thread: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Subject != "hello" {
		t.Fatalf("expected 1 message 'hello', got %+v", msgs)
	}
}
