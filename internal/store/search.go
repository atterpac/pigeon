package store

import (
	"context"
	"strings"
	"time"

	"github.com/atterpac/email/internal/model"
)

// searchQuery is a parsed search expression: free-text FTS terms plus filters.
type searchQuery struct {
	terms     []string // free text → FTS MATCH
	from      []string // from: substrings
	to        []string // to: substrings
	subject   []string // subject: substrings
	labels    []string // label:
	hasUnread *bool    // is:unread / is:read
	hasStar   *bool    // is:starred
	hasAttach *bool    // has:attachment
	after     *int64   // after:YYYY-MM-DD (unix)
	before    *int64   // before:YYYY-MM-DD (unix)
}

// parseSearch turns a query string with operators into a searchQuery.
//
// Supported operators: from:, to:, subject:, label:, is:unread, is:read,
// is:starred, has:attachment, after:YYYY-MM-DD, before:YYYY-MM-DD. Everything
// else is treated as full-text terms.
func parseSearch(q string) searchQuery {
	var sq searchQuery
	for _, tok := range strings.Fields(q) {
		key, val, ok := strings.Cut(tok, ":")
		if !ok {
			sq.terms = append(sq.terms, tok)
			continue
		}
		switch strings.ToLower(key) {
		case "from":
			sq.from = append(sq.from, val)
		case "to":
			sq.to = append(sq.to, val)
		case "subject":
			sq.subject = append(sq.subject, val)
		case "label":
			sq.labels = append(sq.labels, val)
		case "is":
			switch strings.ToLower(val) {
			case "unread":
				b := true
				sq.hasUnread = &b
			case "read":
				b := false
				sq.hasUnread = &b
			case "starred", "flagged":
				b := true
				sq.hasStar = &b
			}
		case "has":
			if strings.EqualFold(val, "attachment") || strings.EqualFold(val, "attachments") {
				b := true
				sq.hasAttach = &b
			}
		case "after", "since":
			if t, err := time.Parse("2006-01-02", val); err == nil {
				u := t.Unix()
				sq.after = &u
			}
		case "before", "until":
			if t, err := time.Parse("2006-01-02", val); err == nil {
				u := t.Unix()
				sq.before = &u
			}
		default:
			sq.terms = append(sq.terms, tok)
		}
	}
	return sq
}

// Search runs a structured local query (operators + full text), newest first.
func (s *Store) Search(ctx context.Context, account model.AccountID, query string, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	sq := parseSearch(query)

	var (
		joins  []string
		wheres = []string{"m.account = ?"}
		args   = []any{string(account)}
		like   = func(col, v string) {
			wheres = append(wheres, col+" LIKE ?")
			args = append(args, "%"+v+"%")
		}
	)

	// Free-text terms use the FTS index.
	if len(sq.terms) > 0 {
		joins = append(joins, "JOIN messages_fts f ON m.rowid = f.rowid")
		wheres = append(wheres, "messages_fts MATCH ?")
		args = append(args, strings.Join(sq.terms, " "))
	}
	for _, v := range sq.from {
		like("m.from_json", v)
	}
	for _, v := range sq.to {
		like("m.to_json", v)
	}
	for _, v := range sq.subject {
		like("m.subject", v)
	}
	for _, lbl := range sq.labels {
		wheres = append(wheres, "EXISTS (SELECT 1 FROM message_labels ml WHERE ml.account = m.account AND ml.message = m.id AND ml.label = ?)")
		args = append(args, lbl)
	}
	if sq.hasUnread != nil {
		if *sq.hasUnread {
			wheres = append(wheres, "m.flags NOT LIKE ?")
		} else {
			wheres = append(wheres, "m.flags LIKE ?")
		}
		args = append(args, "%\\Seen%")
	}
	if sq.hasStar != nil && *sq.hasStar {
		wheres = append(wheres, "m.flags LIKE ?")
		args = append(args, "%\\Flagged%")
	}
	if sq.hasAttach != nil && *sq.hasAttach {
		wheres = append(wheres, "m.has_attachments = 1")
	}
	if sq.after != nil {
		wheres = append(wheres, "m.date >= ?")
		args = append(args, *sq.after)
	}
	if sq.before != nil {
		wheres = append(wheres, "m.date < ?")
		args = append(args, *sq.before)
	}

	sqlText := "SELECT m.id, m.thread, m.account, m.subject, m.from_json, m.date, m.category, m.flags, m.has_attachments FROM messages m " +
		strings.Join(joins, " ") + " WHERE " + strings.Join(wheres, " AND ") +
		" ORDER BY m.date DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.Message
	for rows.Next() {
		var (
			id, thread, acct, subject, fromJSON, category, flags string
			date, hasAtt                                         int64
		)
		if err := rows.Scan(&id, &thread, &acct, &subject, &fromJSON, &date, &category, &flags, &hasAtt); err != nil {
			return nil, err
		}
		m := model.Message{
			ID: model.MessageID(id), Thread: model.ThreadID(thread), Account: model.AccountID(acct),
			Subject: subject, Date: time.Unix(date, 0), Category: model.Category(category), Flags: splitFlags(flags), HasAttachments: hasAtt != 0,
			From: decodeAddrs(fromJSON),
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
