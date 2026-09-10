package service_test

import (
	"context"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/in"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
)

// -- out.SpotifyAuthGateway -------------------------------------------------

type fakeAuthGateway struct {
	authURL   string
	gotState  string
	gotScopes []string

	exchangeToken domain.Token
	exchangeErr   error
	gotCode       string

	refreshToken domain.Token
	refreshErr   error
	gotRefresh   string
}

func (f *fakeAuthGateway) BuildAuthorizeURL(state string, scopes []string) string {
	f.gotState = state
	f.gotScopes = scopes
	return f.authURL
}

func (f *fakeAuthGateway) ExchangeCode(_ context.Context, code string) (domain.Token, error) {
	f.gotCode = code
	return f.exchangeToken, f.exchangeErr
}

func (f *fakeAuthGateway) RefreshToken(_ context.Context, refreshToken string) (domain.Token, error) {
	f.gotRefresh = refreshToken
	return f.refreshToken, f.refreshErr
}

// -- out.TokenRepository -----------------------------------------------------

type fakeTokenRepo struct {
	token    domain.Token
	hasToken bool

	saveErr   error
	loadErr   error
	deleteErr error

	saved   []domain.Token
	deleted bool
}

func (f *fakeTokenRepo) Save(_ context.Context, token domain.Token) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.token = token
	f.hasToken = true
	f.saved = append(f.saved, token)
	return nil
}

func (f *fakeTokenRepo) Load(_ context.Context) (domain.Token, error) {
	if f.loadErr != nil {
		return domain.Token{}, f.loadErr
	}
	if !f.hasToken {
		return domain.Token{}, domain.ErrNotAuthenticated
	}
	return f.token, nil
}

func (f *fakeTokenRepo) Delete(_ context.Context) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.hasToken = false
	f.deleted = true
	return nil
}

// -- out.BrowserOpener --------------------------------------------------------

type fakeBrowser struct {
	openedURL string
	err       error
}

func (f *fakeBrowser) Open(url string) error {
	f.openedURL = url
	return f.err
}

// -- out.CallbackListener ------------------------------------------------------

type fakeCallback struct {
	fn func(ctx context.Context) (out.CallbackResult, error)
}

func (f *fakeCallback) Await(ctx context.Context) (out.CallbackResult, error) {
	return f.fn(ctx)
}

// -- in.AuthService (stub, for services that only need ValidToken) -----------

type fakeAuthService struct {
	validToken domain.Token
	validErr   error
}

func (f *fakeAuthService) Login(context.Context, func(string)) (domain.Token, error) {
	return domain.Token{}, nil
}
func (f *fakeAuthService) Logout(context.Context) error              { return nil }
func (f *fakeAuthService) Status(context.Context) (in.Status, error) { return in.Status{}, nil }
func (f *fakeAuthService) ValidToken(context.Context) (domain.Token, error) {
	return f.validToken, f.validErr
}

// -- out.ProfileGateway --------------------------------------------------------

type fakeProfileGateway struct {
	user     domain.User
	err      error
	gotToken string
}

func (f *fakeProfileGateway) CurrentUser(_ context.Context, accessToken string) (domain.User, error) {
	f.gotToken = accessToken
	return f.user, f.err
}

// -- in.ProfileService (stub, for services that only need Me) ----------------

type fakeProfileService struct {
	user domain.User
	err  error
}

func (f *fakeProfileService) Me(context.Context) (domain.User, error) {
	return f.user, f.err
}

// -- out.PlaylistGateway --------------------------------------------------------

type fakePlaylistGateway struct {
	tracks        []domain.Track
	listTracksErr error

	trackURIs        map[string]map[string]struct{} // playlistID -> uris
	listTrackURIsErr error

	ownPlaylists []domain.Playlist
	listOwnErr   error

	createdPlaylists []domain.Playlist // records CreatePlaylist calls, in order
	createErr        error
	nextCreatedID    int

	addedTracks map[string][]string // playlistID -> uris added, across calls
	addErr      error
}

func newFakePlaylistGateway() *fakePlaylistGateway {
	return &fakePlaylistGateway{
		trackURIs:   map[string]map[string]struct{}{},
		addedTracks: map[string][]string{},
	}
}

func (f *fakePlaylistGateway) ListTracks(_ context.Context, _, _ string) ([]domain.Track, error) {
	return f.tracks, f.listTracksErr
}

func (f *fakePlaylistGateway) ListTrackURIs(_ context.Context, _, playlistID string) (map[string]struct{}, error) {
	if f.listTrackURIsErr != nil {
		return nil, f.listTrackURIsErr
	}
	existing := f.trackURIs[playlistID]
	out := make(map[string]struct{}, len(existing))
	for uri := range existing {
		out[uri] = struct{}{}
	}
	return out, nil
}

func (f *fakePlaylistGateway) ListOwnPlaylists(_ context.Context, _, _ string) ([]domain.Playlist, error) {
	return f.ownPlaylists, f.listOwnErr
}

func (f *fakePlaylistGateway) CreatePlaylist(_ context.Context, _, name string, public bool) (domain.Playlist, error) {
	if f.createErr != nil {
		return domain.Playlist{}, f.createErr
	}
	f.nextCreatedID++
	p := domain.Playlist{ID: "created-" + name, Name: name, OwnerID: "me", Public: public}
	f.createdPlaylists = append(f.createdPlaylists, p)
	if f.trackURIs[p.ID] == nil {
		f.trackURIs[p.ID] = map[string]struct{}{}
	}
	return p, nil
}

func (f *fakePlaylistGateway) AddTracks(_ context.Context, _, playlistID string, trackURIs []string) error {
	if f.addErr != nil {
		return f.addErr
	}
	f.addedTracks[playlistID] = append(f.addedTracks[playlistID], trackURIs...)
	if f.trackURIs[playlistID] == nil {
		f.trackURIs[playlistID] = map[string]struct{}{}
	}
	for _, uri := range trackURIs {
		f.trackURIs[playlistID][uri] = struct{}{}
	}
	return nil
}
