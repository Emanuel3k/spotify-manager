package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/in"
	"github.com/Emanuel3k/spotify-manager/internal/core/service"
)

func newTestSplitService(auth *fakeAuthService, profile *fakeProfileService, gw *fakePlaylistGateway) *service.PlaylistSplitService {
	return service.NewPlaylistSplitService(auth, profile, gw, nil)
}

func resultsByYear(results []in.YearSplitResult) map[int]in.YearSplitResult {
	m := make(map[int]in.YearSplitResult, len(results))
	for _, r := range results {
		m[r.Year] = r
	}
	return m
}

const validPlaylistLink = "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"

func TestPlaylistSplitService_SplitByYear_CreatesOnePlaylistPerYear(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{
		{URI: "spotify:track:1", ReleaseYear: 2020},
		{URI: "spotify:track:2", ReleaseYear: 2020},
		{URI: "spotify:track:3", ReleaseYear: 2019},
		{URI: "spotify:track:4", ReleaseYear: 0}, // unknown year, must be skipped
	}

	svc := newTestSplitService(auth, profile, gw)

	results, err := svc.SplitByYear(context.Background(), validPlaylistLink)
	if err != nil {
		t.Fatalf("SplitByYear() error = %v, want nil", err)
	}
	if len(results) != 2 {
		t.Fatalf("SplitByYear() returned %d results, want 2 (one per year)", len(results))
	}

	byYear := resultsByYear(results)

	r2020 := byYear[2020]
	if !r2020.PlaylistCreated || r2020.PlaylistName != "2020" || r2020.TracksAdded != 2 || r2020.TracksSkipped != 0 {
		t.Errorf("2020 result = %+v, want created playlist named 2020 with 2 tracks added", r2020)
	}
	r2019 := byYear[2019]
	if !r2019.PlaylistCreated || r2019.PlaylistName != "2019" || r2019.TracksAdded != 1 {
		t.Errorf("2019 result = %+v, want created playlist named 2019 with 1 track added", r2019)
	}

	if got := gw.addedTracks[r2020.PlaylistID]; len(got) != 2 {
		t.Errorf("playlist %s got tracks %v, want 2", r2020.PlaylistID, got)
	}
}

func TestPlaylistSplitService_SplitByYear_UpsertsExistingPlaylist(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{
		{URI: "spotify:track:new", ReleaseYear: 2021},
	}
	// A "2021" playlist already exists, owned by the current user.
	gw.ownPlaylists = []domain.Playlist{{ID: "existing-2021", Name: "2021", OwnerID: "me"}}

	svc := newTestSplitService(auth, profile, gw)

	results, err := svc.SplitByYear(context.Background(), validPlaylistLink)
	if err != nil {
		t.Fatalf("SplitByYear() error = %v, want nil", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	got := results[0]
	if got.PlaylistCreated {
		t.Errorf("existing playlist should be reused, not recreated: %+v", got)
	}
	if got.PlaylistID != "existing-2021" {
		t.Errorf("PlaylistID = %q, want %q", got.PlaylistID, "existing-2021")
	}
	if len(gw.createdPlaylists) != 0 {
		t.Errorf("CreatePlaylist should not have been called, got %d calls", len(gw.createdPlaylists))
	}
	if got.TracksAdded != 1 {
		t.Errorf("TracksAdded = %d, want 1", got.TracksAdded)
	}
}

func TestPlaylistSplitService_SplitByYear_UpsertsTracks_SkipsAlreadyPresent(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{
		{URI: "spotify:track:already-there", ReleaseYear: 2022},
		{URI: "spotify:track:brand-new", ReleaseYear: 2022},
	}
	gw.ownPlaylists = []domain.Playlist{{ID: "p2022", Name: "2022", OwnerID: "me"}}
	gw.trackURIs["p2022"] = map[string]struct{}{"spotify:track:already-there": {}}

	svc := newTestSplitService(auth, profile, gw)

	results, err := svc.SplitByYear(context.Background(), validPlaylistLink)
	if err != nil {
		t.Fatalf("SplitByYear() error = %v, want nil", err)
	}

	got := results[0]
	if got.TracksAdded != 1 || got.TracksSkipped != 1 {
		t.Fatalf("got TracksAdded=%d TracksSkipped=%d, want 1 and 1", got.TracksAdded, got.TracksSkipped)
	}
	added := gw.addedTracks["p2022"]
	if len(added) != 1 || added[0] != "spotify:track:brand-new" {
		t.Errorf("AddTracks called with %v, want only the new track", added)
	}
}

func TestPlaylistSplitService_SplitByYear_DedupesDuplicateURIsInSourcePlaylist(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{
		{URI: "spotify:track:dup", ReleaseYear: 2023},
		{URI: "spotify:track:dup", ReleaseYear: 2023}, // same song appears twice in the source
	}

	svc := newTestSplitService(auth, profile, gw)

	results, err := svc.SplitByYear(context.Background(), validPlaylistLink)
	if err != nil {
		t.Fatalf("SplitByYear() error = %v, want nil", err)
	}

	got := results[0]
	if got.TracksAdded != 1 || got.TracksSkipped != 1 {
		t.Fatalf("got TracksAdded=%d TracksSkipped=%d, want 1 and 1 (second occurrence deduped)", got.TracksAdded, got.TracksSkipped)
	}
}

func TestPlaylistSplitService_SplitByYear_NoTracksWithKnownYear(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{{URI: "spotify:track:x", ReleaseYear: 0}}

	svc := newTestSplitService(auth, profile, gw)

	results, err := svc.SplitByYear(context.Background(), validPlaylistLink)
	if err != nil {
		t.Fatalf("SplitByYear() error = %v, want nil", err)
	}
	if len(results) != 0 {
		t.Fatalf("got %d results, want 0", len(results))
	}
	if len(gw.createdPlaylists) != 0 {
		t.Errorf("no playlist should have been created")
	}
}

func TestPlaylistSplitService_SplitByYear_InvalidLink(t *testing.T) {
	svc := newTestSplitService(&fakeAuthService{}, &fakeProfileService{}, newFakePlaylistGateway())

	_, err := svc.SplitByYear(context.Background(), "not a playlist link")
	if err == nil {
		t.Fatal("SplitByYear() error = nil, want an error for an unparseable link")
	}
}

func TestPlaylistSplitService_SplitByYear_PropagatesAuthError(t *testing.T) {
	wantErr := errors.New("not authenticated")
	auth := &fakeAuthService{validErr: wantErr}
	svc := newTestSplitService(auth, &fakeProfileService{}, newFakePlaylistGateway())

	_, err := svc.SplitByYear(context.Background(), validPlaylistLink)
	if !errors.Is(err, wantErr) {
		t.Fatalf("SplitByYear() error = %v, want %v", err, wantErr)
	}
}

func TestPlaylistSplitService_SplitByYear_CreatePlaylistPrivate(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{{URI: "spotify:track:1", ReleaseYear: 2018}}

	svc := newTestSplitService(auth, profile, gw)

	if _, err := svc.SplitByYear(context.Background(), validPlaylistLink); err != nil {
		t.Fatalf("SplitByYear() error = %v, want nil", err)
	}

	if len(gw.createdPlaylists) != 1 {
		t.Fatalf("got %d created playlists, want 1", len(gw.createdPlaylists))
	}
	if gw.createdPlaylists[0].Public {
		t.Errorf("year playlist must be created private, got Public = true")
	}
}
