package domain

import "errors"

// Sentinel errors shared by the core and its adapters. Adapters should wrap
// these with fmt.Errorf("...: %w", ErrX) so callers can use errors.Is.
var (
	// ErrNotAuthenticated is returned when an operation requires a stored
	// session but none exists yet (the user never logged in, or logged out).
	ErrNotAuthenticated = errors.New("not authenticated: run `spotify-manager auth login` first")

	// ErrAuthorizationDenied is returned when the user declines the consent
	// screen in the browser (Spotify redirects back with error=access_denied).
	ErrAuthorizationDenied = errors.New("authorization denied by user")

	// ErrStateMismatch is returned when the "state" parameter echoed back by
	// the OAuth callback does not match the one we generated, which signals
	// a potential CSRF attempt and must abort the flow.
	ErrStateMismatch = errors.New("oauth state mismatch, aborting login for safety")

	// ErrLoginTimeout is returned when the user does not complete the browser
	// authorization within the allotted time window.
	ErrLoginTimeout = errors.New("timed out waiting for spotify authorization")
)
