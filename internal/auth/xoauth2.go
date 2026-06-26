package auth

import (
	"context"
	"fmt"

	"github.com/emersion/go-sasl"
	"golang.org/x/oauth2"
)

// xoauth2Client implements the Gmail/Outlook "XOAUTH2" SASL mechanism, which
// go-sasl does not provide (it only ships OAUTHBEARER). The initial response is
//
//	base64("user=" <user> ^A "auth=Bearer " <token> ^A ^A)   where ^A = 0x01
type xoauth2Client struct {
	username string
	token    string
}

func (c *xoauth2Client) Start() (mech string, ir []byte, err error) {
	ir = fmt.Appendf(nil, "user=%s\x01auth=Bearer %s\x01\x01", c.username, c.token)
	return "XOAUTH2", ir, nil
}

// Next handles the server's empty error challenge: per the spec the client must
// reply with an empty response so the server can return the proper error.
func (c *xoauth2Client) Next(challenge []byte) ([]byte, error) {
	return []byte{}, nil
}

// XOAuth2Client builds a SASL XOAUTH2 client for IMAP/SMTP using a live access
// token from ts. Call it right before login so the token is fresh.
func XOAuth2Client(username string, ts oauth2.TokenSource) (sasl.Client, error) {
	tok, err := ts.Token()
	if err != nil {
		return nil, err
	}
	return &xoauth2Client{username: username, token: tok.AccessToken}, nil
}

// AccessToken returns a fresh bearer token string (for the Gmail REST client).
func AccessToken(_ context.Context, ts oauth2.TokenSource) (string, error) {
	tok, err := ts.Token()
	if err != nil {
		return "", err
	}
	return tok.AccessToken, nil
}
