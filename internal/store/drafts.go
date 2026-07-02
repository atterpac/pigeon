package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/atterpac/pigeon/internal/model"
	gen "github.com/atterpac/pigeon/internal/store/db"
)

// SaveDraft upserts a compose draft. If id is empty a new one is generated and
// returned. Drafts are local (autosave); sending goes through the normal path.
func (s *Store) SaveDraft(ctx context.Context, account model.AccountID, id string, out model.Outgoing) (string, error) {
	if id == "" {
		id = newID()
	}
	payload, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	if err := s.q.UpsertDraft(ctx, gen.UpsertDraftParams{
		ID: id, Account: string(account), Payload: string(payload), Updated: time.Now().Unix(),
	}); err != nil {
		return "", err
	}
	return id, nil
}

// GetDraft returns a single draft.
func (s *Store) GetDraft(ctx context.Context, account model.AccountID, id string) (model.Draft, error) {
	r, err := s.q.GetDraft(ctx, gen.GetDraftParams{Account: string(account), ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return model.Draft{}, ErrNotFound
	}
	if err != nil {
		return model.Draft{}, err
	}
	return toDraft(r.ID, r.Account, r.Payload, r.Updated), nil
}

// ListDrafts returns an account's drafts, most recently updated first.
func (s *Store) ListDrafts(ctx context.Context, account model.AccountID) ([]model.Draft, error) {
	rows, err := s.q.ListDrafts(ctx, string(account))
	if err != nil {
		return nil, err
	}
	out := make([]model.Draft, len(rows))
	for i, r := range rows {
		out[i] = toDraft(r.ID, r.Account, r.Payload, r.Updated)
	}
	return out, nil
}

// DeleteDraft discards a draft.
func (s *Store) DeleteDraft(ctx context.Context, account model.AccountID, id string) error {
	return s.q.DeleteDraft(ctx, gen.DeleteDraftParams{Account: string(account), ID: id})
}

func toDraft(id, account, payload string, updated int64) model.Draft {
	d := model.Draft{ID: id, Account: model.AccountID(account), Updated: time.Unix(updated, 0)}
	_ = json.Unmarshal([]byte(payload), &d.Message)
	return d
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
