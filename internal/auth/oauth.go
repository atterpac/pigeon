package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// GmailScope grants Gmail API access AND IMAP/SMTP XOAUTH2 in one scope.
const GmailScope = "https://mail.google.com/"

// LoadGoogleConfig reads a Desktop-app client_secret.json and returns an
// oauth2.Config with a placeholder loopback redirect (the real port is chosen
// per-authorization in InteractiveAuth).
func LoadGoogleConfig(path string, scopes ...string) (*oauth2.Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read google credentials: %w", err)
	}
	if len(scopes) == 0 {
		scopes = []string{GmailScope}
	}
	cfg, err := google.ConfigFromJSON(b, scopes...)
	if err != nil {
		return nil, fmt.Errorf("parse google credentials: %w", err)
	}
	return cfg, nil
}

// InteractiveAuth runs the loopback authorization-code flow: it starts a local
// server on a random port, opens the consent URL (via openBrowser, which may be
// nil to just print it), and exchanges the returned code for a token. Returns a
// Credential carrying the refresh token.
func InteractiveAuth(ctx context.Context, cfg *oauth2.Config, openBrowser func(string) error) (Credential, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return Credential{}, fmt.Errorf("loopback listen: %w", err)
	}
	defer ln.Close()

	// Clone cfg with the concrete redirect URI for this attempt.
	local := *cfg
	local.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)

	state, err := randomState()
	if err != nil {
		return Credential{}, err
	}

	// PKCE: bind this authorization request to a one-time verifier so an
	// intercepted code can't be exchanged without it.
	verifier := oauth2.GenerateVerifier()

	type result struct {
		code string
		err  error
	}
	resCh := make(chan result, 1)
	var once sync.Once
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		send := func(res result) {
			once.Do(func() { resCh <- res })
		}
		if e := q.Get("error"); e != "" {
			http.Error(w, "authorization denied", http.StatusForbidden)
			send(result{err: fmt.Errorf("oauth: %s", e)})
			return
		}
		if q.Get("state") != state {
			http.Error(w, "state mismatch", http.StatusBadRequest)
			send(result{err: fmt.Errorf("oauth: state mismatch")})
			return
		}
		fmt.Fprintln(w, "Authorization complete — you can close this tab.")
		send(result{code: q.Get("code")})
	})}
	go srv.Serve(ln)
	defer srv.Close()

	// Offline access + consent prompt forces a refresh token to be returned.
	authURL := local.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.ApprovalForce, oauth2.S256ChallengeOption(verifier))
	if openBrowser != nil {
		_ = openBrowser(authURL)
	} else {
		fmt.Println("Open this URL to authorize:\n" + authURL)
	}

	var res result
	select {
	case <-ctx.Done():
		return Credential{}, ctx.Err()
	case res = <-resCh:
	}
	if res.err != nil {
		return Credential{}, res.err
	}

	tok, err := local.Exchange(ctx, res.code, oauth2.VerifierOption(verifier))
	if err != nil {
		return Credential{}, fmt.Errorf("token exchange: %w", err)
	}
	if tok.RefreshToken == "" {
		return Credential{}, fmt.Errorf("oauth: no refresh token returned (re-consent required)")
	}
	return Credential{}.FromToken(tok), nil
}

// TokenSource returns an oauth2.TokenSource that refreshes against cfg and
// persists rotated tokens back to store under account. Use its Token() to get a
// live access token for Gmail API or XOAUTH2.
func TokenSource(ctx context.Context, cfg *oauth2.Config, store CredentialStore, account string, c Credential) oauth2.TokenSource {
	base := cfg.TokenSource(ctx, c.Token())
	return &persisting{ctx: ctx, store: store, account: account, base: base, last: c}
}

type persisting struct {
	ctx     context.Context
	store   CredentialStore
	account string
	base    oauth2.TokenSource
	mu      sync.Mutex
	last    Credential
}

func (p *persisting) Token() (*oauth2.Token, error) {
	tok, err := p.base.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if tok.AccessToken != p.last.AccessToken {
		p.last = p.last.FromToken(tok)
		if err := p.store.Set(p.ctx, p.account, p.last); err != nil {
			return nil, fmt.Errorf("persist refreshed token: %w", err)
		}
	}
	return tok, nil
}

func randomState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}
