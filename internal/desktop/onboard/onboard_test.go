package onboard

import (
	"context"
	"errors"
	"testing"

	"github.com/atterpac/email/internal/auth"
	"github.com/atterpac/email/internal/email"
)

// fakeCreds is an in-memory auth.CredentialStore that records writes and
// deletes and can be made to fail on demand.
type fakeCreds struct {
	stored  map[string]auth.Credential
	deleted []string
	setErr  error
	delErr  error
}

func (f *fakeCreds) Get(ctx context.Context, account string) (auth.Credential, error) {
	return f.stored[account], nil
}

func (f *fakeCreds) Set(ctx context.Context, account string, c auth.Credential) error {
	if f.setErr != nil {
		return f.setErr
	}
	if f.stored == nil {
		f.stored = map[string]auth.Credential{}
	}
	f.stored[account] = c
	return nil
}

func (f *fakeCreds) Delete(ctx context.Context, account string) error {
	f.deleted = append(f.deleted, account)
	return f.delErr
}

type fakeHost struct{ creds *fakeCreds }

func (h fakeHost) Creds() auth.CredentialStore    { return h.creds }
func (h fakeHost) SyncOptions() email.SyncOptions { return email.SyncOptions{} }

// fakeClient records the accounts onboarding adds, forgets, and starts syncing.
type fakeClient struct {
	added     []email.Account
	forgotten []email.AccountID
	syncCalls int
	accounts  []email.Account
	addErr    error
	syncErr   error
	forgetErr error
}

func (c *fakeClient) AddAccount(ctx context.Context, acct email.Account) ([]email.Mailbox, error) {
	if c.addErr != nil {
		return nil, c.addErr
	}
	c.added = append(c.added, acct)
	return nil, nil
}

func (c *fakeClient) ForgetAccount(ctx context.Context, id email.AccountID) error {
	c.forgotten = append(c.forgotten, id)
	return c.forgetErr
}

func (c *fakeClient) Accounts(ctx context.Context) ([]email.Account, error) {
	return c.accounts, nil
}

func (c *fakeClient) StartSync(ctx context.Context, acct email.Account, mailboxes []email.LabelID, opts email.SyncOptions) error {
	c.syncCalls++
	return c.syncErr
}

func newTest(t *testing.T) (*Onboarding, *fakeCreds, *fakeClient) {
	t.Helper()
	creds := &fakeCreds{}
	client := &fakeClient{}
	return &Onboarding{app: fakeHost{creds: creds}, client: client}, creds, client
}

func TestAddAppPasswordAccount(t *testing.T) {
	o, creds, client := newTest(t)

	acct, err := o.AddAppPasswordAccount(context.Background(), "  User@Example.COM ", "  Jane Doe ", "abcd efgh ijkl mnop")
	if err != nil {
		t.Fatalf("AddAppPasswordAccount: %v", err)
	}

	if acct.Email != "user@example.com" {
		t.Errorf("Email = %q, want normalized lowercase", acct.Email)
	}
	if acct.ID != email.AccountID("user@example.com") {
		t.Errorf("ID = %q, want account id from email", acct.ID)
	}
	if acct.Name != "Jane Doe" {
		t.Errorf("Name = %q, want trimmed display name", acct.Name)
	}
	if acct.Kind != email.KindIMAP {
		t.Errorf("Kind = %v, want KindIMAP", acct.Kind)
	}
	// App-password spaces are stripped before storage.
	if got := creds.stored["user@example.com"].Password; got != "abcdefghijklmnop" {
		t.Errorf("stored password = %q, want spaces stripped", got)
	}
	if len(client.added) != 1 {
		t.Fatalf("AddAccount called %d times, want 1", len(client.added))
	}
	if client.syncCalls != 1 {
		t.Errorf("StartSync called %d times, want 1", client.syncCalls)
	}
}

func TestAddAppPasswordAccountValidation(t *testing.T) {
	tests := []struct {
		name, email, password string
	}{
		{"empty email", "   ", "pw"},
		{"empty password", "a@b.com", ""},
		{"password all spaces", "a@b.com", "   "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, creds, client := newTest(t)
			_, err := o.AddAppPasswordAccount(context.Background(), tc.email, "", tc.password)
			if err == nil {
				t.Fatal("want validation error, got nil")
			}
			if len(creds.stored) != 0 || len(client.added) != 0 {
				t.Error("no credential or account should be persisted on validation failure")
			}
		})
	}
}

