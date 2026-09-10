package out

import (
	"context"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

// PlaylistGateway is the driven port through which the core reads and
// writes playlists via the Spotify Web API. All methods take accessToken
// explicitly (rather than the gateway holding a token itself), matching
// ProfileGateway's convention: the core always fetches a guaranteed-valid
// token via AuthService first.
type PlaylistGateway interface {
	// ListTracks returns every track in the given playlist, paginating as
	// needed. Local files and removed/null tracks are skipped.
	ListTracks(ctx context.Context, accessToken, playlistID string) ([]domain.Track, error)

	// ListTrackURIs returns the set of track URIs already present in the
	// given playlist, for upsert/dedupe purposes. Cheaper than ListTracks
	// since it doesn't need names/artists/release dates.
	ListTrackURIs(ctx context.Context, accessToken, playlistID string) (map[string]struct{}, error)

	// ListOwnPlaylists returns every playlist owned by the current user
	// (ownerID), so the core can look one up by name for upsert purposes.
	ListOwnPlaylists(ctx context.Context, accessToken, ownerID string) ([]domain.Playlist, error)

	// CreatePlaylist creates a new playlist for the currently authenticated
	// user (identified implicitly by accessToken).
	CreatePlaylist(ctx context.Context, accessToken, name string, public bool) (domain.Playlist, error)

	// AddTracks appends the given track URIs to a playlist, chunking
	// internally to respect the Web API's 100-tracks-per-request limit.
	// Callers are responsible for not passing URIs already in the playlist.
	AddTracks(ctx context.Context, accessToken, playlistID string, trackURIs []string) error
}
