// Package service implements the core use cases (driving ports) on top of
// the driven ports. It is pure application logic: no HTTP, no filesystem,
// no CLI framework — those all live in internal/adapter.
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/in"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// AuthService implements port/in.AuthService by orchestrating the driven
// ports. It holds no infrastructure details itself, only interfaces.
type AuthService struct {
	gateway  out.SpotifyAuthGateway
	tokens   out.TokenRepository
	browser  out.BrowserOpener
	callback out.CallbackListener
	scopes   []string
	log      *slog.Logger
}

// compile-time check that AuthService satisfies the driving port.
var _ in.AuthService = (*AuthService)(nil)

// NewAuthService wires the use case to its driven ports. scopes is the set
// of Spotify permission scopes requested on every login. logger may be nil,
// in which case log output is discarded.
func NewAuthService(
	gateway out.SpotifyAuthGateway,
	tokens out.TokenRepository,
	browser out.BrowserOpener,
	callback out.CallbackListener,
	scopes []string,
	logger *slog.Logger,
) *AuthService {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &AuthService{
		gateway:  gateway,
		tokens:   tokens,
		browser:  browser,
		callback: callback,
		scopes:   scopes,
		log:      logger.With("component", "auth_service"),
	}
}

// Login runs the Authorization Code flow end to end.
func (s *AuthService) Login(ctx context.Context, onAuthURL func(authURL string)) (domain.Token, error) {
	s.log.Info("login: starting")

	state, err := randomState()
	if err != nil {
		s.log.Error("login: generate state failed", "error", err)
		return domain.Token{}, fmt.Errorf("generate oauth state: %w", err)
	}

	authURL := s.gateway.BuildAuthorizeURL(state, s.scopes)
	s.log.Debug("login: authorize url built", "scopes", s.scopes)

	if onAuthURL != nil {
		onAuthURL(authURL)
	}

	// Opening the browser is best-effort: if it fails (headless machine,
	// unsupported OS, ...) the user still has the URL, printed above, to
	// open manually. The callback listener works either way.
	if err := s.browser.Open(authURL); err != nil {
		s.log.Warn("login: could not auto-open browser, user must open the printed url manually", "error", err)
	}

	s.log.Info("login: waiting for oauth callback")
	result, err := s.callback.Await(ctx)
	if err != nil {
		s.log.Error("login: callback wait failed", "error", err)
		return domain.Token{}, fmt.Errorf("await oauth callback: %w", err)
	}

	if result.Error != "" {
		s.log.Warn("login: spotify returned an error on the callback", "error", result.Error, "description", result.ErrorDescription)
		if result.Error == "access_denied" {
			return domain.Token{}, domain.ErrAuthorizationDenied
		}
		return domain.Token{}, fmt.Errorf("spotify returned error %q: %s", result.Error, result.ErrorDescription)
	}

	if result.State != state {
		s.log.Error("login: oauth state mismatch", "want", state, "got", result.State)
		return domain.Token{}, domain.ErrStateMismatch
	}

	s.log.Info("login: callback received, exchanging code")
	token, err := s.gateway.ExchangeCode(ctx, result.Code)
	if err != nil {
		s.log.Error("login: code exchange failed", "error", err)
		return domain.Token{}, fmt.Errorf("exchange authorization code: %w", err)
	}

	if err := s.tokens.Save(ctx, token); err != nil {
		s.log.Error("login: persisting token failed", "error", err)
		return domain.Token{}, fmt.Errorf("persist token: %w", err)
	}

	s.log.Info("login: success", "scope", token.Scope, "expires_at", token.ExpiresAt)
	return token, nil
}

// Logout deletes any stored session.
func (s *AuthService) Logout(ctx context.Context) error {
	if err := s.tokens.Delete(ctx); err != nil {
		s.log.Error("logout: delete stored token failed", "error", err)
		return fmt.Errorf("delete stored token: %w", err)
	}
	s.log.Info("logout: success")
	return nil
}

// Status reports the current session state without mutating anything.
func (s *AuthService) Status(ctx context.Context) (in.Status, error) {
	token, err := s.tokens.Load(ctx)
	if err != nil {
		return in.Status{Authenticated: false}, nil
	}
	if token.IsZero() {
		return in.Status{Authenticated: false}, nil
	}

	return in.Status{
		Authenticated: !token.IsExpired() || token.RefreshToken != "",
		Scope:         token.Scope,
		ExpiresAt:     token.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}, nil
}

// ValidToken returns a token guaranteed usable right now, refreshing and
// re-persisting it transparently when it has expired.
func (s *AuthService) ValidToken(ctx context.Context) (domain.Token, error) {
	token, err := s.tokens.Load(ctx)
	if err != nil {
		return domain.Token{}, fmt.Errorf("%w", domain.ErrNotAuthenticated)
	}
	if token.IsZero() {
		return domain.Token{}, domain.ErrNotAuthenticated
	}

	if !token.IsExpired() {
		return token, nil
	}

	if token.RefreshToken == "" {
		return domain.Token{}, domain.ErrNotAuthenticated
	}

	s.log.Info("valid_token: access token expired, refreshing")
	refreshed, err := s.gateway.RefreshToken(ctx, token.RefreshToken)
	if err != nil {
		s.log.Error("valid_token: refresh failed", "error", err)
		return domain.Token{}, fmt.Errorf("refresh token: %w", err)
	}

	// Spotify may omit refresh_token on refresh responses; keep the old one.
	if refreshed.RefreshToken == "" {
		refreshed.RefreshToken = token.RefreshToken
	}

	if err := s.tokens.Save(ctx, refreshed); err != nil {
		s.log.Error("valid_token: persisting refreshed token failed", "error", err)
		return domain.Token{}, fmt.Errorf("persist refreshed token: %w", err)
	}

	s.log.Info("valid_token: refreshed successfully", "expires_at", refreshed.ExpiresAt)
	return refreshed, nil
}

// randomState generates a URL-safe, unpredictable value used to protect the
// OAuth redirect against CSRF, as recommended by Spotify's docs.
func randomState() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
