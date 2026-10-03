package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/in"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// trackCacheTTL bounds how long a fetched playlist's tracks are reused
// across the two calls this use case's UI flow always makes back-to-back
// (ListArtists to show a picker, then CreateFromArtist once the user picks
// one): without it, a large playlist (seen: 2000+ tracks) was fetched in
// full twice, roughly doubling an already-slow operation. Short enough that
// a stale read is very unlikely to matter for a personal tool like this.
const trackCacheTTL = 2 * time.Minute

// ArtistPlaylistService implements port/in.ArtistPlaylistService: pick an
// artist out of one of the user's playlists and collect that artist's
// tracks (from that playlist) into a new, upserted playlist.
type ArtistPlaylistService struct {
	auth      in.AuthService
	profile   in.ProfileService
	playlists out.PlaylistGateway
	log       *slog.Logger

	cacheMu sync.Mutex
	cache   map[string]cachedTracks // playlistID -> last fetch
}

type cachedTracks struct {
	tracks    []domain.Track
	fetchedAt time.Time
}

var _ in.ArtistPlaylistService = (*ArtistPlaylistService)(nil)

// NewArtistPlaylistService wires the use case to auth (for a valid token),
// profile (to resolve the current user id) and the playlist driven port.
// logger may be nil, in which case log output is discarded.
func NewArtistPlaylistService(auth in.AuthService, profile in.ProfileService, playlists out.PlaylistGateway, logger *slog.Logger) *ArtistPlaylistService {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &ArtistPlaylistService{
		auth:      auth,
		profile:   profile,
		playlists: playlists,
		log:       logger.With("component", "artist_playlist_service"),
		cache:     make(map[string]cachedTracks),
	}
}

// tracksFor returns playlistID's tracks, reusing a recent fetch (within
// trackCacheTTL) instead of re-fetching the whole playlist.
func (s *ArtistPlaylistService) tracksFor(ctx context.Context, accessToken, playlistID string) ([]domain.Track, error) {
	s.cacheMu.Lock()
	cached, ok := s.cache[playlistID]
	s.cacheMu.Unlock()
	if ok && time.Since(cached.fetchedAt) < trackCacheTTL {
		s.log.Debug("tracks_for: cache hit, skipping a redundant fetch", "playlist_id", playlistID, "count", len(cached.tracks))
		return cached.tracks, nil
	}

	tracks, err := s.playlists.ListTracks(ctx, accessToken, playlistID)
	if err != nil {
		return nil, err
	}

	s.cacheMu.Lock()
	s.cache[playlistID] = cachedTracks{tracks: tracks, fetchedAt: time.Now()}
	s.cacheMu.Unlock()

	return tracks, nil
}

func (s *ArtistPlaylistService) ListOwnPlaylists(ctx context.Context) ([]in.PlaylistRef, error) {
	token, err := s.auth.ValidToken(ctx)
	if err != nil {
		s.log.Error("list_own_playlists: could not obtain a valid token", "error", err)
		return nil, err
	}

	me, err := s.profile.Me(ctx)
	if err != nil {
		s.log.Error("list_own_playlists: could not resolve current user", "error", err)
		return nil, fmt.Errorf("resolve current user: %w", err)
	}

	playlists, err := s.playlists.ListOwnPlaylists(ctx, token.AccessToken, me.ID)
	if err != nil {
		s.log.Error("list_own_playlists: listing playlists failed", "error", err)
		return nil, fmt.Errorf("list your playlists: %w", err)
	}

	refs := make([]in.PlaylistRef, 0, len(playlists))
	for _, p := range playlists {
		if p.OwnerID != me.ID {
			continue
		}
		refs = append(refs, in.PlaylistRef{ID: p.ID, Name: p.Name})
	}

	s.log.Info("list_own_playlists: done", "count", len(refs))
	return refs, nil
}

func (s *ArtistPlaylistService) ListArtists(ctx context.Context, playlistRef string) ([]in.ArtistRef, error) {
	playlistID, err := domain.ParsePlaylistID(playlistRef)
	if err != nil {
		s.log.Error("list_artists: could not parse playlist reference", "input", playlistRef, "error", err)
		return nil, fmt.Errorf("parse playlist link: %w", err)
	}

	token, err := s.auth.ValidToken(ctx)
	if err != nil {
		s.log.Error("list_artists: could not obtain a valid token", "error", err)
		return nil, err
	}

	tracks, err := s.tracksFor(ctx, token.AccessToken, playlistID)
	if err != nil {
		s.log.Error("list_artists: listing source playlist tracks failed", "error", err)
		return nil, fmt.Errorf("list source playlist tracks: %w", err)
	}
	warnIfNoSourceTracks(s.log, tracks)

	artists := aggregateArtists(tracks)
	s.log.Info("list_artists: done", "source_playlist_id", playlistID, "artist_count", len(artists))
	return artists, nil
}

