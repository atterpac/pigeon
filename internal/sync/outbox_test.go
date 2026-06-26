package sync

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
	"github.com/atterpac/email/internal/store"
)

func TestOutboxEnqueueDrainAndRetry(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	const acct model.AccountID = "a@x.io"
	if err := st.UpsertAccount(ctx, model.Account{ID: acct, Email: string(acct)}); err != nil {
		t.Fatal(err)
	}
	eng := New(st)

	if err := eng.EnqueueSend(ctx, acct, model.RawMessage{Bytes: []byte("RAW BYTES")}, provider.SendOpts{}); err != nil {
		t.Fatal(err)
	}

	// Base the test clock just after enqueue so the op is initially due.
	t0 := time.Now().Add(time.Minute)

	// First drain: provider fails, op stays queued (rescheduled into the future).
	p := &fakeProvider{failSends: 1}
	if n, err := eng.drainOutboxAt(ctx, p, acct, t0); err != nil || n != 0 {
		t.Fatalf("expected 0 sent on failure, got n=%d err=%v", n, err)
	}
	if len(p.sent) != 0 {
		t.Fatalf("expected nothing delivered, got %d", len(p.sent))
	}

	// Immediate retry at the same instant finds nothing ready — backoff works.
	if n, _ := eng.drainOutboxAt(ctx, p, acct, t0); n != 0 {
		t.Fatalf("expected op to be backed off, but it ran (n=%d)", n)
	}

	// After the backoff window the op is ready again and succeeds.
	n, err := eng.drainOutboxAt(ctx, p, acct, t0.Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("expected 1 sent, got n=%d err=%v", n, err)
	}
	if len(p.sent) != 1 || string(p.sent[0]) != "RAW BYTES" {
		t.Fatalf("delivered payload mismatch: %v", p.sent)
	}

	// Outbox now empty.
	if n, _ := eng.DrainOutbox(ctx, p, acct); n != 0 {
		t.Fatalf("expected empty outbox, drained %d", n)
	}
}
