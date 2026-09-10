package out

import (
	"context"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

// TokenRepository is the driven port responsible for persisting the user's
// session between CLI invocations. The core does not care whether the
// implementation writes to a JSON file, the OS keychain, or anything else.
type TokenRepository interface {
	// Save persists the token, overwriting any previously stored one.
	Save(ctx context.Context, token domain.Token) error

	// Load returns the previously stored token. It returns
	// domain.ErrNotAuthenticated (wrapped) when nothing has been persisted.
	Load(ctx context.Context) (domain.Token, error)

	// Delete removes any stored token. It must not fail when nothing is
	// stored, so logout stays idempotent.
	Delete(ctx context.Context) error
}
