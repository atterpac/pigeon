package imap

import (
	"bytes"
	"context"
	"fmt"
	"net/mail"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-smtp"

	"github.com/atterpac/email/internal/model"
	"github.com/atterpac/email/internal/provider"
)

// Send delivers a raw message over SMTP (STARTTLS) authenticated with the same
// SASL mechanism as IMAP. Gmail auto-files SMTP-sent mail into Sent.
func (p *Provider) Send(_ context.Context, raw model.RawMessage, _ provider.SendOpts) (model.MessageID, error) {
	from, rcpts, msgID, err := envelope(raw.Bytes)
	if err != nil {
		return "", err
	}

	host := p.cfg.SMTPHost
	if host == "" {
		host = p.cfg.Host
	}
	port := p.cfg.SMTPPort
	if port == 0 {
		port = 587
	}
	addr := fmt.Sprintf("%s:%d", host, port)

	c, err := smtp.DialStartTLS(addr, nil)
	if err != nil {
		return "", fmt.Errorf("smtp dial %s: %w", addr, err)
	}
	defer c.Close()

	saslClient, err := p.saslClient()
	if err != nil {
		return "", err
	}
	if err := c.Auth(saslClient); err != nil {
		return "", fmt.Errorf("smtp auth: %w", err)
	}
	if err := c.SendMail(from, rcpts, bytes.NewReader(raw.Bytes)); err != nil {
		return "", fmt.Errorf("smtp send: %w", err)
	}
	return model.MessageID(msgID), nil
}

// SaveDraft appends a draft to the Drafts folder via IMAP APPEND.
func (p *Provider) SaveDraft(ctx context.Context, raw model.RawMessage) (model.MessageID, error) {
	c, err := p.conn(ctx)
	if err != nil {
		return "", err
	}
	_, _, msgID, _ := envelope(raw.Bytes)
	opts := &imap.AppendOptions{Flags: []imap.Flag{imap.FlagDraft}}
	ac := c.Append("Drafts", int64(len(raw.Bytes)), opts)
	if _, err := ac.Write(raw.Bytes); err != nil {
		return "", fmt.Errorf("imap append draft: %w", err)
	}
	if err := ac.Close(); err != nil {
		return "", fmt.Errorf("imap append draft: %w", err)
	}
	if _, err := ac.Wait(); err != nil {
		return "", fmt.Errorf("imap append draft: %w", err)
	}
	return model.MessageID(msgID), nil
}

// envelope parses From / recipients / Message-ID from a raw RFC 5322 message.
func envelope(raw []byte) (from string, rcpts []string, msgID string, err error) {
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return "", nil, "", fmt.Errorf("parse outgoing message: %w", err)
	}
	h := mail.Header(m.Header)
	if fa, e := h.AddressList("From"); e == nil && len(fa) > 0 {
		from = fa[0].Address
	}
	for _, key := range []string{"To", "Cc", "Bcc"} {
		if al, e := h.AddressList(key); e == nil {
			for _, a := range al {
				rcpts = append(rcpts, a.Address)
			}
		}
	}
	msgID = h.Get("Message-ID")
	return from, rcpts, msgID, nil
}
