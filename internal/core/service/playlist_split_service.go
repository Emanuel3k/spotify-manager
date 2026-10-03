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
	warnIfNoSourceTracks(s.log, tracks)

	byYear, unknownYear := groupTracksByYear(tracks)
	if unknownYear > 0 {
		s.log.Warn("split_by_year: skipping tracks with unknown release year", "count", unknownYear)
	}

	years := make([]int, 0, len(byYear))
	for y := range byYear {
		years = append(years, y)
	}
	sort.Ints(years)

	byName, err := ownPlaylistsByName(ctx, s.playlists, token.AccessToken, me.ID)
	if err != nil {
		s.log.Error("split_by_year: listing existing playlists failed", "error", err)
		return nil, err
	}

	results := make([]in.YearSplitResult, 0, len(years))
	for _, year := range years {
		yearTracks := byYear[year]
		name := strconv.Itoa(year)
		uris := make([]string, len(yearTracks))
		for i, t := range yearTracks {
			uris[i] = t.URI
		}

		playlist, created, added, skipped, err := upsertPlaylist(ctx, s.playlists, s.log.With("year", year), token.AccessToken, name, byName, uris)
		if err != nil {
			return nil, err
		}

		results = append(results, in.YearSplitResult{
			Year:            year,
			PlaylistID:      playlist.ID,
			PlaylistName:    name,
			PlaylistCreated: created,
			TracksInYear:    len(yearTracks),
			TracksAdded:     added,
			TracksSkipped:   skipped,
		})
	}

	s.log.Info("split_by_year: done", "years", len(results))
	return results, nil
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
