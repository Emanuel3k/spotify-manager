package spotifyweb

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"sync"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// maxTracksPerAddRequest is the Web API's hard limit on how many track URIs
// a single POST /playlists/{id}/items call may carry.
const maxTracksPerAddRequest = 100

// playlistItemsPageSize is how many items each GET /playlists/{id}/items
// request fetches at once (the Web API's own max for this endpoint).
const playlistItemsPageSize = 100

// maxConcurrentPageFetches bounds how many playlist-item pages are fetched
// in parallel once the total is known. Large playlists (seen: 2000+ tracks,
// 20+ pages) were previously fetched one page at a time, which made this
// feature feel like it hung; a handful of concurrent requests is a large
// speedup without hammering Spotify's rate limits.
const maxConcurrentPageFetches = 6

// PlaylistGateway implements out.PlaylistGateway via the Web API's
// /v1/playlists, /v1/me/playlists and /v1/me/playlists (create) endpoints.
//
// As of Spotify's February 2026 Developer Access changes, Development Mode
// apps use a reduced/renamed endpoint set: playlist item endpoints moved
// from "/tracks" to "/items" (with each item's "track" key renamed to
// "item"), and creating a playlist moved from POST /users/{id}/playlists to
// POST /me/playlists. See the migration guide:
// https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide
//
// Also per that guide, GET /playlists/{id}/items only returns track data
// for playlists the current user owns or collaborates on; for anything
// else it comes back with no items.
type PlaylistGateway struct {
	client
}

var _ out.PlaylistGateway = (*PlaylistGateway)(nil)

// NewPlaylistGateway builds a PlaylistGateway. logger may be nil, in which
// case log output is discarded.
func NewPlaylistGateway(logger *slog.Logger) *PlaylistGateway {
	return &PlaylistGateway{client: newClient(logger)}
}

