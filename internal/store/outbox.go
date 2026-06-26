package store

import (
	"context"
	"time"

	"github.com/atterpac/email/internal/model"
	gen "github.com/atterpac/email/internal/store/db"
)

// Op is a queued outbound operation awaiting delivery to the provider.
type Op struct {
	ID       int64
	Op       string
	Payload  []byte
	Attempts int
}

// EnqueueOp appends an operation to the outbox, due at runAt.
func (s *Store) EnqueueOp(ctx context.Context, account model.AccountID, op string, payload []byte, runAt time.Time) error {
	return s.q.EnqueueOp(ctx, gen.EnqueueOpParams{
		Account: string(account),
		Op:      op,
		Payload: string(payload),
		NextAt:  runAt.Unix(),
		Created: time.Now().Unix(),
	})
}

// ReadyOps returns operations due at or before now, oldest first.
func (s *Store) ReadyOps(ctx context.Context, account model.AccountID, now time.Time, limit int) ([]Op, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.q.ReadyOps(ctx, gen.ReadyOpsParams{
		Account: string(account), NextAt: now.Unix(), Limit: int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]Op, len(rows))
	for i, r := range rows {
		out[i] = Op{ID: r.ID, Op: r.Op, Payload: []byte(r.Payload), Attempts: int(r.Attempts)}
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
