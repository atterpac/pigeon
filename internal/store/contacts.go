package store

import (
	"context"
	"strings"
	"time"

	"github.com/atterpac/email/internal/model"
	gen "github.com/atterpac/email/internal/store/db"
)

// harvestContacts upserts the address-book entries for one message's envelope
// (From/To/Cc/Bcc) using the message date as the last-seen timestamp. It runs
// inside the SaveMessages transaction (q is tx-scoped) so harvesting is atomic
// with the message write. Addresses are keyed by their lowercased form; blank or
// malformed addresses are skipped.
func harvestContacts(ctx context.Context, q *gen.Queries, account model.AccountID, m model.Message) error {
	at := m.Date.Unix()
	seen := make(map[string]struct{})
	for _, group := range [][]model.Address{m.From, m.To, m.Cc, m.Bcc} {
		for _, a := range group {
			addr := strings.ToLower(strings.TrimSpace(a.Addr))
			if addr == "" || !strings.Contains(addr, "@") {
				continue
			}
			if _, dup := seen[addr]; dup {
				continue // count an address at most once per message
			}
			seen[addr] = struct{}{}
			if err := q.UpsertContact(ctx, gen.UpsertContactParams{
				Account:  string(account),
				Addr:     addr,
				Name:     strings.TrimSpace(a.Name),
				LastSeen: at,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// SearchContacts returns address-book entries matching query (a prefix/substring
// of the address or display name), ranked by frequency then recency. An empty
// query returns the most-used contacts.
func (s *Store) SearchContacts(ctx context.Context, account model.AccountID, query string, limit int) ([]model.Contact, error) {
	if limit <= 0 {
		limit = 10
	}
	pattern := "%" + likeEscape(strings.ToLower(strings.TrimSpace(query))) + "%"
	rows, err := s.q.SearchContacts(ctx, gen.SearchContactsParams{
		Account: string(account),
		Addr:    pattern,
		Name:    pattern,
		Limit:   int64(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.Contact, len(rows))
	for i, r := range rows {
		out[i] = model.Contact{
			Name:     r.Name,
			Addr:     r.Addr,
			LastSeen: time.Unix(r.LastSeen, 0),
			Freq:     int(r.Freq),
		}
	}
	return out, nil
}

// likeEscape neutralizes LIKE wildcards in user input. The queries use no ESCAPE
// clause, so the safest approach is to strip the wildcard metacharacters rather
// than escape them — recipient prefixes rarely contain '%' or '_' literally.
func likeEscape(s string) string {
	return strings.NewReplacer("%", "", "_", "").Replace(s)
}