func (s *ArtistPlaylistService) CreateFromArtist(ctx context.Context, playlistRef, artistID, artistName string) (in.ArtistPlaylistResult, error) {
	playlistID, err := domain.ParsePlaylistID(playlistRef)
	if err != nil {
		s.log.Error("create_from_artist: could not parse playlist reference", "input", playlistRef, "error", err)
		return in.ArtistPlaylistResult{}, fmt.Errorf("parse playlist link: %w", err)
	}
	log := s.log.With("source_playlist_id", playlistID, "artist_id", artistID, "artist_name", artistName)
	log.Info("create_from_artist: starting")

	token, err := s.auth.ValidToken(ctx)
	if err != nil {
		log.Error("create_from_artist: could not obtain a valid token", "error", err)
		return in.ArtistPlaylistResult{}, err
	}

	me, err := s.profile.Me(ctx)
	if err != nil {
		log.Error("create_from_artist: could not resolve current user", "error", err)
		return in.ArtistPlaylistResult{}, fmt.Errorf("resolve current user: %w", err)
	}

	tracks, err := s.tracksFor(ctx, token.AccessToken, playlistID)
	if err != nil {
		log.Error("create_from_artist: listing source playlist tracks failed", "error", err)
		return in.ArtistPlaylistResult{}, fmt.Errorf("list source playlist tracks: %w", err)
	}
	warnIfNoSourceTracks(log, tracks)

	uris := make([]string, 0, len(tracks))
	for _, t := range tracks {
		for _, a := range t.Artists {
			if a.ID == artistID {
				uris = append(uris, t.URI)
				break
			}
		}
	}
	if len(uris) == 0 {
		log.Warn("create_from_artist: no tracks by this artist found in the source playlist")
		return in.ArtistPlaylistResult{}, fmt.Errorf("no tracks by that artist found in the source playlist")
	}
	log.Info("create_from_artist: matched tracks", "count", len(uris))

	byName, err := ownPlaylistsByName(ctx, s.playlists, token.AccessToken, me.ID)
	if err != nil {
		log.Error("create_from_artist: listing existing playlists failed", "error", err)
		return in.ArtistPlaylistResult{}, err
	}

	playlist, created, added, skipped, err := upsertPlaylist(ctx, s.playlists, log, token.AccessToken, artistName, byName, uris)
	if err != nil {
		return in.ArtistPlaylistResult{}, err
	}

	log.Info("create_from_artist: done", "created", created, "added", added, "already_present", skipped)
	return in.ArtistPlaylistResult{
		ArtistName:      artistName,
		PlaylistID:      playlist.ID,
		PlaylistName:    playlist.Name,
		PlaylistCreated: created,
		TracksAdded:     added,
		TracksSkipped:   skipped,
	}, nil
}

// aggregateArtists collects the distinct artists across tracks, counting
// how many tracks belong to each, sorted most-tracks-first (ties broken
// alphabetically) so the most "worth picking" artists surface first in a
// picker UI. Tracks with no artist ID (shouldn't normally happen) are
// skipped.
func aggregateArtists(tracks []domain.Track) []in.ArtistRef {
	type agg struct {
		name  string
		count int
	}
	byID := make(map[string]*agg)
	order := make([]string, 0)

	for _, t := range tracks {
		for _, a := range t.Artists {
			if a.ID == "" {
				continue
			}
			entry, ok := byID[a.ID]
			if !ok {
				entry = &agg{name: a.Name}
				byID[a.ID] = entry
				order = append(order, a.ID)
			}
			entry.count++
		}
	}

	artists := make([]in.ArtistRef, 0, len(order))
	for _, id := range order {
		a := byID[id]
		artists = append(artists, in.ArtistRef{ID: id, Name: a.name, TrackCount: a.count})
	}

	sort.Slice(artists, func(i, j int) bool {
		if artists[i].TrackCount != artists[j].TrackCount {
			return artists[i].TrackCount > artists[j].TrackCount
		}
		return artists[i].Name < artists[j].Name
	})
	return artists
}
