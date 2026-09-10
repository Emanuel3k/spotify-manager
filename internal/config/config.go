// Package config loads application configuration (Spotify app credentials,
// redirect URI, requested scopes, file locations) from the process
// environment / a local .env file. It sits outside the hexagon: it is
// infrastructure, not a domain concern, but every adapter that needs
// configuration depends on this instead of reading os.Getenv itself.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

// DefaultScopes is the set of permissions requested at login. It is broad
// on purpose: this CLI is meant to grow into several personal Spotify
// features (playback control, library, playlists, top items, ...) and
// re-authenticating every time a new feature needs a new scope is poor UX.
var DefaultScopes = []string{
	"user-read-private",
	"user-read-email",
	"user-library-read",
	"user-library-modify",
	"user-top-read",
	"user-read-recently-played",
	"user-read-playback-state",
	"user-modify-playback-state",
	"user-read-currently-playing",
	"playlist-read-private",
	"playlist-read-collaborative",
	"playlist-modify-public",
	"playlist-modify-private",
	"streaming",
}

// Config holds everything the auth flow (and future features) need.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Scopes       []string
}

// Load reads configuration from a .env file in the working directory (if
// present) and then from the environment, which always takes precedence.
// Recognized variables:
//
//	SPOTIFY_CLIENT_ID       (required)
//	SPOTIFY_CLIENT_SECRET   (required)
//	SPOTIFY_REDIRECT_URI    (optional, defaults to http://127.0.0.1:8080/callback)
//	SPOTIFY_SCOPES          (optional, space-separated, overrides DefaultScopes)
func Load() (Config, error) {
	// Best effort: it's fine if there is no .env file, e.g. in CI or when
	// the user exports variables in their shell profile instead.
	_ = godotenv.Load()

	cfg := Config{
		ClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		ClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		RedirectURI:  os.Getenv("SPOTIFY_REDIRECT_URI"),
		Scopes:       DefaultScopes,
	}

	if cfg.RedirectURI == "" {
		cfg.RedirectURI = "http://127.0.0.1:8080/callback"
	}

	if raw := os.Getenv("SPOTIFY_SCOPES"); raw != "" {
		cfg.Scopes = strings.Fields(raw)
	}

	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return Config{}, fmt.Errorf(
			"missing Spotify credentials: set SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET " +
				"(e.g. in a .env file — see .env.example)",
		)
	}

	return cfg, nil
}
