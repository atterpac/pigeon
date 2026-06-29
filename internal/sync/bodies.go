package sync

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"sync"

	"github.com/atterpac/email/internal/classify"
	"github.com/atterpac/email/internal/mime"
	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
	"github.com/atterpac/email/internal/store"
)

// warmChunk is the number of bodies fetched per launch-warm round. Warming in
// small chunks releases the shared connection's lock between rounds (letting
// forward sync and mutations interleave) rather than holding it for one large
// fetch of the whole warm window.
const warmChunk = 10

// foregroundFetcher is implemented by providers that offer a connection reserved
// for interactive body fetches (see imap.Provider.FetchBodiesForeground). When
// present, a foreground open uses it so it never queues behind a background
// warm/backfill. Providers without it fall back to the shared FetchBodies.
type foregroundFetcher interface {
	FetchBodiesForeground(ctx context.Context, mb provider.MailboxRef, ids []model.MessageID) ([]model.RawMessage, error)
}

// fetchBodies fetches raw messages, routing foreground opens to the provider's
// dedicated connection when it has one.
func fetchBodies(ctx context.Context, p provider.Provider, mb provider.MailboxRef, ids []model.MessageID, fg bool) ([]model.RawMessage, error) {
	if fg {
		if ff, ok := p.(foregroundFetcher); ok {
			return ff.FetchBodiesForeground(ctx, mb, ids)
		}
	}
	return p.FetchBodies(ctx, mb, ids)
}

// LoadBody returns a message's decoded parts (inline + attachments), fetching
// and persisting them on first access. Subsequent calls are served from the
// local store with no network. After the first load the message's body text is
// also searchable via the FTS index. Use LoadBodyForeground for interactive
// opens so the fetch runs on the provider's dedicated connection.
func (e *Engine) LoadBody(ctx context.Context, p provider.Provider, account model.AccountID, id model.MessageID) ([]model.Part, error) {
	return e.loadBody(ctx, p, account, id, false)
}

// LoadBodyForeground is LoadBody for interactive opens: it routes the fetch to
// the provider's foreground connection so it never queues behind a bulk warm.
func (e *Engine) LoadBodyForeground(ctx context.Context, p provider.Provider, account model.AccountID, id model.MessageID) ([]model.Part, error) {
	return e.loadBody(ctx, p, account, id, true)
}

func (e *Engine) loadBody(ctx context.Context, p provider.Provider, account model.AccountID, id model.MessageID, fg bool) ([]model.Part, error) {
	if loaded, err := e.store.IsBodyLoaded(ctx, account, id); err == nil && loaded {
		return e.store.Parts(ctx, account, id)
	}
	// Coalesce with any concurrent load of this message: if another caller owns
	// it, wait for them and serve from the store rather than re-fetching.
	owned, release, waits := e.bodies.claim(account, []model.MessageID{id})
	defer release()
	if len(owned) == 0 {
		for _, w := range waits {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-w:
			}
		}
		return e.store.Parts(ctx, account, id)
	}
	raws, err := fetchBodies(ctx, p, e.messageMailbox(ctx, account, id), []model.MessageID{id}, fg)
	if err != nil {
		return nil, err
	}
	if len(raws) == 0 {
		return nil, store.ErrNotFound
	}
	parsed, err := mime.Parse(raws[0].Bytes)
	if err != nil {
		return nil, err
	}
	msg, err := e.store.Message(ctx, account, id)
	if err != nil {
		slog.Debug("body: message lookup failed; classifying on zero message", "account", account, "id", id, "err", err)
	}
	if err := e.store.SaveBody(ctx, account, id, parsed.Parts, parsed.Text, classify.MessageWithHeadersAndBody(msg, parsed.Headers, parsed.Text)); err != nil {
		return nil, err
	}
	return parsed.Parts, nil
}

// WarmBodies fetches and caches bodies for the newest `limit` messages in a
// mailbox, skipping any already loaded. It is used at launch to prioritize the
// first screens the user will actually open, ahead of full history backfill.
// Fetching runs in warmChunk-sized rounds so the shared connection is released
// between rounds rather than held for one large fetch.
func (e *Engine) WarmBodies(ctx context.Context, p provider.Provider, account model.AccountID, mailbox model.LabelID, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	msgs, err := e.store.MailboxMessages(ctx, account, mailbox, limit)
	if err != nil {
		return 0, err
	}
	ids := make([]model.MessageID, 0, len(msgs))
	for _, m := range msgs {
		if m.BodyLoaded {
			continue
		}
		ids = append(ids, m.ID)
	}

	total := 0
	var errs []error
	for start := 0; start < len(ids); start += warmChunk {
		if err := ctx.Err(); err != nil {
			errs = append(errs, err)
			break
		}
		end := min(start+warmChunk, len(ids))
		n, err := e.LoadBodies(ctx, p, account, ids[start:end])
		total += n
		if err != nil {
			errs = append(errs, err)
		}
	}
	return total, errors.Join(errs...)
}

// LoadBodies fetches and persists bodies for a batch of messages, skipping
// messages already cached locally. It returns the number newly loaded. A
// per-message parse/store failure does not stop the whole batch, but is joined
// into the returned error so callers can log it. Use LoadBodiesForeground for
// interactive thread opens.
func (e *Engine) LoadBodies(ctx context.Context, p provider.Provider, account model.AccountID, ids []model.MessageID) (int, error) {
	return e.loadBodies(ctx, p, account, ids, false)
}

