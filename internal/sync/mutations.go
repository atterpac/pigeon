package sync

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/atterpac/pigeon/internal/model"
	"github.com/atterpac/pigeon/internal/provider"
	"github.com/atterpac/pigeon/internal/store"
)

// OpMutate is the outbox op type for flag/label/move/delete mutations.
const OpMutate = "mutate"

type mutateKind string

const (
	mutFlags  mutateKind = "flags"
	mutLabels mutateKind = "labels"
	mutMove   mutateKind = "move"
	mutDelete mutateKind = "delete"
)

// mutatePayload is the JSON stored in op_log for a mutation.
type mutatePayload struct {
	Kind         mutateKind        `json:"kind"`
	IDs          []model.MessageID `json:"ids"`
	AddFlags     []model.Flag      `json:"add_flags,omitempty"`
	RemoveFlags  []model.Flag      `json:"remove_flags,omitempty"`
	AddLabels    []model.LabelID   `json:"add_labels,omitempty"`
	RemoveLabels []model.LabelID   `json:"remove_labels,omitempty"`
	Dst          model.LabelID     `json:"dst,omitempty"`
}

// SetFlags applies a flag change optimistically to the local store and queues
// the provider mutation. Use add/remove to toggle (e.g. add FlagSeen to mark
// read, remove it to mark unread).
func (e *Engine) SetFlags(ctx context.Context, acct model.AccountID, ids []model.MessageID, add, remove []model.Flag) error {
	deltas := make([]store.FlagDelta, len(ids))
	for i, id := range ids {
		deltas[i] = store.FlagDelta{ID: id, AddFlags: add, RemoveFlags: remove}
	}
	if err := e.store.ApplyFlagDeltas(ctx, acct, deltas); err != nil {
		return err
	}
	return e.enqueueMutate(ctx, acct, mutatePayload{Kind: mutFlags, IDs: ids, AddFlags: add, RemoveFlags: remove})
}

// ApplyLabels adds/removes labels optimistically and queues the mutation.
func (e *Engine) ApplyLabels(ctx context.Context, acct model.AccountID, ids []model.MessageID, add, remove []model.LabelID) error {
	deltas := make([]store.FlagDelta, len(ids))
	for i, id := range ids {
		deltas[i] = store.FlagDelta{ID: id, AddLabels: add, RemoveLabels: remove}
	}
	if err := e.store.ApplyFlagDeltas(ctx, acct, deltas); err != nil {
		return err
	}
	return e.enqueueMutate(ctx, acct, mutatePayload{Kind: mutLabels, IDs: ids, AddLabels: add, RemoveLabels: remove})
}

// Move relocates messages to dst (provider semantics) and queues the mutation.
// Local label state is updated to reflect dst.
func (e *Engine) Move(ctx context.Context, acct model.AccountID, ids []model.MessageID, dst model.LabelID) error {
	deltas := make([]store.FlagDelta, len(ids))
	for i, id := range ids {
		d := store.FlagDelta{ID: id, AddLabels: []model.LabelID{dst}}
		if dst != "INBOX" {
			d.RemoveLabels = []model.LabelID{"INBOX"}
		}
		deltas[i] = d
	}
	if err := e.store.ApplyFlagDeltas(ctx, acct, deltas); err != nil {
		return err
	}
	return e.enqueueMutate(ctx, acct, mutatePayload{Kind: mutMove, IDs: ids, Dst: dst})
}

// Delete removes messages locally (optimistic) and queues the provider delete
// (which moves them to Trash).
func (e *Engine) Delete(ctx context.Context, acct model.AccountID, ids []model.MessageID) error {
	if err := e.store.DeleteMessages(ctx, acct, ids); err != nil {
		return err
	}
	return e.enqueueMutate(ctx, acct, mutatePayload{Kind: mutDelete, IDs: ids})
}

func (e *Engine) enqueueMutate(ctx context.Context, acct model.AccountID, pl mutatePayload) error {
	b, err := json.Marshal(pl)
	if err != nil {
		return err
	}
	_, err = e.store.EnqueueOp(ctx, acct, OpMutate, b, time.Now())
	return err
}

// applyMutate dispatches a queued mutation to the provider.
func (e *Engine) applyMutate(ctx context.Context, p provider.Provider, pl mutatePayload) error {
	slog.Debug("applyMutate", "kind", pl.Kind, "ids", len(pl.IDs), "dst", pl.Dst,
		"addLabels", pl.AddLabels, "removeLabels", pl.RemoveLabels)
	var err error
	switch pl.Kind {
	case mutFlags:
		err = p.ApplyFlags(ctx, pl.IDs, pl.AddFlags, pl.RemoveFlags)
	case mutLabels:
		err = p.ApplyLabels(ctx, pl.IDs, pl.AddLabels, pl.RemoveLabels)
	case mutMove:
		err = p.Move(ctx, pl.IDs, provider.MailboxRef{ID: pl.Dst, Path: string(pl.Dst)})
	case mutDelete:
		err = p.Delete(ctx, pl.IDs)
	default:
		return nil // unknown kind: drop
	}
	if err != nil {
		slog.Error("applyMutate: provider mutation failed", "kind", pl.Kind, "dst", pl.Dst, "err", err)
	}
	return err
}
