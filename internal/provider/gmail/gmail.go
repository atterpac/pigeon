// Package gmail implements provider.Provider over the Gmail REST API:
// label topology, message-list backfill, incremental sync via users.history,
// and RAW body fetch. Gmail's native threadId gives correct threading, and the
// All Mail label avoids the per-folder duplication IMAP suffers.
//
// Write operations and send are stubbed pending later milestones.
package gmail

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"golang.org/x/oauth2"
	"golang.org/x/sync/errgroup"
	gmailapi "google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

var errNotImplemented = errors.New("gmail: not implemented yet")

// metadataHeaders is the header set fetched for envelopes.
var metadataHeaders = []string{
	"Subject", "From", "To", "Cc", "Bcc", "Date", "Message-ID", "References", "In-Reply-To",
	"List-ID", "List-Unsubscribe", "List-Unsubscribe-Post", "Precedence", "Auto-Submitted",
}

// metaConcurrency bounds parallel per-message metadata fetches.
const metaConcurrency = 10

// Provider is a Gmail-API-backed provider.Provider.
type Provider struct {
	account model.AccountID
	svc     *gmailapi.Service
}

// New builds a Gmail provider authenticated with ts (a refreshing token source).
func New(ctx context.Context, account model.AccountID, ts oauth2.TokenSource) (*Provider, error) {
	svc, err := gmailapi.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("gmail service: %w", err)
	}
	return &Provider{account: account, svc: svc}, nil
}

func (p *Provider) ListMailboxes(ctx context.Context) ([]model.Mailbox, error) {
	resp, err := p.svc.Users.Labels.List("me").Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("gmail labels.list: %w", err)
	}
	out := make([]model.Mailbox, 0, len(resp.Labels))
	for _, l := range resp.Labels {
		mb := model.Mailbox{
			ID:      model.LabelID(l.Id),
			Account: p.account,
			Name:    l.Name,
			Role:    roleFromLabel(l.Id),
		}
		// Counts require a per-label Get; fetch for system labels only to keep
		// this cheap.
		if l.Type == "system" {
			if d, err := p.svc.Users.Labels.Get("me", l.Id).Context(ctx).Do(); err == nil {
				mb.Total = int(d.MessagesTotal)
				mb.Unread = int(d.MessagesUnread)
			}
		}
		out = append(out, mb)
	}
	return out, nil
}

// backfillCursor is the Gmail message-list page token.
type backfillCursor struct {
	Token string `json:"token"`
}

func (p *Provider) Backfill(ctx context.Context, mb provider.MailboxRef, page *provider.Cursor, limit int) (provider.Changes, *provider.Cursor, bool, error) {
	if limit <= 0 {
		limit = 100
	}
	call := p.svc.Users.Messages.List("me").
		MaxResults(int64(limit)).
		Context(ctx)
	// Empty / "ALL" id => no label filter == Gmail's All Mail (every message,
	// once), which avoids the per-folder duplication of label-scoped backfills.
	if mb.ID != "" && mb.ID != "ALL" {
		call = call.LabelIds(string(mb.ID))
	}
	if bc, ok := decode[backfillCursor](page); ok && bc.Token != "" {
		call = call.PageToken(bc.Token)
	}
	resp, err := call.Do()
	if err != nil {
		return provider.Changes{}, nil, false, fmt.Errorf("gmail messages.list: %w", err)
	}

	ids := make([]string, len(resp.Messages))
	for i, m := range resp.Messages {
		ids[i] = m.Id
	}
	msgs, err := p.fetchMetadata(ctx, ids)
	if err != nil {
		return provider.Changes{}, nil, false, err
	}

	done := resp.NextPageToken == ""
	if done {
		return provider.Changes{Upserted: msgs}, nil, true, nil
	}
	return provider.Changes{Upserted: msgs}, encode(backfillCursor{Token: resp.NextPageToken}), false, nil
}

// syncCursor is the Gmail history id high-water mark.
type syncCursor struct {
	HistoryID uint64 `json:"history_id"`
}

