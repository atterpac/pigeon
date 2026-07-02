package store

import (
	"context"
	"time"

	"github.com/atterpac/pigeon/internal/model"
	gen "github.com/atterpac/pigeon/internal/store/db"
)

// Op is a queued outbound operation awaiting delivery to the provider.
type Op struct {
	ID       int64
	Kind     string
	Payload  []byte
	Attempts int
}

// EnqueueOp appends an operation to the outbox, due at runAt, and returns its id.
func (s *Store) EnqueueOp(ctx context.Context, account model.AccountID, op string, payload []byte, runAt time.Time) (int64, error) {
	return s.q.EnqueueOp(ctx, gen.EnqueueOpParams{
		Account: string(account),
		Op:      op,
		Payload: string(payload),
		NextAt:  runAt.Unix(),
		Created: time.Now().Unix(),
	})
}

// CancelSend removes a still-queued send op (the undo-send window). It only
// matches op='send' for the account, so it can never cancel a mutation, and
// no-ops once the op has already been delivered and deleted. Returns whether a
// row was removed.
func (s *Store) CancelSend(ctx context.Context, account model.AccountID, id int64) (bool, error) {
	n, err := s.q.CancelSendOp(ctx, gen.CancelSendOpParams{Account: string(account), ID: id})
	return n > 0, err
}

// ReadyOps returns operations due at or before now, oldest first.
func (s *Store) ReadyOps(ctx context.Context, account model.AccountID, now time.Time, limit int) ([]Op, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	rows, err := s.q.ReadyOps(ctx, gen.ReadyOpsParams{
		Account: string(account), NextAt: now.Unix(), Limit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Op, len(rows))
	for i, r := range rows {
		out[i] = Op{ID: r.ID, Kind: r.Op, Payload: []byte(r.Payload), Attempts: int(r.Attempts)}
	}
	return out, nil
}

// BumpOp records a failed attempt and reschedules the op for nextAt.
func (s *Store) BumpOp(ctx context.Context, id int64, nextAt time.Time) error {
	return s.q.BumpOp(ctx, gen.BumpOpParams{NextAt: nextAt.Unix(), ID: id})
}

// DeleteOp removes a completed op from the outbox.
func (s *Store) DeleteOp(ctx context.Context, id int64) error {
	return s.q.DeleteOp(ctx, id)
}