// LoadBodiesForeground is LoadBodies for interactive thread opens: it routes
// fetches to the provider's foreground connection so the open never queues
// behind a bulk warm or backfill.
func (e *Engine) LoadBodiesForeground(ctx context.Context, p provider.Provider, account model.AccountID, ids []model.MessageID) (int, error) {
	return e.loadBodies(ctx, p, account, ids, true)
}

func (e *Engine) loadBodies(ctx context.Context, p provider.Provider, account model.AccountID, ids []model.MessageID, fg bool) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}

	seen := make(map[model.MessageID]bool, len(ids))
	unloaded := make([]model.MessageID, 0, len(ids))
	for _, id := range ids {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		loaded, err := e.store.IsBodyLoaded(ctx, account, id)
		if err != nil || loaded {
			continue
		}
		unloaded = append(unloaded, id)
	}
	if len(unloaded) == 0 {
		return 0, nil
	}

	// Coalesce with any concurrent load of the same message (e.g. a launch warm
	// fetching a body the user just opened): fetch only the ids we own, then wait
	// for the rest so they're in the store before returning for callers that
	// re-read it (such as a thread open).
	owned, release, waits := e.bodies.claim(account, unloaded)
	defer release()

	loaded := 0
	var errs []error
	if len(owned) > 0 {
		// Group by the mailbox each message lives in: IMAP body fetches are scoped
		// to the selected folder, so a thread/batch spanning folders needs one
		// fetch per folder rather than a single (INBOX-only) call.
		byMailbox := map[provider.MailboxRef][]model.MessageID{}
		for _, id := range owned {
			mb := e.messageMailbox(ctx, account, id)
			byMailbox[mb] = append(byMailbox[mb], id)
		}
		for mb, group := range byMailbox {
			raws, err := fetchBodies(ctx, p, mb, group, fg)
			if err != nil {
				errs = append(errs, err)
				continue
			}
			for _, raw := range raws {
				if raw.ID == "" {
					continue
				}
				parsed, err := mime.Parse(raw.Bytes)
				if err != nil {
					errs = append(errs, err)
					continue
				}
				msg, err := e.store.Message(ctx, account, raw.ID)
				if err != nil {
					slog.Debug("body: message lookup failed; classifying on zero message", "account", account, "id", raw.ID, "err", err)
				}
				if err := e.store.SaveBody(ctx, account, raw.ID, parsed.Parts, parsed.Text, classify.MessageWithHeadersAndBody(msg, parsed.Headers, parsed.Text)); err != nil {
					errs = append(errs, err)
					continue
				}
				loaded++
			}
		}
	}

	for _, w := range waits {
		select {
		case <-ctx.Done():
			return loaded, errors.Join(append(errs, ctx.Err())...)
		case <-w:
		}
	}
	return loaded, errors.Join(errs...)
}

// messageMailbox resolves the folder a stored message lives in, for body fetches.
// It prefers INBOX when present (the common case, and what fetching assumed
// before), otherwise the message's first label; defaults to INBOX when unknown.
func (e *Engine) messageMailbox(ctx context.Context, account model.AccountID, id model.MessageID) provider.MailboxRef {
	inbox := provider.MailboxRef{ID: "INBOX", Path: "INBOX"}
	msg, err := e.store.Message(ctx, account, id)
	if err != nil || len(msg.Labels) == 0 {
		return inbox
	}
	if slices.Contains(msg.Labels, "INBOX") {
		return inbox
	}
	first := msg.Labels[0]
	return provider.MailboxRef{ID: first, Path: string(first)}
}

// inflightBodies coalesces concurrent body loads for the same message so a
// foreground open and a background warm don't both fetch it. Each in-flight load
// owns a channel that closes when the load finishes (success or failure);
// late-arriving callers for the same id wait on it and then read from the store.
type inflightBodies struct {
	mu sync.Mutex
	m  map[string]chan struct{}
}

func newInflightBodies() *inflightBodies {
	return &inflightBodies{m: map[string]chan struct{}{}}
}

func bodyKey(account model.AccountID, id model.MessageID) string {
	return string(account) + "\x00" + string(id)
}

// claim takes ownership of every id not already being loaded. It returns the ids
// this caller now owns (and must load before calling release), a release that
// completes those loads and wakes any waiters, and the channels of ids owned by
// other in-flight callers (each closed when that load finishes).
func (f *inflightBodies) claim(account model.AccountID, ids []model.MessageID) (owned []model.MessageID, release func(), waits []<-chan struct{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ownedKeys := make([]string, 0, len(ids))
	ownedChans := make([]chan struct{}, 0, len(ids))
	for _, id := range ids {
		key := bodyKey(account, id)
		if ch, ok := f.m[key]; ok {
			waits = append(waits, ch)
			continue
		}
		ch := make(chan struct{})
		f.m[key] = ch
		owned = append(owned, id)
		ownedKeys = append(ownedKeys, key)
		ownedChans = append(ownedChans, ch)
	}
	release = func() {
		f.mu.Lock()
		for i, key := range ownedKeys {
			delete(f.m, key)
			close(ownedChans[i])
		}
		f.mu.Unlock()
	}
	return owned, release, waits
}
