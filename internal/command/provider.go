package command

import (
	"context"
	"fmt"
	"os"

	"github.com/atterpac/email/internal/provider"
)

// newProvider builds a provider for account, selected by EMAIL_PROVIDER:
//
//	gmail (default) — native Gmail REST API
//	imap            — Gmail-over-IMAP via XOAUTH2
//
// This is the single place that maps an account to a concrete backend; the
// engine and store stay backend-agnostic.
func newProvider(ctx context.Context, account string) (provider.Provider, error) {
	switch os.Getenv("EMAIL_PROVIDER") {
	case "imap":
		return gmailProvider(ctx, account)
	case "", "gmail":
		return gmailRESTProvider(ctx, account)
	default:
		return nil, fmt.Errorf("unknown EMAIL_PROVIDER %q (want gmail|imap)", os.Getenv("EMAIL_PROVIDER"))
	}
}
