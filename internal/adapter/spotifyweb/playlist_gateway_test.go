package spotifyweb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
)

func newTestPlaylistGateway(srv *httptest.Server) *PlaylistGateway {
	g := NewPlaylistGateway(nil)
	g.baseURL = srv.URL
	return g
}

// fakeArtist/fakeItem/fakeEntry mirror the JSON shape the real Web API
// returns for GET /playlists/{id}/items, for building test fixtures.
type fakeArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type fakeItem struct {
	ID      string       `json:"id"`
	URI     string       `json:"uri"`
	Name    string       `json:"name"`
	IsLocal bool         `json:"is_local"`
	Artists []fakeArtist `json:"artists"`
	Album   struct {
		ReleaseDate string `json:"release_date"`
	} `json:"album"`
}

type fakeEntry struct {
	Item *fakeItem `json:"item"`
}

// TestPlaylistGateway_ListTracks_PaginatesConcurrentlyAndParsesFields uses a
// playlist large enough to span 3 pages (at the gateway's 100-per-page
// size) to exercise the concurrent, offset-based pagination added to fix a
// real large-playlist performance issue (a 2000+ track playlist made the
// old sequential "follow next" pagination feel like a hang). It also checks
// that concurrently-fetched pages are reassembled in the correct order.
func TestPlaylistGateway_ListTracks_PaginatesConcurrentlyAndParsesFields(t *testing.T) {
	const total = 250 // forces 3 pages: 100 + 100 + 50

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		q := r.URL.Query()
		offset, _ := strconv.Atoi(q.Get("offset"))
		limit, _ := strconv.Atoi(q.Get("limit"))

		end := offset + limit
		if end > total {
			end = total
		}

		items := make([]fakeEntry, 0, end-offset)
		for i := offset; i < end; i++ {
			switch i {
			case 1:
				items = append(items, fakeEntry{}) // removed track: null item
			case 2:
				items = append(items, fakeEntry{Item: &fakeItem{ID: "local", URI: "spotify:local:abc", Name: "Local file", IsLocal: true}})
			default:
				item := &fakeItem{
					ID:   fmt.Sprintf("id-%d", i),
					URI:  fmt.Sprintf("spotify:track:%d", i),
					Name: fmt.Sprintf("Song %d", i),
					Artists: []fakeArtist{
						{ID: fmt.Sprintf("artist-%d", i%5), Name: fmt.Sprintf("Artist %d", i%5)},
					},
				}
				item.Album.ReleaseDate = "2020-01-01"
				items = append(items, fakeEntry{Item: item})
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"total": total, "items": items})
	}))
	defer srv.Close()

	g := newTestPlaylistGateway(srv)
	tracks, err := g.ListTracks(context.Background(), "at", "pid")
	if err != nil {
		t.Fatalf("ListTracks() error = %v, want nil", err)
	}

	if calls != 3 {
		t.Fatalf("server got %d requests, want 3 (250 items at 100/page)", calls)
	}

	wantCount := total - 2 // one null item, one local file, both excluded
	if len(tracks) != wantCount {
		t.Fatalf("got %d tracks, want %d (null/local entries skipped)", len(tracks), wantCount)
	}

	// Pages are fetched concurrently but must be reassembled in offset
	// order: track 0 must still be first and the final offset's track last.
	if tracks[0].URI != "spotify:track:0" {
		t.Errorf("tracks[0].URI = %q, want %q (pages must be reassembled in order)", tracks[0].URI, "spotify:track:0")
	}
	last := tracks[len(tracks)-1]
	wantLastURI := fmt.Sprintf("spotify:track:%d", total-1)
	if last.URI != wantLastURI {
		t.Errorf("last track URI = %q, want %q (order broken)", last.URI, wantLastURI)
	}
	if last.ReleaseYear != 2020 {
		t.Errorf("last track ReleaseYear = %d, want 2020", last.ReleaseYear)
	}
	if len(last.Artists) != 1 || last.Artists[0].Name == "" {
		t.Errorf("last track Artists = %+v, want one populated artist", last.Artists)
	}
}

func TestPlaylistGateway_ListTrackURIs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"total":3,"items":[
			{"item":{"uri":"spotify:track:1","is_local":false}},
			{"item":{"uri":"spotify:track:2","is_local":true}},
			{"item":null}
		]}`)
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