func (p *Provider) Sync(ctx context.Context, mb provider.MailboxRef, cur *provider.Cursor) (provider.Changes, *provider.Cursor, error) {
	sc, ok := decode[syncCursor](cur)
	if !ok {
		// First sync: establish the baseline historyId without backfilling.
		prof, err := p.svc.Users.GetProfile("me").Context(ctx).Do()
		if err != nil {
			return provider.Changes{}, nil, fmt.Errorf("gmail getProfile: %w", err)
		}
		return provider.Changes{}, encode(syncCursor{HistoryID: prof.HistoryId}), nil
	}

	addedIDs := map[string]struct{}{}
	deletedIDs := map[string]struct{}{}
	// Per-message accumulated label add/remove across the history window.
	labelAdds := map[string][]string{}
	labelRems := map[string][]string{}
	var latest uint64 = sc.HistoryID
	pageToken := ""
	for {
		call := p.svc.Users.History.List("me").
			StartHistoryId(sc.HistoryID).
			HistoryTypes("messageAdded", "messageDeleted", "labelAdded", "labelRemoved").
			Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		resp, err := call.Do()
		if err != nil {
			// 404 => historyId too old; caller should re-baseline (backfill has
			// it covered). Signal by resetting the cursor to current.
			if ae := (*googleapi.Error)(nil); errors.As(err, &ae) && ae.Code == http.StatusNotFound {
				prof, perr := p.svc.Users.GetProfile("me").Context(ctx).Do()
				if perr != nil {
					return provider.Changes{}, nil, perr
				}
				return provider.Changes{}, encode(syncCursor{HistoryID: prof.HistoryId}), nil
			}
			return provider.Changes{}, nil, fmt.Errorf("gmail history.list: %w", err)
		}
		for _, h := range resp.History {
			for _, a := range h.MessagesAdded {
				if a.Message != nil {
					addedIDs[a.Message.Id] = struct{}{}
				}
			}
			for _, d := range h.MessagesDeleted {
				if d.Message != nil {
					deletedIDs[d.Message.Id] = struct{}{}
				}
			}
			for _, la := range h.LabelsAdded {
				if la.Message != nil {
					labelAdds[la.Message.Id] = append(labelAdds[la.Message.Id], la.LabelIds...)
				}
			}
			for _, lr := range h.LabelsRemoved {
				if lr.Message != nil {
					labelRems[lr.Message.Id] = append(labelRems[lr.Message.Id], lr.LabelIds...)
				}
			}
		}
		if resp.HistoryId > latest {
			latest = resp.HistoryId
		}
		if resp.NextPageToken == "" {
			break
		}
		pageToken = resp.NextPageToken
	}

	ch := provider.Changes{}

	// Added messages → full envelope upserts.
	ids := make([]string, 0, len(addedIDs))
	for id := range addedIDs {
		ids = append(ids, id)
	}
	msgs, err := p.fetchMetadata(ctx, ids)
	if err != nil {
		return provider.Changes{}, nil, err
	}
	ch.Upserted = msgs

	// Label changes → flag/label deltas (skip messages also newly added, since
	// their upsert already carries the current label set).
	for id := range mergeKeys(labelAdds, labelRems) {
		if _, added := addedIDs[id]; added {
			continue
		}
		ch.Flagged = append(ch.Flagged, labelChange(id, labelAdds[id], labelRems[id]))
	}

	// Deletions.
	for id := range deletedIDs {
		ch.Removed = append(ch.Removed, messageID(id))
	}

	return ch, encode(syncCursor{HistoryID: latest}), nil
}

// labelChange converts Gmail label add/remove ids into a provider.FlagChange,
// translating UNREAD/STARRED into model flags.
func labelChange(id string, added, removed []string) provider.FlagChange {
	fc := provider.FlagChange{ID: messageID(id)}
	for _, l := range added {
		switch l {
		case "UNREAD":
			fc.RemoveFlags = append(fc.RemoveFlags, model.FlagSeen)
		case "STARRED":
			fc.AddFlags = append(fc.AddFlags, model.FlagFlagged)
		default:
			fc.AddLabels = append(fc.AddLabels, model.LabelID(l))
		}
	}
	for _, l := range removed {
		switch l {
		case "UNREAD":
			fc.AddFlags = append(fc.AddFlags, model.FlagSeen)
		case "STARRED":
			fc.RemoveFlags = append(fc.RemoveFlags, model.FlagFlagged)
		default:
			fc.RemoveLabels = append(fc.RemoveLabels, model.LabelID(l))
		}
	}
	return fc
}

func mergeKeys(a, b map[string][]string) map[string]struct{} {
	out := make(map[string]struct{}, len(a)+len(b))
	for k := range a {
		out[k] = struct{}{}
	}
	for k := range b {
		out[k] = struct{}{}
	}
	return out
}

func (p *Provider) FetchBodies(ctx context.Context, ids []model.MessageID) ([]model.RawMessage, error) {
	out := make([]model.RawMessage, 0, len(ids))
	for _, id := range ids {
		m, err := p.svc.Users.Messages.Get("me", gmailID(id)).Format("raw").Context(ctx).Do()
		if err != nil {
			return nil, fmt.Errorf("gmail messages.get raw: %w", err)
		}
		raw, err := decodeRaw(m.Raw)
		if err != nil {
			return nil, err
		}
		out = append(out, model.RawMessage{ID: id, Bytes: raw})
	}
	return out, nil
}

