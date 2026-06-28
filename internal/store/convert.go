package store

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/atterpac/email/internal/model"
	gen "github.com/atterpac/email/internal/store/db"
)

// toUpsertParams maps a domain message onto the generated upsert row, encoding
// address lists as JSON and flags/references as space-joined strings.
func toUpsertParams(m model.Message) gen.UpsertMessageParams {
	return gen.UpsertMessageParams{
		ID:             string(m.ID),
		Account:        string(m.Account),
		Thread:         string(m.Thread),
		Subject:        m.Subject,
		FromJson:       mustJSON(m.From),
		ToJson:         mustJSON(m.To),
		CcJson:         mustJSON(m.Cc),
		BccJson:        mustJSON(m.Bcc),
		Date:           m.Date.Unix(),
		Snippet:        m.Snippet,
		Category:       string(m.Category),
		Flags:          joinFlags(m.Flags),
		HasAttachments: boolToInt(m.HasAttachments),
		BodyLoaded:     boolToInt(m.BodyLoaded),
		RfcMessageID:   m.RFCMessageID,
		Refs:           strings.Join(m.References, " "),
	}
}

func mapMessages(rows []gen.Message) []model.Message {
	out := make([]model.Message, len(rows))
	for i, r := range rows {
		out[i] = rowToMessage(r)
	}
	return out
}

func rowToMessage(r gen.Message) model.Message {
	m := model.Message{
		ID: model.MessageID(r.ID), Thread: model.ThreadID(r.Thread), Account: model.AccountID(r.Account),
		Subject: r.Subject, Snippet: r.Snippet, Date: time.Unix(r.Date, 0),
		Category: model.Category(r.Category), Flags: splitFlags(r.Flags), HasAttachments: r.HasAttachments != 0, BodyLoaded: r.BodyLoaded != 0,
		BodyCachedAt: unixTime(r.BodyCachedAt), LastOpenedAt: unixTime(r.LastOpenedAt),
		RFCMessageID: r.RfcMessageID, References: strings.Fields(r.Refs),
	}
	// Address columns tolerate corruption: a bad blob yields empty recipients
	// rather than failing the read.
	_ = json.Unmarshal([]byte(r.FromJson), &m.From)
	_ = json.Unmarshal([]byte(r.ToJson), &m.To)
	_ = json.Unmarshal([]byte(r.CcJson), &m.Cc)
	_ = json.Unmarshal([]byte(r.BccJson), &m.Bcc)
	return m
}

// decodeAddrs parses a JSON address list, tolerating a corrupt blob as empty.
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

func unixTime(sec int64) time.Time {
	if sec == 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}

// mustJSON marshals v, falling back to an empty array on the (practically
// impossible) failure to encode an address list.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func joinFlags(fs []model.Flag) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = string(f)
	}
	return strings.Join(parts, " ")
}

func splitFlags(s string) []model.Flag {
	if s == "" {
		return nil
	}
	parts := strings.Fields(s)
	out := make([]model.Flag, len(parts))
	for i, p := range parts {
		out[i] = model.Flag(p)
	}
	return out
}

func flagSet(fs []model.Flag) map[model.Flag]struct{} {
	m := make(map[model.Flag]struct{}, len(fs))
	for _, f := range fs {
		m[f] = struct{}{}
	}
	return m
}

func flagSlice(m map[model.Flag]struct{}) []model.Flag {
	out := make([]model.Flag, 0, len(m))
	for f := range m {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}

func sender(m model.Message) string {
	if len(m.From) == 0 {
		return ""
	}
	return m.From[0].Name + " " + m.From[0].Addr
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// idArgs builds "?,?,…" placeholders and a query-arg list of the form
// [account, id…] for an `IN (…)` clause scoped to a single account.
func idArgs[T ~string](account model.AccountID, ids []T) (placeholders string, args []any) {
	ph := make([]string, len(ids))
	args = make([]any, 0, len(ids)+1)
	args = append(args, string(account))
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, string(id))
	}
	return strings.Join(ph, ","), args
}
