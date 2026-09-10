// Package spotifyweb implements driven ports backed by the Spotify Web API
// (api.spotify.com), as opposed to package spotifyauth which only talks to
// the separate Accounts service used for the OAuth token dance.
package spotifyweb

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// ProfileGateway implements out.ProfileGateway via the Web API's /v1/me.
type ProfileGateway struct {
	client
}

var _ out.ProfileGateway = (*ProfileGateway)(nil)

// New builds a ProfileGateway. logger may be nil, in which case log output
// is discarded.
func New(logger *slog.Logger) *ProfileGateway {
	return &ProfileGateway{client: newClient(logger)}
}

// meResponse mirrors the subset of GET /v1/me's JSON body the CLI uses.
type meResponse struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Product     string `json:"product"`
}

func (g *ProfileGateway) CurrentUser(ctx context.Context, accessToken string) (domain.User, error) {
	var mr meResponse
	if err := g.do(ctx, "current_user", http.MethodGet, "/v1/me", accessToken, nil, &mr); err != nil {
		return domain.User{}, err
	}

	return domain.User{
		ID:          mr.ID,
		DisplayName: mr.DisplayName,
		Email:       mr.Email,
		Product:     mr.Product,
	}, nil
}
