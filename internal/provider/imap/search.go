package imap

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

// Search runs a server-side UID SEARCH over the mailbox and returns matching
// envelopes newest-first. This reaches mail that hasn't been synced/indexed
// locally — the rest of the mailbox history — at the cost of a round-trip and
// the server's (often header/substring, not ranked) SEARCH semantics.
func (p *Provider) Search(ctx context.Context, mb provider.MailboxRef, query string, limit int) ([]model.Message, error) {
	crit := buildSearchCriteria(query)
	if crit == nil {
		return nil, nil
	}

	p.opMu.Lock()
	defer p.opMu.Unlock()

	c, err := p.conn(ctx)
	if err != nil {
		return nil, err
	}
	if p.selected != mb.Path {
		if _, err := c.Select(mb.Path, nil).Wait(); err != nil {
			p.reset()
			return nil, fmt.Errorf("imap select %q: %w", mb.Path, err)
		}
		p.selected = mb.Path
	}

	data, err := c.UIDSearch(crit, nil).Wait()
	if err != nil {
		p.reset()
		return nil, fmt.Errorf("imap search: %w", err)
	}
	uids := data.AllUIDs()
	if len(uids) == 0 {
		return nil, nil
	}
	// UIDs are ascending ≈ oldest→newest; keep the newest `limit` for the fetch.
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	if limit > 0 && len(uids) > limit {
		uids = uids[len(uids)-limit:]
	}
	var set imap.UIDSet
	for _, uid := range uids {
		set.AddNum(uid)
	}

	msgs, err := p.fetchEnvelopes(ctx, c, mb.Path, set, 0)
	if err != nil {
		return nil, err
	}
	out := make([]model.Message, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, toMessage(p.cfg.Account, mb, m))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.After(out[j].Date) })
	return out, nil
}

// buildSearchCriteria maps the app's query mini-syntax to IMAP SEARCH criteria.
// Recognized operators: from:/to:/subject:, is:unread|read|starred,
// after:/before: (YYYY-MM-DD); everything else becomes a free-text TEXT term
// (matched against headers + body, ANDed). Returns nil for an empty query.
func buildSearchCriteria(query string) *imap.SearchCriteria {
	crit := &imap.SearchCriteria{}
	has := false
	for _, tok := range strings.Fields(query) {
		low := strings.ToLower(tok)
		switch {
		case strings.HasPrefix(low, "from:"):
			if v := tok[len("from:"):]; v != "" {
				crit.Header = append(crit.Header, imap.SearchCriteriaHeaderField{Key: "From", Value: v})
				has = true
			}
		case strings.HasPrefix(low, "to:"):
			if v := tok[len("to:"):]; v != "" {
				crit.Header = append(crit.Header, imap.SearchCriteriaHeaderField{Key: "To", Value: v})
				has = true
			}
		case strings.HasPrefix(low, "subject:"):
			if v := tok[len("subject:"):]; v != "" {
				crit.Header = append(crit.Header, imap.SearchCriteriaHeaderField{Key: "Subject", Value: v})
				has = true
			}
		case low == "is:unread":
			crit.NotFlag = append(crit.NotFlag, imap.FlagSeen)
			has = true
		case low == "is:read":
			crit.Flag = append(crit.Flag, imap.FlagSeen)
			has = true
		case low == "is:starred", low == "is:flagged":
			crit.Flag = append(crit.Flag, imap.FlagFlagged)
			has = true
		case strings.HasPrefix(low, "after:"):
			if t, ok := parseSearchDate(tok[len("after:"):]); ok {
				crit.Since = t
				has = true
			}
		case strings.HasPrefix(low, "before:"):
			if t, ok := parseSearchDate(tok[len("before:"):]); ok {
				crit.Before = t
				has = true
			}
		case low == "has:attachment":
			// Not expressible in base IMAP SEARCH; ignored.
		default:
			crit.Text = append(crit.Text, tok)
			has = true
		}
	}
	if !has {
		return nil
	}
	return crit
}

func parseSearchDate(s string) (time.Time, bool) {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
