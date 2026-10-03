package in

import "context"

// PlaylistRef is a minimal playlist identity — enough to drive a picker UI
// (list the user's playlists, let them choose one by name).
type PlaylistRef struct {
	ID   string
	Name string
}

// ArtistRef is a minimal artist identity found within a playlist, enough to
// drive a picker UI. TrackCount is how many of that playlist's tracks are
// theirs, shown so the user can tell at a glance which pick is substantial.
type ArtistRef struct {
	ID         string
	Name       string
	TrackCount int
}

// ArtistPlaylistResult reports what happened creating/upserting the
// artist-only playlist.
type ArtistPlaylistResult struct {
	ArtistName      string
	PlaylistID      string
	PlaylistName    string
	PlaylistCreated bool // false when an existing playlist was reused (upsert)
	TracksAdded     int
	TracksSkipped   int // already present in the destination playlist (upsert dedupe)
}

// ArtistPlaylistService is the driving port behind "pick an artist out of
// one of my playlists, collect their tracks from it into a new playlist":
// list playlists → list artists in the chosen one → create/upsert a
// playlist with just that artist's tracks from it.
type ArtistPlaylistService interface {
	// ListOwnPlaylists lists the current user's playlists, for a picker UI.
	ListOwnPlaylists(ctx context.Context) ([]PlaylistRef, error)

	// ListArtists lists the distinct artists present in the playlist
	// identified by playlistRef (a share URL, spotify: URI, or bare ID),
	// most-tracks-first, for a picker UI. Spotify only returns track data
	// for playlists the current user owns or collaborates on; for any
	// other playlist this returns an empty slice, not an error.
	ListArtists(ctx context.Context, playlistRef string) ([]ArtistRef, error)

	// CreateFromArtist upserts a private playlist named artistName
	// containing every track by artistID found in the playlist identified
	// by playlistRef. Existing playlists/tracks are reused, not duplicated
	// (same upsert semantics as PlaylistSplitService).
	CreateFromArtist(ctx context.Context, playlistRef, artistID, artistName string) (ArtistPlaylistResult, error)
}
