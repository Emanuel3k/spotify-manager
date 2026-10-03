package spotifyweb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

func newTestPlaylistGateway(srv *httptest.Server) *PlaylistGateway {
	g := NewPlaylistGateway(nil)
	g.baseURL = srv.URL
	return g
}

func TestPlaylistGateway_ListTracks_PaginatesAndParsesReleaseYear(t *testing.T) {
	var calls int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")

		if n == 1 {
			fmt.Fprintf(w, `{
				"items": [
					{"item": {"id":"1","uri":"spotify:track:1","name":"Song A","is_local":false,
						"artists":[{"id":"artist-a","name":"Artist A"}],"album":{"release_date":"2020-05-01"}}},
					{"item": {"id":"2","uri":"spotify:track:2","name":"Song B","is_local":false,
						"artists":[{"id":"artist-b","name":"Artist B"}],"album":{"release_date":"1999"}}},
					{"item": null},
					{"item": {"id":"3","uri":"spotify:local:abc","name":"Local file","is_local":true,
						"artists":[],"album":{"release_date":""}}}
				],
				"next": %q
			}`, srv.URL+"/v1/playlists/pid/items?offset=100&limit=100")
			return
		}

		fmt.Fprint(w, `{
			"items": [
				{"item": {"id":"4","uri":"spotify:track:4","name":"Song D","is_local":false,
					"artists":[{"id":"artist-d1","name":"Artist D"},{"id":"artist-d2","name":"Feat. Artist"}],"album":{"release_date":"2020-12"}}}
			],
			"next": null
		}`)
	}))
	defer srv.Close()

	g := newTestPlaylistGateway(srv)
	tracks, err := g.ListTracks(context.Background(), "at", "pid")
	if err != nil {
		t.Fatalf("ListTracks() error = %v, want nil", err)
	}
	if calls != 2 {
		t.Fatalf("server got %d requests, want 2 (pagination followed)", calls)
	}

	// Null and local tracks must be skipped; 3 real tracks remain across both pages.
	if len(tracks) != 3 {
		t.Fatalf("got %d tracks, want 3 (nulls/local files skipped)", len(tracks))
	}

	byURI := map[string]domain.Track{}
	for _, tr := range tracks {
		byURI[tr.URI] = tr
	}
	if byURI["spotify:track:1"].ReleaseYear != 2020 {
		t.Errorf("track 1 ReleaseYear = %d, want 2020 (from YYYY-MM-DD)", byURI["spotify:track:1"].ReleaseYear)
	}
	if byURI["spotify:track:2"].ReleaseYear != 1999 {
		t.Errorf("track 2 ReleaseYear = %d, want 1999 (from YYYY)", byURI["spotify:track:2"].ReleaseYear)
	}
	if byURI["spotify:track:4"].ReleaseYear != 2020 {
		t.Errorf("track 4 ReleaseYear = %d, want 2020 (from YYYY-MM)", byURI["spotify:track:4"].ReleaseYear)
	}

	track4Artists := byURI["spotify:track:4"].Artists
	wantArtists := []domain.Artist{{ID: "artist-d1", Name: "Artist D"}, {ID: "artist-d2", Name: "Feat. Artist"}}
	if len(track4Artists) != len(wantArtists) || track4Artists[0] != wantArtists[0] || track4Artists[1] != wantArtists[1] {
		t.Errorf("track 4 Artists = %+v, want %+v", track4Artists, wantArtists)
	}
}

