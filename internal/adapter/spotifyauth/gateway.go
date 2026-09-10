// Package spotifyauth implements the out.SpotifyAuthGateway driven port by
// talking to Spotify's Accounts service over HTTP, per
// https://developer.spotify.com/documentation/web-api/tutorials/code-flow
package spotifyauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

const (
	defaultAuthorizeURL = "https://accounts.spotify.com/authorize"
	defaultTokenURL     = "https://accounts.spotify.com/api/token"
)

// Gateway implements out.SpotifyAuthGateway using the Authorization Code
// Flow (not PKCE): since the CLI holds a confidential client secret
// (provided by the user for their own personal app), token exchange and
// refresh are authenticated with HTTP Basic auth as Spotify requires.
type Gateway struct {
	clientID     string
	clientSecret string
	redirectURI  string
	httpClient   *http.Client
	log          *slog.Logger

	// authorizeURL/tokenURL default to Spotify's real endpoints; tests
	// override them to point at an httptest.Server.
	authorizeURL string
	tokenURL     string
}

var _ out.SpotifyAuthGateway = (*Gateway)(nil)

// New creates a Gateway for the given app credentials and redirect URI.
// logger may be nil, in which case log output is discarded.
func New(clientID, clientSecret, redirectURI string, logger *slog.Logger) *Gateway {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Gateway{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURI:  redirectURI,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		log:          logger.With("component", "spotifyauth_gateway"),
		authorizeURL: defaultAuthorizeURL,
		tokenURL:     defaultTokenURL,
	}
}

// BuildAuthorizeURL builds the URL the user's browser must open to reach
// Spotify's consent screen.
func (g *Gateway) BuildAuthorizeURL(state string, scopes []string) string {
	q := url.Values{}
	q.Set("client_id", g.clientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", g.redirectURI)
	q.Set("state", state)
	q.Set("scope", strings.Join(scopes, " "))

	return g.authorizeURL + "?" + q.Encode()
}

// ExchangeCode swaps an authorization code for a token pair.
func (g *Gateway) ExchangeCode(ctx context.Context, code string) (domain.Token, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", g.redirectURI)

	return g.postForm(ctx, "exchange_code", form)
}

// RefreshToken swaps a refresh token for a new access token.
func (g *Gateway) RefreshToken(ctx context.Context, refreshToken string) (domain.Token, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)

	return g.postForm(ctx, "refresh_token", form)
}

// tokenResponse mirrors the JSON body returned by POST /api/token.
type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}

// errorResponse mirrors Spotify's { "error": "...", "error_description": "..." }
// body returned on 4xx responses from the token endpoint.
type errorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

func (g *Gateway) postForm(ctx context.Context, op string, form url.Values) (domain.Token, error) {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return domain.Token{}, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+g.basicAuth())

	g.log.Debug("token endpoint request", "op", op, "grant_type", form.Get("grant_type"))

	resp, err := g.httpClient.Do(req)
	if err != nil {
		g.log.Error("token endpoint call failed", "op", op, "error", err)
		return domain.Token{}, fmt.Errorf("call token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		g.log.Error("token endpoint response read failed", "op", op, "error", err)
		return domain.Token{}, fmt.Errorf("read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var apiErr errorResponse
		_ = json.Unmarshal(body, &apiErr)
		g.log.Error("token endpoint returned an error", "op", op, "status", resp.Status, "spotify_error", apiErr.Error, "description", apiErr.ErrorDescription)
		if apiErr.Error != "" {
			return domain.Token{}, fmt.Errorf("spotify token endpoint returned %s: %s (%s)", resp.Status, apiErr.Error, apiErr.ErrorDescription)
		}
		return domain.Token{}, fmt.Errorf("spotify token endpoint returned %s: %s", resp.Status, string(body))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		g.log.Error("token endpoint response decode failed", "op", op, "error", err)
		return domain.Token{}, fmt.Errorf("decode token response: %w", err)
	}

	g.log.Debug("token endpoint request succeeded", "op", op, "scope", tr.Scope, "expires_in", tr.ExpiresIn, "duration_ms", time.Since(start).Milliseconds())

	return domain.Token{
		AccessToken:  tr.AccessToken,
		TokenType:    tr.TokenType,
		Scope:        tr.Scope,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second),
	}, nil
}

func (g *Gateway) basicAuth() string {
	return base64.StdEncoding.EncodeToString([]byte(g.clientID + ":" + g.clientSecret))
}
