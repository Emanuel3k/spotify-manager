package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/service"
)

func newTestArtistPlaylistService(auth *fakeAuthService, profile *fakeProfileService, gw *fakePlaylistGateway) *service.ArtistPlaylistService {
	return service.NewArtistPlaylistService(auth, profile, gw, nil)
}

func TestArtistPlaylistService_ListOwnPlaylists(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.ownPlaylists = []domain.Playlist{
		{ID: "p1", Name: "My Playlist", OwnerID: "me"},
		{ID: "p2", Name: "Someone else's", OwnerID: "someone-else"},
	}

	svc := newTestArtistPlaylistService(auth, profile, gw)
	refs, err := svc.ListOwnPlaylists(context.Background())
	if err != nil {
		t.Fatalf("ListOwnPlaylists() error = %v, want nil", err)
	}
	if len(refs) != 1 || refs[0].ID != "p1" {
		t.Fatalf("ListOwnPlaylists() = %+v, want only p1 (owned by me)", refs)
	}
}

func TestArtistPlaylistService_ListArtists_AggregatesAndSorts(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{
		{URI: "spotify:track:1", Artists: []domain.Artist{{ID: "a-queen", Name: "Queen"}}},
		{URI: "spotify:track:2", Artists: []domain.Artist{{ID: "a-queen", Name: "Queen"}}},
		{URI: "spotify:track:3", Artists: []domain.Artist{{ID: "a-adele", Name: "Adele"}}},
		{URI: "spotify:track:4", Artists: []domain.Artist{{ID: "a-queen", Name: "Queen"}, {ID: "a-bowie", Name: "David Bowie"}}},
	}

	svc := newTestArtistPlaylistService(auth, profile, gw)
	artists, err := svc.ListArtists(context.Background(), validPlaylistLink)
	if err != nil {
		t.Fatalf("ListArtists() error = %v, want nil", err)
	}
	if len(artists) != 3 {
		t.Fatalf("got %d artists, want 3 (Queen, Adele, David Bowie)", len(artists))
	}

	// Most tracks first: Queen (3), then Adele/David Bowie (1 each, alphabetical).
	if artists[0].ID != "a-queen" || artists[0].TrackCount != 3 {
		t.Errorf("artists[0] = %+v, want Queen with TrackCount 3", artists[0])
	}
	if artists[1].Name != "Adele" || artists[2].Name != "David Bowie" {
		t.Errorf("tie-break order = [%s, %s], want [Adele, David Bowie]", artists[1].Name, artists[2].Name)
	}
}

func TestArtistPlaylistService_CreateFromArtist_FiltersAndUpserts(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{
		{URI: "spotify:track:queen1", Artists: []domain.Artist{{ID: "a-queen", Name: "Queen"}}},
		{URI: "spotify:track:queen2", Artists: []domain.Artist{{ID: "a-queen", Name: "Queen"}}},
		{URI: "spotify:track:adele1", Artists: []domain.Artist{{ID: "a-adele", Name: "Adele"}}},
	}

	svc := newTestArtistPlaylistService(auth, profile, gw)
	result, err := svc.CreateFromArtist(context.Background(), validPlaylistLink, "a-queen", "Queen")
	if err != nil {
		t.Fatalf("CreateFromArtist() error = %v, want nil", err)
	}

	if !result.PlaylistCreated || result.TracksAdded != 2 || result.TracksSkipped != 0 {
		t.Fatalf("result = %+v, want a newly created playlist with 2 tracks added", result)
	}
	if result.PlaylistName != "Queen" {
		t.Errorf("PlaylistName = %q, want %q", result.PlaylistName, "Queen")
	}
	added := gw.addedTracks[result.PlaylistID]
	if len(added) != 2 {
		t.Fatalf("playlist got %v, want exactly Queen's 2 tracks", added)
	}
	for _, uri := range added {
		if uri == "spotify:track:adele1" {
			t.Errorf("Adele's track should not have been added to the Queen playlist")
		}
	}
}

