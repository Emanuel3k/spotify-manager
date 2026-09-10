package out

import "context"

// CallbackResult carries whatever Spotify appended to the redirect URI's
// query string after the user answered the consent screen.
type CallbackResult struct {
	Code             string // present on success
	State            string // must be compared against the state we sent
	Error            string // e.g. "access_denied", present on failure
	ErrorDescription string
}

// CallbackListener is the driven port that captures the OAuth redirect. The
// concrete adapter binds a local HTTP server to the CLI's redirect URI
// (e.g. http://127.0.0.1:8080/callback) and blocks until either a browser
// hits it or ctx is cancelled/times out.
type CallbackListener interface {
	Await(ctx context.Context) (CallbackResult, error)
}
