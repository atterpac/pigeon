package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/atterpac/pigeon/internal/model"
	"github.com/atterpac/pigeon/internal/provider"
	"github.com/atterpac/pigeon/internal/store"
)

// fakeProvider serves a fixed list of messages, newest-first, with paging.
type fakeProvider struct {
	mailbox model.Mailbox
	msgs    []model.Message // index 0 = newest

	failSends int      // fail this many Send calls before succeeding
	sent      [][]byte // captured successful sends

	flagCalls   int // ApplyFlags invocations
	labelCalls  int // ApplyLabels invocations
	moveCalls   int // Move invocations
	deleteCalls int // Delete invocations
}

func (f *fakeProvider) ListMailboxes(context.Context) ([]model.Mailbox, error) {
	return []model.Mailbox{f.mailbox}, nil
}

func (f *fakeProvider) Sync(context.Context, provider.MailboxRef, *provider.Cursor) (provider.Changes, *provider.Cursor, error) {
	return provider.Changes{}, &provider.Cursor{Bytes: []byte("forward")}, nil
}

func (f *fakeProvider) Backfill(_ context.Context, _ provider.MailboxRef, page *provider.Cursor, limit int) (provider.Changes, *provider.Cursor, bool, error) {
	start := 0
	if page != nil {
		_ = json.Unmarshal(page.Bytes, &start)
	}
	end := min(start+limit, len(f.msgs))
	ch := provider.Changes{Upserted: f.msgs[start:end]}
	if end >= len(f.msgs) {
		return ch, nil, true, nil
	}
	b, _ := json.Marshal(end)
	return ch, &provider.Cursor{Bytes: b}, false, nil
}

func (f *fakeProvider) FetchBodies(context.Context, provider.MailboxRef, []model.MessageID) ([]model.RawMessage, error) {
	return nil, nil
}
func (f *fakeProvider) Search(context.Context, provider.MailboxRef, string, int) ([]model.Message, error) {
	return nil, nil
}
func (f *fakeProvider) ApplyFlags(context.Context, []model.MessageID, []model.Flag, []model.Flag) error {
	f.flagCalls++
	return nil
}
func (f *fakeProvider) ApplyLabels(context.Context, []model.MessageID, []model.LabelID, []model.LabelID) error {
	f.labelCalls++
	return nil
}
func (f *fakeProvider) Move(context.Context, []model.MessageID, provider.MailboxRef) error {
	f.moveCalls++
	return nil
}
func (f *fakeProvider) CreateMailbox(context.Context, string) (model.Mailbox, error) {
	return model.Mailbox{}, nil
}
func (f *fakeProvider) RenameMailbox(context.Context, provider.MailboxRef, string) (model.Mailbox, error) {
	return model.Mailbox{}, nil
}
func (f *fakeProvider) DeleteMailbox(context.Context, provider.MailboxRef) error {
	return nil
}
func (f *fakeProvider) Delete(context.Context, []model.MessageID) error {
	f.deleteCalls++
	return nil
}
func (f *fakeProvider) Send(_ context.Context, raw model.RawMessage, _ provider.SendOpts) (model.MessageID, error) {
	if f.failSends > 0 {
		f.failSends--
		return "", fmt.Errorf("simulated transient send failure")
	}
	f.sent = append(f.sent, raw.Bytes)
	return "sent-id", nil
}
func (f *fakeProvider) SaveDraft(context.Context, model.RawMessage) (model.MessageID, error) {
	return "", nil
}
func (f *fakeProvider) Watch(context.Context) (<-chan provider.MailboxRef, error) { return nil, nil }
func (f *fakeProvider) Capabilities() provider.Caps                               { return provider.Caps{} }
func (f *fakeProvider) Close() error                                              { return nil }

func TestBackfillAndSearch(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = st.Close() }()

	const acct model.AccountID = "a@x.io"
	mb := model.Mailbox{ID: "INBOX", Account: acct, Name: "INBOX", Role: model.RoleInbox}

	var msgs []model.Message
	for i := 0; i < 25; i++ {
		msgs = append(msgs, model.Message{
			ID:      model.MessageID(fmt.Sprintf("<m%d@x.io>", i)),
			Thread:  model.ThreadID(fmt.Sprintf("<m%d@x.io>", i)),
			Account: acct,
			Subject: fmt.Sprintf("message about widgets %d", i),
			From:    []model.Address{{Name: "Sender", Addr: "sender@x.io"}},
			Date:    time.Unix(int64(1_700_000_000+i), 0),
			Labels:  []model.LabelID{"INBOX"},
		})
	}
	p := &fakeProvider{mailbox: mb, msgs: msgs}
	eng := New(st)

	if _, err := eng.RegisterAccount(ctx, p, model.Account{ID: acct, Email: string(acct)}); err != nil {
		t.Fatal(err)
	}

	ref := provider.MailboxRef{ID: "INBOX", Path: "INBOX"}
	total, err := eng.BackfillAll(ctx, p, acct, ref, 10, nil) // 3 pages: 10+10+5
	if err != nil {
		t.Fatal(err)
	}
	if total != 25 {
		t.Fatalf("expected 25 backfilled, got %d", total)
	}

	res, err := st.Search(ctx, acct, "widgets", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 25 {
		t.Fatalf("expected 25 search hits, got %d", len(res))
	}
	// Newest first.
	if res[0].Subject != "message about widgets 24" {
		t.Fatalf("expected newest first, got %q", res[0].Subject)
	}

	// Idempotent: re-running backfill from scratch shouldn't duplicate rows.
	if err := st.SetBackfill(ctx, acct, "INBOX", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := eng.BackfillAll(ctx, p, acct, ref, 10, nil); err != nil {
		t.Fatal(err)
	}
	res2, _ := st.Search(ctx, acct, "widgets", 50)
	if len(res2) != 25 {
		t.Fatalf("dedup failed: expected 25 after re-backfill, got %d", len(res2))
	}
}