func TestArtistPlaylistService_CreateFromArtist_UpsertsExistingPlaylistAndDedupesTracks(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{
		{URI: "spotify:track:already-there", Artists: []domain.Artist{{ID: "a-queen", Name: "Queen"}}},
		{URI: "spotify:track:brand-new", Artists: []domain.Artist{{ID: "a-queen", Name: "Queen"}}},
	}
	gw.ownPlaylists = []domain.Playlist{{ID: "existing-queen", Name: "Queen", OwnerID: "me"}}
	gw.trackURIs["existing-queen"] = map[string]struct{}{"spotify:track:already-there": {}}

	svc := newTestArtistPlaylistService(auth, profile, gw)
	result, err := svc.CreateFromArtist(context.Background(), validPlaylistLink, "a-queen", "Queen")
	if err != nil {
		t.Fatalf("CreateFromArtist() error = %v, want nil", err)
	}

	if result.PlaylistCreated {
		t.Errorf("existing playlist should be reused, not recreated: %+v", result)
	}
	if result.PlaylistID != "existing-queen" {
		t.Errorf("PlaylistID = %q, want %q", result.PlaylistID, "existing-queen")
	}
	if len(gw.createdPlaylists) != 0 {
		t.Errorf("CreatePlaylist should not have been called, got %d calls", len(gw.createdPlaylists))
	}
	if result.TracksAdded != 1 || result.TracksSkipped != 1 {
		t.Fatalf("got TracksAdded=%d TracksSkipped=%d, want 1 and 1", result.TracksAdded, result.TracksSkipped)
	}
}

func TestArtistPlaylistService_CreateFromArtist_NoMatchingTracks(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{
		{URI: "spotify:track:adele1", Artists: []domain.Artist{{ID: "a-adele", Name: "Adele"}}},
	}

	svc := newTestArtistPlaylistService(auth, profile, gw)
	_, err := svc.CreateFromArtist(context.Background(), validPlaylistLink, "a-queen", "Queen")
	if err == nil {
		t.Fatal("CreateFromArtist() error = nil, want an error when the artist has no tracks in the source playlist")
	}
	if len(gw.createdPlaylists) != 0 {
		t.Errorf("no playlist should have been created")
	}
}

func TestArtistPlaylistService_PropagatesAuthError(t *testing.T) {
	wantErr := errors.New("not authenticated")
	auth := &fakeAuthService{validErr: wantErr}
	svc := newTestArtistPlaylistService(auth, &fakeProfileService{}, newFakePlaylistGateway())

	if _, err := svc.ListArtists(context.Background(), validPlaylistLink); !errors.Is(err, wantErr) {
		t.Errorf("ListArtists() error = %v, want %v", err, wantErr)
	}
	if _, err := svc.CreateFromArtist(context.Background(), validPlaylistLink, "a1", "Artist"); !errors.Is(err, wantErr) {
		t.Errorf("CreateFromArtist() error = %v, want %v", err, wantErr)
	}
}

// TestArtistPlaylistService_CachesSourceTracksAcrossListAndCreate guards
// against a real performance regression: ListArtists and CreateFromArtist
// are always called back-to-back by the CLI's picker flow against the same
// source playlist, and originally each independently fetched the whole
// playlist — for a playlist with thousands of tracks that doubled an
// already slow operation. The gateway must only be asked once.
func TestArtistPlaylistService_CachesSourceTracksAcrossListAndCreate(t *testing.T) {
	auth := &fakeAuthService{validToken: domain.Token{AccessToken: "at"}}
	profile := &fakeProfileService{user: domain.User{ID: "me"}}
	gw := newFakePlaylistGateway()
	gw.tracks = []domain.Track{
		{URI: "spotify:track:1", Artists: []domain.Artist{{ID: "a-queen", Name: "Queen"}}},
	}

	svc := newTestArtistPlaylistService(auth, profile, gw)

	if _, err := svc.ListArtists(context.Background(), validPlaylistLink); err != nil {
		t.Fatalf("ListArtists() error = %v, want nil", err)
	}
	if _, err := svc.CreateFromArtist(context.Background(), validPlaylistLink, "a-queen", "Queen"); err != nil {
		t.Fatalf("CreateFromArtist() error = %v, want nil", err)
	}

	if gw.listTracksCalls != 1 {
		t.Errorf("ListTracks called %d times, want 1 (CreateFromArtist should reuse ListArtists' fetch)", gw.listTracksCalls)
	}
}
