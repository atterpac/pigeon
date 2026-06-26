package sync

import (
	"context"
	"errors"

	"github.com/atterpac/email/internal/classify"
	"github.com/atterpac/email/internal/mime"
	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
	"github.com/atterpac/email/internal/store"
)

// LoadBody returns a message's decoded parts (inline + attachments), fetching
// and persisting them on first access. Subsequent calls are served from the
// local store with no network. After the first load the message's body text is
// also searchable via the FTS index.
func (e *Engine) LoadBody(ctx context.Context, p provider.Provider, account model.AccountID, id model.MessageID) ([]model.Part, error) {
	if loaded, err := e.store.IsBodyLoaded(ctx, account, id); err == nil && loaded {
		return e.store.Parts(ctx, account, id)
	}
	raws, err := p.FetchBodies(ctx, []model.MessageID{id})
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
	msg, _ := e.store.Message(ctx, account, id)
	if err := e.store.SaveBody(ctx, account, id, parsed.Parts, parsed.Text, classify.MessageWithHeadersAndBody(msg, parsed.Headers, parsed.Text)); err != nil {
		return nil, err
	}
	return parsed.Parts, nil
}

// LoadBodies fetches and persists bodies for a batch of messages, skipping
// messages already cached locally. It returns the number newly loaded. A
// per-message parse/store failure does not stop the whole batch, but is joined
// into the returned error so callers can log it.
func (e *Engine) LoadBodies(ctx context.Context, p provider.Provider, account model.AccountID, ids []model.MessageID) (int, error) {
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

	raws, err := p.FetchBodies(ctx, unloaded)
	if err != nil {
		return 0, err
	}

	loaded := 0
	var errs []error
	for _, raw := range raws {
		if raw.ID == "" {
			continue
		}
		parsed, err := mime.Parse(raw.Bytes)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		msg, _ := e.store.Message(ctx, account, raw.ID)
		if err := e.store.SaveBody(ctx, account, raw.ID, parsed.Parts, parsed.Text, classify.MessageWithHeadersAndBody(msg, parsed.Headers, parsed.Text)); err != nil {
			errs = append(errs, err)
			continue
		}
		loaded++
	}
	return loaded, errors.Join(errs...)
}
