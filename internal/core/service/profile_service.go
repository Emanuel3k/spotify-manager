package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/in"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// ProfileService implements port/in.ProfileService on top of AuthService
// (for a guaranteed-valid token) and a ProfileGateway (for the actual Web
// API call).
type ProfileService struct {
	auth    in.AuthService
	gateway out.ProfileGateway
	log     *slog.Logger
}

var _ in.ProfileService = (*ProfileService)(nil)

// NewProfileService wires the use case to the auth use case and the
// profile driven port. logger may be nil, in which case log output is
// discarded.
func NewProfileService(auth in.AuthService, gateway out.ProfileGateway, logger *slog.Logger) *ProfileService {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &ProfileService{auth: auth, gateway: gateway, log: logger.With("component", "profile_service")}
}

func (s *ProfileService) Me(ctx context.Context) (domain.User, error) {
	token, err := s.auth.ValidToken(ctx)
	if err != nil {
		s.log.Error("me: could not obtain a valid token", "error", err)
		return domain.User{}, err
	}

	user, err := s.gateway.CurrentUser(ctx, token.AccessToken)
	if err != nil {
		s.log.Error("me: fetch current user failed", "error", err)
		return domain.User{}, fmt.Errorf("fetch current user: %w", err)
	}

	s.log.Info("me: fetched current user", "user_id", user.ID)
	return user, nil
}
