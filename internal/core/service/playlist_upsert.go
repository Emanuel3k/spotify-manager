package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// ownPlaylistsByName fetches the current user's playlists and indexes them
// by name, for upsert lookups. Only playlists actually owned by ownerID are
// kept (out.PlaylistGateway.ListOwnPlaylists can include ones merely
// followed, depending on the Web API's own response shape).
func ownPlaylistsByName(ctx context.Context, gw out.PlaylistGateway, accessToken, ownerID string) (map[string]domain.Playlist, error) {
	existing, err := gw.ListOwnPlaylists(ctx, accessToken, ownerID)
	if err != nil {
		return nil, fmt.Errorf("list your playlists: %w", err)
	}

	byName := make(map[string]domain.Playlist, len(existing))
	for _, p := range existing {
		if p.OwnerID == ownerID {
			byName[p.Name] = p
		}
	}
	return byName, nil
}

// upsertPlaylist finds an existing user playlist named `name` (via
// existingByName) or creates a new private one, then adds every URI in
// wantURIs that isn't already in it. This is the shared "upsert playlist +
// upsert tracks" logic behind both PlaylistSplitService and
// ArtistPlaylistService — duplicate URIs within wantURIs itself are also
// deduped, so a caller doesn't need to pre-clean its input.
func upsertPlaylist(
	ctx context.Context,
	gw out.PlaylistGateway,
	log *slog.Logger,
	accessToken, name string,
	existingByName map[string]domain.Playlist,
	wantURIs []string,
) (playlist domain.Playlist, created bool, added, skipped int, err error) {
	var alreadyExisted bool
	playlist, alreadyExisted = existingByName[name]
	if !alreadyExisted {
		log.Info("upsert_playlist: creating playlist", "playlist_name", name)
		playlist, err = gw.CreatePlaylist(ctx, accessToken, name, false)
		if err != nil {
			return domain.Playlist{}, false, 0, 0, fmt.Errorf("create playlist %s: %w", name, err)
		}
		created = true
	} else {
		log.Info("upsert_playlist: reusing existing playlist", "playlist_name", name, "playlist_id", playlist.ID)
	}

	existingURIs, err := gw.ListTrackURIs(ctx, accessToken, playlist.ID)
	if err != nil {
		return domain.Playlist{}, false, 0, 0, fmt.Errorf("list tracks of playlist %s: %w", name, err)
	}

	toAdd := make([]string, 0, len(wantURIs))
	for _, uri := range wantURIs {
		if _, ok := existingURIs[uri]; ok {
			skipped++
			continue
		}
		toAdd = append(toAdd, uri)
		// Mark as seen so a duplicate URI later in wantURIs isn't queued
		// twice in this same run (upsert must stay idempotent within a
		// single call, not just across calls).
		existingURIs[uri] = struct{}{}
	}

	if len(toAdd) > 0 {
		if err := gw.AddTracks(ctx, accessToken, playlist.ID, toAdd); err != nil {
			return domain.Playlist{}, false, 0, 0, fmt.Errorf("add tracks to playlist %s: %w", name, err)
		}
	}

	added = len(toAdd)
	log.Info("upsert_playlist: updated", "playlist_name", name, "created", created, "added", added, "already_present", skipped)
	return playlist, created, added, skipped, nil
}

// warnIfNoSourceTracks logs a hint explaining the most common reason a
// source playlist comes back with zero tracks: Spotify only returns track
// data for playlists the current user owns or collaborates on (see
// CLAUDE.md's "Known Spotify Web API gotchas").
func warnIfNoSourceTracks(log *slog.Logger, tracks []domain.Track) {
	if len(tracks) == 0 {
		log.Warn("source playlist returned zero tracks; " +
			"Spotify only returns playlist contents for playlists you own or collaborate on, " +
			"so this may mean the playlist belongs to someone else")
	}
}