func TestAddIMAPAccountDefaults(t *testing.T) {
	o, creds, client := newTest(t)

	// Custom-server passwords may contain spaces and must be preserved verbatim.
	acct, err := o.AddIMAPAccount(context.Background(), IMAPAccountRequest{
		Email:    "a@b.com",
		Password: "p a s s",
		IMAPHost: " mail.b.com ",
	})
	if err != nil {
		t.Fatalf("AddIMAPAccount: %v", err)
	}
	if acct.IMAPHost != "mail.b.com" {
		t.Errorf("IMAPHost = %q, want trimmed", acct.IMAPHost)
	}
	if acct.IMAPPort != 993 {
		t.Errorf("IMAPPort = %d, want default 993", acct.IMAPPort)
	}
	if acct.SMTPHost != "mail.b.com" {
		t.Errorf("SMTPHost = %q, want fallback to IMAP host", acct.SMTPHost)
	}
	if acct.SMTPPort != 587 {
		t.Errorf("SMTPPort = %d, want default 587", acct.SMTPPort)
	}
	if got := creds.stored["a@b.com"].Password; got != "p a s s" {
		t.Errorf("stored password = %q, want spaces preserved", got)
	}
	if len(client.added) != 1 {
		t.Errorf("AddAccount called %d times, want 1", len(client.added))
	}
}

func TestAddIMAPAccountExplicitPorts(t *testing.T) {
	o, _, _ := newTest(t)
	acct, err := o.AddIMAPAccount(context.Background(), IMAPAccountRequest{
		Email:    "a@b.com",
		Password: "pw",
		IMAPHost: "imap.b.com",
		IMAPPort: 143,
		SMTPHost: "smtp.b.com",
		SMTPPort: 2525,
	})
	if err != nil {
		t.Fatalf("AddIMAPAccount: %v", err)
	}
	if acct.IMAPPort != 143 || acct.SMTPHost != "smtp.b.com" || acct.SMTPPort != 2525 {
		t.Errorf("explicit endpoints not preserved: %+v", acct)
	}
}

func TestAddIMAPAccountValidation(t *testing.T) {
	tests := []struct {
		name string
		req  IMAPAccountRequest
	}{
		{"empty email", IMAPAccountRequest{IMAPHost: "h", Password: "p"}},
		{"empty imap host", IMAPAccountRequest{Email: "a@b.com", Password: "p"}},
		{"empty password", IMAPAccountRequest{Email: "a@b.com", IMAPHost: "h", Password: "   "}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o, creds, client := newTest(t)
			if _, err := o.AddIMAPAccount(context.Background(), tc.req); err == nil {
				t.Fatal("want validation error, got nil")
			}
			if len(creds.stored) != 0 || len(client.added) != 0 {
				t.Error("nothing should be persisted on validation failure")
			}
		})
	}
}

func TestRegisterRollsBackCredentialOnAddFailure(t *testing.T) {
	o, creds, client := newTest(t)
	client.addErr = errors.New("connect refused")

	_, err := o.AddAppPasswordAccount(context.Background(), "a@b.com", "", "pw")
	if err == nil {
		t.Fatal("want error when AddAccount fails")
	}
	if len(creds.deleted) != 1 || creds.deleted[0] != "a@b.com" {
		t.Errorf("deleted = %v, want credential rolled back for a@b.com", creds.deleted)
	}
	if client.syncCalls != 0 {
		t.Errorf("StartSync called %d times, want 0 after add failure", client.syncCalls)
	}
}

func TestStartSyncFailureIsNotFatal(t *testing.T) {
	o, _, client := newTest(t)
	client.syncErr = errors.New("sync busy")

	acct, err := o.AddAppPasswordAccount(context.Background(), "a@b.com", "", "pw")
	if err != nil {
		t.Fatalf("sync failure should not fail the add: %v", err)
	}
	if acct.Email != "a@b.com" {
		t.Errorf("Email = %q, want a@b.com", acct.Email)
	}
}

func TestRemoveAccountAttemptsBothAndJoinsErrors(t *testing.T) {
	o, creds, client := newTest(t)
	client.forgetErr = errors.New("forget failed")
	creds.delErr = errors.New("delete failed")

	err := o.RemoveAccount(context.Background(), "a@b.com")
	if err == nil {
		t.Fatal("want joined error")
	}
	if !errors.Is(err, client.forgetErr) || !errors.Is(err, creds.delErr) {
		t.Errorf("err = %v, want both forget and delete errors joined", err)
	}
	// The credential delete must run even though ForgetAccount failed first.
	if len(creds.deleted) != 1 || creds.deleted[0] != "a@b.com" {
		t.Errorf("deleted = %v, want delete attempted despite forget failure", creds.deleted)
	}
	if len(client.forgotten) != 1 {
		t.Errorf("ForgetAccount called %d times, want 1", len(client.forgotten))
	}
}

func TestListAccountsPassesThrough(t *testing.T) {
	o, _, client := newTest(t)
	client.accounts = []email.Account{{Email: "x@y.com"}}

	got, err := o.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	if len(got) != 1 || got[0].Email != "x@y.com" {
		t.Errorf("ListAccounts = %+v, want passthrough of client accounts", got)
	}
}
