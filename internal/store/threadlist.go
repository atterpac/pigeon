package store

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/atterpac/email/internal/model"
	gen "github.com/atterpac/email/internal/store/db"
)

// ThreadListItems returns conversation-list rows for an account, newest first.
// Each item aggregates its thread's messages (count, participants, latest
// sender, snippet, attachment flag, label union) so a UI can render the inbox
// without a second round of queries.
func (s *Store) ThreadListItems(ctx context.Context, account model.AccountID, limit int) ([]model.ThreadListItem, error) {
	if limit <= 0 {
		limit = 50
	}
	threads, err := s.q.ListThreads(ctx, gen.ListThreadsParams{Account: string(account), Limit: int64(limit)})
	if err != nil {
		return nil, err
	}
	if len(threads) == 0 {
		return nil, nil
	}

	// Fetch all messages across these threads in one query.
	ids := make([]any, 0, len(threads)+1)
	ids = append(ids, string(account))
	ph := make([]string, len(threads))
	for i, t := range threads {
		ph[i] = "?"
		ids = append(ids, t.ID)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT m.id, m.thread, m.subject, m.from_json, m.date, m.snippet, m.category, m.flags, m.has_attachments
		FROM messages m
		WHERE m.account = ? AND m.thread IN (`+strings.Join(ph, ",")+`)
		  AND EXISTS (
		    SELECT 1 FROM message_labels ml
		    WHERE ml.account = m.account AND ml.message = m.id AND ml.label = 'INBOX'
		  )
		ORDER BY date ASC`, ids...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type aggRow struct {
		id       model.MessageID
		snippet  string
		category model.Category
		from     []model.Address
	}
	type agg struct {
		rows           []aggRow // date ASC
		seenSenders    map[string]bool
		participants   []model.Address
		hasAttachments bool
		unread         bool
	}
	byThread := map[string]*agg{}
	for rows.Next() {
		var (
			id, thread, subject, fromJSON, snippet, category, flags string
			date, hasAtt                                            int64
		)
		if err := rows.Scan(&id, &thread, &subject, &fromJSON, &date, &snippet, &category, &flags, &hasAtt); err != nil {
			return nil, err
		}
		a := byThread[thread]
		if a == nil {
			a = &agg{seenSenders: map[string]bool{}}
			byThread[thread] = a
		}
		from := decodeAddrs(fromJSON)
		a.rows = append(a.rows, aggRow{id: model.MessageID(id), snippet: snippet, category: model.Category(category), from: from})
		if !slices.Contains(splitFlags(flags), model.FlagSeen) {
			a.unread = true
		}
		if hasAtt != 0 {
			a.hasAttachments = true
		}
		if len(from) > 0 && !a.seenSenders[from[0].Addr] {
			a.seenSenders[from[0].Addr] = true
			a.participants = append(a.participants, from[0])
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Labels per thread (union), via one batched query over all message ids.
	var allIDs []model.Message
	for _, a := range byThread {
		for _, r := range a.rows {
			allIDs = append(allIDs, model.Message{ID: r.id})
		}
	}
	if err := s.loadLabels(ctx, account, allIDs); err != nil {
		return nil, err
	}
	labelByMsg := map[model.MessageID][]model.LabelID{}
	for _, m := range allIDs {
		labelByMsg[m.ID] = m.Labels
	}

	out := make([]model.ThreadListItem, 0, len(threads))
	for _, t := range threads {
		a := byThread[t.ID]
		if a == nil || len(a.rows) == 0 {
			continue
		}
		item := model.ThreadListItem{
			ID: model.ThreadID(t.ID), Account: account, Subject: t.Subject,
			Last: time.Unix(t.Last, 0), Unread: a.unread,
		}
		item.Count = len(a.rows)
		item.Participants = a.participants
		item.HasAttachments = a.hasAttachments
		newest := a.rows[len(a.rows)-1] // rows came back date ASC
		item.LatestSender = firstAddr(newest.from)
		item.Snippet = newest.snippet
		item.Category = newest.category
		labelSet := map[model.LabelID]bool{}
		for _, r := range a.rows {
			for _, l := range labelByMsg[r.id] {
				if !labelSet[l] {
					labelSet[l] = true
					item.Labels = append(item.Labels, l)
				}
			}
		}
		out = append(out, item)
	}
	return out, nil
}

func decodeAddrs(jsonStr string) []model.Address {
	var a []model.Address
	_ = json.Unmarshal([]byte(jsonStr), &a)
	return a
}

func firstAddr(a []model.Address) model.Address {
	if len(a) > 0 {
		return a[0]
	}
	return model.Address{}
}
