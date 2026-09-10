package out

import (
	"context"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

// SpotifyAuthGateway is the driven port through which the core talks to
// Spotify's Accounts service (accounts.spotify.com). Any HTTP details
// (endpoints, headers, encoding) live entirely in the adapter that
// implements this interface — the core only knows about these three verbs.
type SpotifyAuthGateway interface {
	// BuildAuthorizeURL returns the URL the user must open in a browser to
	// grant (or deny) access, encoding the given anti-CSRF state value and
	// the requested scopes.
	BuildAuthorizeURL(state string, scopes []string) string

	// ExchangeCode swaps an authorization code (obtained from the OAuth
	// redirect) for an access/refresh token pair.
	ExchangeCode(ctx context.Context, code string) (domain.Token, error)

	// RefreshToken swaps a refresh token for a new access token. Spotify does
	// not always return a new refresh token; implementations must preserve
	// the original one when the response omits it.
	RefreshToken(ctx context.Context, refreshToken string) (domain.Token, error)
}
