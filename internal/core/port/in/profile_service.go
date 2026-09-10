package in

import (
	"context"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

// ProfileService is the driving port for reading the logged-in user's own
// Spotify profile.
type ProfileService interface {
	// Me returns the authenticated account's profile, transparently
	// refreshing the access token via AuthService if needed.
	Me(ctx context.Context) (domain.User, error)
}
