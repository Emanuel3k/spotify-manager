package service

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/in"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// PlaylistSplitService implements port/in.PlaylistSplitService: it splits a
// source playlist's tracks into per-release-year playlists, upserting both
// the playlists and their tracks.
type PlaylistSplitService struct {
	auth      in.AuthService
	profile   in.ProfileService
	playlists out.PlaylistGateway
	log       *slog.Logger
}

var _ in.PlaylistSplitService = (*PlaylistSplitService)(nil)

// NewPlaylistSplitService wires the use case to auth (for a valid token),
// profile (to resolve the current user id, needed to create playlists) and
// the playlist driven port. logger may be nil, in which case log output is
// discarded.
func NewPlaylistSplitService(auth in.AuthService, profile in.ProfileService, playlists out.PlaylistGateway, logger *slog.Logger) *PlaylistSplitService {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &PlaylistSplitService{
		auth:      auth,
		profile:   profile,
		playlists: playlists,
		log:       logger.With("component", "playlist_split_service"),
	}
}

func (s *PlaylistSplitService) SplitByYear(ctx context.Context, playlistLink string) ([]in.YearSplitResult, error) {
	playlistID, err := domain.ParsePlaylistID(playlistLink)
	if err != nil {
		s.log.Error("split_by_year: could not parse playlist link", "input", playlistLink, "error", err)
		return nil, fmt.Errorf("parse playlist link: %w", err)
	}
	s.log.Info("split_by_year: starting", "source_playlist_id", playlistID)

	token, err := s.auth.ValidToken(ctx)
	if err != nil {
		s.log.Error("split_by_year: could not obtain a valid token", "error", err)
		return nil, err
	}

	me, err := s.profile.Me(ctx)
	if err != nil {
		s.log.Error("split_by_year: could not resolve current user", "error", err)
		return nil, fmt.Errorf("resolve current user: %w", err)
	}

	tracks, err := s.playlists.ListTracks(ctx, token.AccessToken, playlistID)
	if err != nil {
		s.log.Error("split_by_year: listing source playlist tracks failed", "error", err)
		return nil, fmt.Errorf("list source playlist tracks: %w", err)
	}
	s.log.Info("split_by_year: fetched source tracks", "count", len(tracks))
	if len(tracks) == 0 {
		s.log.Warn("split_by_year: source playlist returned zero tracks; " +
			"Spotify only returns playlist contents for playlists you own or collaborate on, " +
			"so this may mean the playlist belongs to someone else")
	}

	byYear, unknownYear := groupTracksByYear(tracks)
	if unknownYear > 0 {
		s.log.Warn("split_by_year: skipping tracks with unknown release year", "count", unknownYear)
	}

	years := make([]int, 0, len(byYear))
	for y := range byYear {
		years = append(years, y)
	}
	sort.Ints(years)

	existing, err := s.playlists.ListOwnPlaylists(ctx, token.AccessToken, me.ID)
	if err != nil {
		s.log.Error("split_by_year: listing existing playlists failed", "error", err)
		return nil, fmt.Errorf("list your playlists: %w", err)
	}
	byName := make(map[string]domain.Playlist, len(existing))
	for _, p := range existing {
		if p.OwnerID == me.ID {
			byName[p.Name] = p
		}
	}

	results := make([]in.YearSplitResult, 0, len(years))
	for _, year := range years {
		result, err := s.upsertYearPlaylist(ctx, token.AccessToken, year, byYear[year], byName)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	s.log.Info("split_by_year: done", "years", len(results))
	return results, nil
}

func (s *PlaylistSplitService) upsertYearPlaylist(
	ctx context.Context,
	accessToken string,
	year int,
	yearTracks []domain.Track,
	byName map[string]domain.Playlist,
) (in.YearSplitResult, error) {
	name := strconv.Itoa(year)
	log := s.log.With("year", year, "playlist_name", name)

	playlist, alreadyExisted := byName[name]
	created := false
	if !alreadyExisted {
		log.Info("split_by_year: creating year playlist")
		var err error
		playlist, err = s.playlists.CreatePlaylist(ctx, accessToken, name, false)
		if err != nil {
			log.Error("split_by_year: create playlist failed", "error", err)
			return in.YearSplitResult{}, fmt.Errorf("create playlist %s: %w", name, err)
		}
		created = true
	} else {
		log.Info("split_by_year: reusing existing year playlist", "playlist_id", playlist.ID)
	}

	existingURIs, err := s.playlists.ListTrackURIs(ctx, accessToken, playlist.ID)
	if err != nil {
		log.Error("split_by_year: listing year playlist tracks failed", "error", err)
		return in.YearSplitResult{}, fmt.Errorf("list tracks of playlist %s: %w", name, err)
	}

	toAdd := make([]string, 0, len(yearTracks))
	skipped := 0
	for _, t := range yearTracks {
		if _, ok := existingURIs[t.URI]; ok {
			skipped++
			continue
		}
		toAdd = append(toAdd, t.URI)
		// Mark as seen so a duplicate URI later in the same source
		// playlist isn't queued twice in this same run (upsert must stay
		// idempotent within a single call, not just across calls).
		existingURIs[t.URI] = struct{}{}
	}

	if len(toAdd) > 0 {
		if err := s.playlists.AddTracks(ctx, accessToken, playlist.ID, toAdd); err != nil {
			log.Error("split_by_year: adding tracks failed", "error", err, "count", len(toAdd))
			return in.YearSplitResult{}, fmt.Errorf("add tracks to playlist %s: %w", name, err)
		}
	}

	log.Info("split_by_year: year playlist updated", "created", created, "added", len(toAdd), "already_present", skipped)

	return in.YearSplitResult{
		Year:            year,
		PlaylistID:      playlist.ID,
		PlaylistName:    name,
		PlaylistCreated: created,
		TracksInYear:    len(yearTracks),
		TracksAdded:     len(toAdd),
		TracksSkipped:   skipped,
	}, nil
}

// groupTracksByYear buckets tracks by domain.Track.ReleaseYear, reporting
// separately how many were excluded for having no known release year.
func groupTracksByYear(tracks []domain.Track) (map[int][]domain.Track, int) {
	byYear := make(map[int][]domain.Track)
	unknown := 0
	for _, t := range tracks {
		if t.ReleaseYear == 0 {
			unknown++
			continue
		}
		byYear[t.ReleaseYear] = append(byYear[t.ReleaseYear], t)
	}
	return byYear, unknown
}