// fetchMetadata fetches envelope metadata for message ids with bounded concurrency.
func (p *Provider) fetchMetadata(ctx context.Context, ids []string) ([]model.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]model.Message, len(ids))
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(metaConcurrency)
	for i, id := range ids {
		i, id := i, id
		g.Go(func() error {
			m, err := p.svc.Users.Messages.Get("me", id).
				Format("metadata").
				MetadataHeaders(metadataHeaders...).
				Context(ctx).Do()
			if err != nil {
				return fmt.Errorf("gmail messages.get %s: %w", id, err)
			}
			out[i] = toMessage(p.account, m)
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	return out, nil
}

func (p *Provider) Capabilities() provider.Caps {
	return provider.Caps{History: true, ServerSearch: true}
}

func (p *Provider) Close() error { return nil }

// --- write/send path: reserved for later milestones ---

// ApplyFlags maps flags to Gmail labels and batch-modifies. Note the inversion:
// adding FlagSeen means *removing* the UNREAD label.
func (p *Provider) ApplyFlags(ctx context.Context, ids []model.MessageID, add, remove []model.Flag) error {
	var addL, remL []string
	for _, f := range add {
		switch f {
		case model.FlagSeen:
			remL = append(remL, "UNREAD")
		case model.FlagFlagged:
			addL = append(addL, "STARRED")
		}
	}
	for _, f := range remove {
		switch f {
		case model.FlagSeen:
			addL = append(addL, "UNREAD")
		case model.FlagFlagged:
			remL = append(remL, "STARRED")
		}
	}
	return p.batchModify(ctx, ids, addL, remL)
}

func (p *Provider) ApplyLabels(ctx context.Context, ids []model.MessageID, add, remove []model.LabelID) error {
	return p.batchModify(ctx, ids, labelStrings(add), labelStrings(remove))
}

// Move adds the destination label. Removing the source label (e.g. INBOX for
// archive) is the caller's job via ApplyLabels, since Gmail messages are
// multi-labeled rather than living in one folder.
func (p *Provider) Move(ctx context.Context, ids []model.MessageID, dst provider.MailboxRef) error {
	return p.batchModify(ctx, ids, []string{string(dst.ID)}, nil)
}

// Mailbox CRUD is not exposed on Gmail: "folders" are labels, so use
// ApplyLabels / the label API rather than CREATE/RENAME/DELETE.
func (p *Provider) CreateMailbox(context.Context, string) (model.Mailbox, error) {
	return model.Mailbox{}, fmt.Errorf("gmail: mailbox create unsupported (use labels)")
}

func (p *Provider) RenameMailbox(context.Context, provider.MailboxRef, string) (model.Mailbox, error) {
	return model.Mailbox{}, fmt.Errorf("gmail: mailbox rename unsupported (use labels)")
}

func (p *Provider) DeleteMailbox(context.Context, provider.MailboxRef) error {
	return fmt.Errorf("gmail: mailbox delete unsupported (use labels)")
}

// Delete moves messages to Trash (recoverable). Permanent deletion is not
// exposed here.
func (p *Provider) Delete(ctx context.Context, ids []model.MessageID) error {
	for _, id := range ids {
		if _, err := p.svc.Users.Messages.Trash("me", gmailID(id)).Context(ctx).Do(); err != nil {
			return fmt.Errorf("gmail messages.trash: %w", err)
		}
	}
	return nil
}

func (p *Provider) batchModify(ctx context.Context, ids []model.MessageID, add, remove []string) error {
	if len(ids) == 0 || (len(add) == 0 && len(remove) == 0) {
		return nil
	}
	gids := make([]string, len(ids))
	for i, id := range ids {
		gids[i] = gmailID(id)
	}
	req := &gmailapi.BatchModifyMessagesRequest{Ids: gids, AddLabelIds: add, RemoveLabelIds: remove}
	if err := p.svc.Users.Messages.BatchModify("me", req).Context(ctx).Do(); err != nil {
		return fmt.Errorf("gmail messages.batchModify: %w", err)
	}
	return nil
}

func labelStrings(ls []model.LabelID) []string {
	out := make([]string, len(ls))
	for i, l := range ls {
		out[i] = string(l)
	}
	return out
}
func (p *Provider) Send(ctx context.Context, raw model.RawMessage, opts provider.SendOpts) (model.MessageID, error) {
	msg := &gmailapi.Message{Raw: base64.URLEncoding.EncodeToString(raw.Bytes)}
	if opts.Thread != "" {
		msg.ThreadId = string(opts.Thread)
	}
	sent, err := p.svc.Users.Messages.Send("me", msg).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("gmail messages.send: %w", err)
	}
	return messageID(sent.Id), nil
}

func (p *Provider) SaveDraft(ctx context.Context, raw model.RawMessage) (model.MessageID, error) {
	d := &gmailapi.Draft{Message: &gmailapi.Message{Raw: base64.URLEncoding.EncodeToString(raw.Bytes)}}
	created, err := p.svc.Users.Drafts.Create("me", d).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("gmail drafts.create: %w", err)
	}
	if created.Message != nil {
		return messageID(created.Message.Id), nil
	}
	return model.MessageID("gmail-draft:" + created.Id), nil
}
func (p *Provider) Watch(context.Context) (<-chan provider.MailboxRef, error) {
	return nil, errNotImplemented
}

var _ provider.Provider = (*Provider)(nil)

// --- cursor helpers ---

func encode[T any](v T) *provider.Cursor {
	b, _ := json.Marshal(v)
	return &provider.Cursor{Bytes: b}
}

func decode[T any](c *provider.Cursor) (T, bool) {
	var v T
	if c == nil || len(c.Bytes) == 0 {
		return v, false
	}
	if err := json.Unmarshal(c.Bytes, &v); err != nil {
		return v, false
	}
	return v, true
}
