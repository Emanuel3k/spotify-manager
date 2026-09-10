// Package in holds the driving ports: the use-case interfaces that outer
// adapters (CLI today, maybe a TUI or scheduled job later) call into. They
// describe what the application can do, never how.
package in

import (
	"context"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

// Status describes the current session, for `auth status`.
type Status struct {
	Authenticated bool
	Scope         string
	ExpiresAt     string // RFC3339, empty when not authenticated
}

// AuthService is the driving port for authentication/authorization: logging
// in through the browser, logging out, checking status, and handing other
// use cases (future features) a guaranteed-valid access token.
type AuthService interface {
	// Login runs the full Authorization Code flow: it builds the
	// authorization URL, hands it to onAuthURL (so the caller can print it
	// and/or attempt to open a browser) *before* blocking on the redirect,
	// then waits for the callback, exchanges the code, and persists the
	// token. onAuthURL may be nil.
	Login(ctx context.Context, onAuthURL func(authURL string)) (domain.Token, error)

	// Logout deletes any stored session. Idempotent.
	Logout(ctx context.Context) error

	// Status reports whether a session is stored and, if so, its metadata.
	Status(ctx context.Context) (Status, error)

	// ValidToken returns a valid, non-expired access token for use by other
	// features, transparently refreshing (and re-persisting) it if needed.
	ValidToken(ctx context.Context) (domain.Token, error)
}
