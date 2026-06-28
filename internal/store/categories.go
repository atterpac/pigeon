package store

import (
	"context"
	"strings"

	"github.com/atterpac/email/internal/classify"
	"github.com/atterpac/email/internal/events"
	"github.com/atterpac/email/internal/model"
)

// ReclassifyMailbox recalculates categories for recent messages in a mailbox.
// Loaded body parts are included as text hints; unloaded messages use envelope
// metadata only. It returns the number of rows whose category changed.
// defaultReclassifyLimit caps how many recent messages a reclassify pass scans.
const defaultReclassifyLimit = 100

func (s *Store) ReclassifyMailbox(ctx context.Context, account model.AccountID, mailbox model.LabelID, limit int) (int, error) {
	if limit <= 0 {
		limit = defaultReclassifyLimit
	}
	msgs, err := s.MailboxMessages(ctx, account, mailbox, limit)
	if err != nil {
		return 0, err
	}

	changed := 0
	for _, msg := range msgs {
		body := ""
		if msg.BodyLoaded {
			parts, err := s.Parts(ctx, account, msg.ID)
			if err == nil {
				body = categoryBodyText(parts)
			}
		}
		next := classify.MessageWithHeadersAndBody(msg, nil, body)
		if next == "" || next == msg.Category {
			continue
		}
		if _, err := s.db.ExecContext(ctx,
			`UPDATE messages SET category = ? WHERE account = ? AND id = ?`,
			string(next), string(account), string(msg.ID)); err != nil {
			return changed, err
		}
		changed++
	}
	if changed > 0 {
		s.publish(account, events.KindUpsert, msgIDs(msgs))
	}
	return changed, nil
}

func categoryBodyText(parts []model.Part) string {
	var b strings.Builder
	for _, part := range parts {
		if part.Disposition == "attachment" || len(part.Content) == 0 {
			continue
		}
		contentType := strings.ToLower(part.ContentType)
		if !strings.Contains(contentType, "text/plain") && !strings.Contains(contentType, "text/html") {
			continue
		}
		b.WriteByte(' ')
		b.Write(part.Content)
		if b.Len() > 4096 {
			break
		}
	}
	return b.String()
}