// playlistItemEntry mirrors one element of GET /v1/playlists/{id}/items's
// "items" array. Item is a pointer because Spotify returns null for
// tracks/episodes that have been removed since the playlist was indexed.
type playlistItemEntry struct {
	Item *struct {
		ID      string `json:"id"`
		URI     string `json:"uri"`
		Name    string `json:"name"`
		IsLocal bool   `json:"is_local"`
		Artists []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"artists"`
		Album struct {
			ReleaseDate string `json:"release_date"`
		} `json:"album"`
	} `json:"item"`
}

type playlistItemsPage struct {
	Items []playlistItemEntry `json:"items"`
	Total *int                `json:"total"`
}

// fetchPlaylistItems pages through GET /v1/playlists/{id}/items. The first
// page tells us the total item count, so every remaining page's offset is
// known up front and fetched concurrently (bounded by
// maxConcurrentPageFetches) instead of waiting on each page's "next" link
// one at a time — large playlists (thousands of tracks, 20+ pages) used to
// make this feel like a hang. fields restricts the response to just what
// the caller needs (ListTracks wants names/artists/release dates,
// ListTrackURIs only needs the URI). Spotify only returns item data here
// for playlists the current user owns or collaborates on; for any other
// playlist the result is an empty (but successful) page.
func (g *PlaylistGateway) fetchPlaylistItems(ctx context.Context, accessToken, playlistID, fields string) ([]playlistItemEntry, error) {
	basePath := fmt.Sprintf("/v1/playlists/%s/items", url.PathEscape(playlistID))
	requestFields := "total," + fields

	fetchPage := func(offset int) (playlistItemsPage, error) {
		values := url.Values{}
		values.Set("limit", strconv.Itoa(playlistItemsPageSize))
		values.Set("offset", strconv.Itoa(offset))
		values.Set("fields", requestFields)

		var page playlistItemsPage
		err := g.do(ctx, "list_playlist_items", http.MethodGet, basePath+"?"+values.Encode(), accessToken, nil, &page)
		return page, err
	}

	first, err := fetchPage(0)
	if err != nil {
		return nil, err
	}

	total := len(first.Items)
	if first.Total != nil {
		total = *first.Total
	}
	numPages := 1
	if total > playlistItemsPageSize {
		numPages = (total + playlistItemsPageSize - 1) / playlistItemsPageSize
	}

	pages := make([][]playlistItemEntry, numPages)
	pages[0] = first.Items

	if numPages > 1 {
		g.log.Debug("list_playlist_items: fetching remaining pages concurrently",
			"playlist_id", playlistID, "total", total, "pages", numPages, "concurrency", maxConcurrentPageFetches)

		var wg sync.WaitGroup
		sem := make(chan struct{}, maxConcurrentPageFetches)
		errCh := make(chan error, numPages-1)

		for p := 1; p < numPages; p++ {
			wg.Add(1)
			go func(p int) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				page, err := fetchPage(p * playlistItemsPageSize)
				if err != nil {
					errCh <- err
					return
				}
				pages[p] = page.Items
			}(p)
		}

		wg.Wait()
		close(errCh)
		if pageErr, ok := <-errCh; ok {
			return nil, pageErr
		}
	}

	all := make([]playlistItemEntry, 0, total)
	for _, p := range pages {
		all = append(all, p...)
	}
	return all, nil
}

func (g *PlaylistGateway) ListTracks(ctx context.Context, accessToken, playlistID string) ([]domain.Track, error) {
	items, err := g.fetchPlaylistItems(ctx, accessToken, playlistID,
		"items(item(id,uri,name,is_local,artists(id,name),album(release_date)))")
	if err != nil {
		return nil, err
	}

	tracks := make([]domain.Track, 0, len(items))
	for _, entry := range items {
		if entry.Item == nil || entry.Item.IsLocal || entry.Item.URI == "" {
			continue
		}
		artists := make([]domain.Artist, 0, len(entry.Item.Artists))
		for _, a := range entry.Item.Artists {
			artists = append(artists, domain.Artist{ID: a.ID, Name: a.Name})
		}
		tracks = append(tracks, domain.Track{
			URI:         entry.Item.URI,
			ID:          entry.Item.ID,
			Name:        entry.Item.Name,
			Artists:     artists,
			ReleaseYear: parseReleaseYear(entry.Item.Album.ReleaseDate),
		})
	}
	return tracks, nil
}

func (g *PlaylistGateway) ListTrackURIs(ctx context.Context, accessToken, playlistID string) (map[string]struct{}, error) {
	items, err := g.fetchPlaylistItems(ctx, accessToken, playlistID, "items(item(uri,is_local))")
	if err != nil {
		return nil, err
	}

	uris := make(map[string]struct{}, len(items))
	for _, entry := range items {
		if entry.Item == nil || entry.Item.IsLocal || entry.Item.URI == "" {
			continue
		}
		uris[entry.Item.URI] = struct{}{}
	}
	return uris, nil
}

type ownPlaylistsPage struct {
	Items []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Public *bool  `json:"public"`
		Owner  struct {
			ID string `json:"id"`
		} `json:"owner"`
	} `json:"items"`
	Next *string `json:"next"`
}

func (g *PlaylistGateway) ListOwnPlaylists(ctx context.Context, accessToken, ownerID string) ([]domain.Playlist, error) {
	path := "/v1/me/playlists?limit=50"

	var all []domain.Playlist
	for path != "" {
		var page ownPlaylistsPage
		if err := g.do(ctx, "list_own_playlists", http.MethodGet, path, accessToken, nil, &page); err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if item.Owner.ID != ownerID {
				continue
			}
			public := item.Public != nil && *item.Public
			all = append(all, domain.Playlist{ID: item.ID, Name: item.Name, OwnerID: item.Owner.ID, Public: public})
		}

		if page.Next == nil || *page.Next == "" {
			break
		}
		next, err := url.Parse(*page.Next)
		if err != nil {
			return nil, fmt.Errorf("parse next page url: %w", err)
		}
		path = next.Path
		if next.RawQuery != "" {
			path += "?" + next.RawQuery
		}
	}
	return all, nil
}

type createPlaylistRequest struct {
	Name        string `json:"name"`
	Public      bool   `json:"public"`
	Description string `json:"description,omitempty"`
}

type createPlaylistResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Public bool   `json:"public"`
	Owner  struct {
		ID string `json:"id"`
	} `json:"owner"`
}

// CreatePlaylist creates a playlist for the currently authenticated user
// via POST /v1/me/playlists (the pre-Feb-2026 POST /v1/users/{id}/playlists
// endpoint was removed for Development Mode apps).
//
// Known Spotify platform issue (observed Sep 2026): POST /v1/me/playlists
// currently ignores the requested "public" value and always creates the
// playlist as public, even though the request body is correct. As a
// best-effort workaround, when that happens we immediately issue a follow-up
// PUT /v1/playlists/{id} to set the intended visibility. This sometimes
// still doesn't take effect on Spotify's side (also observed), so callers
// should treat the returned Playlist.Public as the true last-known state,
// not a guarantee, and a warning is logged whenever the requested
// visibility could not be confirmed.
func (g *PlaylistGateway) CreatePlaylist(ctx context.Context, accessToken, name string, public bool) (domain.Playlist, error) {
	body, err := json.Marshal(createPlaylistRequest{
		Name:        name,
		Public:      public,
		Description: "Created automatically by spotify-manager (split by year).",
	})
	if err != nil {
		return domain.Playlist{}, fmt.Errorf("encode create playlist request: %w", err)
	}

	var resp createPlaylistResponse
	if err := g.do(ctx, "create_playlist", http.MethodPost, "/v1/me/playlists", accessToken, bytes.NewReader(body), &resp); err != nil {
		return domain.Playlist{}, err
	}

	playlist := domain.Playlist{ID: resp.ID, Name: resp.Name, OwnerID: resp.Owner.ID, Public: resp.Public}

	if resp.Public != public {
		g.log.Warn("create_playlist: spotify ignored the requested visibility, attempting a follow-up fix",
			"playlist_id", resp.ID, "requested_public", public, "actual_public", resp.Public)

		if fixErr := g.setPlaylistPublic(ctx, accessToken, resp.ID, public); fixErr != nil {
			g.log.Warn("create_playlist: follow-up visibility fix failed", "playlist_id", resp.ID, "error", fixErr)
		} else {
			g.log.Warn("create_playlist: sent follow-up visibility fix; Spotify does not always apply it immediately, verify manually if this matters",
				"playlist_id", resp.ID)
		}
	}

	return playlist, nil
}

// setPlaylistPublic issues PUT /v1/playlists/{id} to change just the
// playlist's public/private flag.
func (g *PlaylistGateway) setPlaylistPublic(ctx context.Context, accessToken, playlistID string, public bool) error {
	body, err := json.Marshal(map[string]bool{"public": public})
	if err != nil {
		return fmt.Errorf("encode update playlist details request: %w", err)
	}
	path := fmt.Sprintf("/v1/playlists/%s", url.PathEscape(playlistID))
	return g.do(ctx, "update_playlist_details", http.MethodPut, path, accessToken, bytes.NewReader(body), nil)
}

type addTracksRequest struct {
	URIs []string `json:"uris"`
}

func (g *PlaylistGateway) AddTracks(ctx context.Context, accessToken, playlistID string, trackURIs []string) error {
	path := fmt.Sprintf("/v1/playlists/%s/items", url.PathEscape(playlistID))

	for start := 0; start < len(trackURIs); start += maxTracksPerAddRequest {
		end := min(start+maxTracksPerAddRequest, len(trackURIs))

		body, err := json.Marshal(addTracksRequest{URIs: trackURIs[start:end]})
		if err != nil {
			return fmt.Errorf("encode add tracks request: %w", err)
		}
		if err := g.do(ctx, "add_tracks", http.MethodPost, path, accessToken, bytes.NewReader(body), nil); err != nil {
			return err
		}
	}
	return nil
}

// parseReleaseYear reads the leading 4-digit year out of a Spotify
// release_date, which may be "YYYY", "YYYY-MM" or "YYYY-MM-DD" depending on
// the album's release_date_precision. Returns 0 when unparseable.
func parseReleaseYear(releaseDate string) int {
	if len(releaseDate) < 4 {
		return 0
	}
	year, err := strconv.Atoi(releaseDate[:4])
	if err != nil {
		return 0
	}
	return year
}
