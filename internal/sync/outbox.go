package sync

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
	"github.com/atterpac/email/internal/store"
)

// OpSend is the outbox op type for outbound message delivery.
const OpSend = "send"

// sendPayload is the JSON stored in op_log for a send operation.
type sendPayload struct {
	Raw    []byte         `json:"raw"`
	Thread model.ThreadID `json:"thread,omitempty"`
}

// EnqueueSend queues a built RFC 5322 message for delivery. It returns once the
// message is durably in the outbox; DrainOutbox performs the actual send, so
// this works offline.
func (e *Engine) EnqueueSend(ctx context.Context, acct model.AccountID, raw model.RawMessage, opts provider.SendOpts) error {
	b, err := json.Marshal(sendPayload{Raw: raw.Bytes, Thread: opts.Thread})
	if err != nil {
		return err
	}
	return e.store.EnqueueOp(ctx, acct, OpSend, b, time.Now())
}

// maxAttempts caps outbox retries before an op is given up on (left in place
// with a far-future schedule for inspection).
const maxAttempts = 8

// DrainOutbox delivers ready outbox ops for an account. Failures are retried
// with exponential backoff; successes are removed. Returns the number sent.
func (e *Engine) DrainOutbox(ctx context.Context, p provider.Provider, acct model.AccountID) (int, error) {
	return e.drainOutboxAt(ctx, p, acct, time.Now())
}

// drainOutboxAt is DrainOutbox with an injectable clock for tests.
func (e *Engine) drainOutboxAt(ctx context.Context, p provider.Provider, acct model.AccountID, now time.Time) (int, error) {
	ops, err := e.store.ReadyOps(ctx, acct, now, 50)
	if err != nil {
		return 0, err
	}
	if len(ops) > 0 {
		slog.Debug("outbox: draining", "account", acct, "ready", len(ops))
	}
	sent := 0
	for _, op := range ops {
		opErr := e.runOp(ctx, p, op)
		if opErr == errDropOp {
			_ = e.store.DeleteOp(ctx, op.ID)
			continue
		}
		if opErr == nil {
			if err := e.store.DeleteOp(ctx, op.ID); err != nil {
				return sent, err
			}
			sent++
			continue
		}
		// Reschedule with backoff.
		next := now.Add(backoff(op.Attempts + 1))
		if op.Attempts+1 >= maxAttempts {
			next = now.Add(24 * time.Hour) // park it
		}
		if err := e.store.BumpOp(ctx, op.ID, next); err != nil {
			return sent, err
		}
	}
	return sent, nil
}

// errDropOp signals an unrecoverable op that should be removed, not retried.
var errDropOp = errors.New("drop op")

// runOp executes a single outbox op against the provider.
func (e *Engine) runOp(ctx context.Context, p provider.Provider, op store.Op) error {
	switch op.Op {
	case OpSend:
		var pl sendPayload
		if err := json.Unmarshal(op.Payload, &pl); err != nil {
			return errDropOp
		}
		_, err := p.Send(ctx, model.RawMessage{Bytes: pl.Raw}, provider.SendOpts{Thread: pl.Thread})
		return err
	case OpMutate:
		var pl mutatePayload
		if err := json.Unmarshal(op.Payload, &pl); err != nil {
			return errDropOp
		}
		return e.applyMutate(ctx, p, pl)
	default:
		return errDropOp
	}
}

// backoff returns an exponential delay for the given attempt count, capped.
func backoff(attempt int) time.Duration {
	d := time.Duration(1<<uint(min(attempt, 6))) * 15 * time.Second // 30s,1m,2m,...,~16m
	if d > 30*time.Minute {
		d = 30 * time.Minute
	}
	return d
}