func TestPlaylistGateway_ListTrackURIs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[
			{"item":{"uri":"spotify:track:1","is_local":false}},
			{"item":{"uri":"spotify:track:2","is_local":true}},
			{"item":null}
		],"next":null}`)
	}))
	defer srv.Close()

	g := newTestPlaylistGateway(srv)
	uris, err := g.ListTrackURIs(context.Background(), "at", "pid")
	if err != nil {
		t.Fatalf("ListTrackURIs() error = %v, want nil", err)
	}
	if _, ok := uris["spotify:track:1"]; !ok || len(uris) != 1 {
		t.Errorf("uris = %v, want just {spotify:track:1} (local file and null track excluded)", uris)
	}
}

func TestPlaylistGateway_ListOwnPlaylists_FiltersByOwner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"items":[
			{"id":"p1","name":"2020","public":false,"owner":{"id":"me"}},
			{"id":"p2","name":"Someone else's","public":true,"owner":{"id":"someone-else"}}
		],"next":null}`)
	}))
	defer srv.Close()

	g := newTestPlaylistGateway(srv)
	playlists, err := g.ListOwnPlaylists(context.Background(), "at", "me")
	if err != nil {
		t.Fatalf("ListOwnPlaylists() error = %v, want nil", err)
	}
	if len(playlists) != 1 || playlists[0].ID != "p1" {
		t.Fatalf("playlists = %+v, want only p1 (owned by me)", playlists)
	}
}

func TestPlaylistGateway_CreatePlaylist_IsPrivateAndUsesMePlaylistsPath(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)

		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"new-id","name":"2020","public":false,"owner":{"id":"me"}}`)
	}))
	defer srv.Close()

	g := newTestPlaylistGateway(srv)
	playlist, err := g.CreatePlaylist(context.Background(), "at", "2020", false)
	if err != nil {
		t.Fatalf("CreatePlaylist() error = %v, want nil", err)
	}
	if gotPath != "/v1/me/playlists" {
		t.Errorf("request path = %q, want %q", gotPath, "/v1/me/playlists")
	}

	var reqBody createPlaylistRequest
	if err := json.Unmarshal([]byte(gotBody), &reqBody); err != nil {
		t.Fatalf("could not decode request body %q: %v", gotBody, err)
	}
	if reqBody.Public {
		t.Errorf("request body had public=true, want false (year playlists must be private)")
	}
	if reqBody.Name != "2020" {
		t.Errorf("request body name = %q, want %q", reqBody.Name, "2020")
	}

	if playlist.ID != "new-id" || playlist.OwnerID != "me" {
		t.Errorf("playlist = %+v, unexpected fields", playlist)
	}
}

func TestPlaylistGateway_AddTracks_ChunksAt100(t *testing.T) {
	var requestSizes []int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body addTracksRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("could not decode add-tracks request body: %v", err)
		}
		requestSizes = append(requestSizes, len(body.URIs))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	uris := make([]string, 150)
	for i := range uris {
		uris[i] = fmt.Sprintf("spotify:track:%d", i)
	}

	g := newTestPlaylistGateway(srv)
	if err := g.AddTracks(context.Background(), "at", "pid", uris); err != nil {
		t.Fatalf("AddTracks() error = %v, want nil", err)
	}

	if len(requestSizes) != 2 {
		t.Fatalf("got %d requests, want 2 (150 uris chunked at 100)", len(requestSizes))
	}
	if requestSizes[0] != 100 || requestSizes[1] != 50 {
		t.Errorf("request chunk sizes = %v, want [100 50]", requestSizes)
	}
}

func TestPlaylistGateway_AddTracks_NoRequestWhenEmpty(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()

	g := newTestPlaylistGateway(srv)
	if err := g.AddTracks(context.Background(), "at", "pid", nil); err != nil {
		t.Fatalf("AddTracks() error = %v, want nil", err)
	}
	if calls != 0 {
		t.Errorf("server got %d requests, want 0 for an empty track list", calls)
	}
}

func TestParseReleaseYear(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"2020-05-01", 2020},
		{"2020-05", 2020},
		{"2020", 2020},
		{"", 0},
		{"abcd", 0},
		{"9", 0},
	}
	for _, tt := range tests {
		if got := parseReleaseYear(tt.in); got != tt.want {
			t.Errorf("parseReleaseYear(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
