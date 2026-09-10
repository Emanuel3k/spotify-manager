package out

import (
	"context"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

// ProfileGateway is the driven port for reading account data from the
// Spotify Web API (as opposed to spotifyauth.Gateway, which only talks to
// the Accounts service for tokens).
type ProfileGateway interface {
	// CurrentUser fetches the profile of the account owning accessToken.
	CurrentUser(ctx context.Context, accessToken string) (domain.User, error)
}
