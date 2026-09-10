package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
	"github.com/Emanuel3k/spotify-manager/internal/core/port/out"
	"github.com/Emanuel3k/spotify-manager/internal/core/service"
)

// newTestAuthService wires an AuthService with fakes the test can inspect
// and mutate; cb.fn correlates the state generated inside Login (via
// gw.gotState, only known once BuildAuthorizeURL has run) unless the test
// overrides it.
func newTestAuthService(gw *fakeAuthGateway, repo *fakeTokenRepo, browser *fakeBrowser, cb *fakeCallback) *service.AuthService {
	return service.NewAuthService(gw, repo, browser, cb, []string{"scope-a", "scope-b"}, nil)
}

func TestAuthService_Login_Success(t *testing.T) {
	wantToken := domain.Token{AccessToken: "at", RefreshToken: "rt", Scope: "scope-a scope-b", ExpiresAt: time.Now().Add(time.Hour)}
	gw := &fakeAuthGateway{authURL: "https://accounts.spotify.com/authorize?mock=1", exchangeToken: wantToken}
	repo := &fakeTokenRepo{}
	browser := &fakeBrowser{}
	cb := &fakeCallback{}
	cb.fn = func(context.Context) (out.CallbackResult, error) {
		return out.CallbackResult{Code: "auth-code", State: gw.gotState}, nil
	}

	svc := newTestAuthService(gw, repo, browser, cb)

	var gotURL string
	token, err := svc.Login(context.Background(), func(u string) { gotURL = u })
	if err != nil {
		t.Fatalf("Login() error = %v, want nil", err)
	}
	if token != wantToken {
		t.Fatalf("Login() token = %+v, want %+v", token, wantToken)
	}
	if gotURL != gw.authURL {
		t.Errorf("onAuthURL callback got %q, want %q", gotURL, gw.authURL)
	}
	if browser.openedURL != gw.authURL {
		t.Errorf("browser.Open called with %q, want %q", browser.openedURL, gw.authURL)
	}
	if gw.gotCode != "auth-code" {
		t.Errorf("ExchangeCode called with code %q, want %q", gw.gotCode, "auth-code")
	}
	if !repo.hasToken || repo.token != wantToken {
		t.Errorf("token not persisted correctly, got %+v", repo.token)
	}
}

func TestAuthService_Login_AccessDenied(t *testing.T) {
	gw := &fakeAuthGateway{authURL: "https://accounts.spotify.com/authorize"}
	repo := &fakeTokenRepo{}
	cb := &fakeCallback{fn: func(context.Context) (out.CallbackResult, error) {
		return out.CallbackResult{Error: "access_denied"}, nil
	}}

	svc := newTestAuthService(gw, repo, &fakeBrowser{}, cb)

	_, err := svc.Login(context.Background(), nil)
	if !errors.Is(err, domain.ErrAuthorizationDenied) {
		t.Fatalf("Login() error = %v, want ErrAuthorizationDenied", err)
	}
	if repo.hasToken {
		t.Errorf("token should not be persisted when authorization is denied")
	}
}

func TestAuthService_Login_StateMismatch(t *testing.T) {
	gw := &fakeAuthGateway{authURL: "https://accounts.spotify.com/authorize"}
	cb := &fakeCallback{fn: func(context.Context) (out.CallbackResult, error) {
		return out.CallbackResult{Code: "code", State: "not-the-real-state"}, nil
	}}

	svc := newTestAuthService(gw, &fakeTokenRepo{}, &fakeBrowser{}, cb)

	_, err := svc.Login(context.Background(), nil)
	if !errors.Is(err, domain.ErrStateMismatch) {
		t.Fatalf("Login() error = %v, want ErrStateMismatch", err)
	}
}

func TestAuthService_Login_ExchangeError(t *testing.T) {
	wantErr := errors.New("boom")
	gw := &fakeAuthGateway{authURL: "https://accounts.spotify.com/authorize", exchangeErr: wantErr}
	repo := &fakeTokenRepo{}
	cb := &fakeCallback{fn: func(context.Context) (out.CallbackResult, error) {
		return out.CallbackResult{Code: "code", State: gw.gotState}, nil
	}}

	svc := newTestAuthService(gw, repo, &fakeBrowser{}, cb)

	_, err := svc.Login(context.Background(), nil)
	if !errors.Is(err, wantErr) {
		t.Fatalf("Login() error = %v, want wrapped %v", err, wantErr)
	}
	if repo.hasToken {
		t.Errorf("token should not be persisted when exchange fails")
	}
}

func TestAuthService_Logout(t *testing.T) {
	repo := &fakeTokenRepo{hasToken: true, token: domain.Token{AccessToken: "at"}}
	svc := newTestAuthService(&fakeAuthGateway{}, repo, &fakeBrowser{}, &fakeCallback{})

	if err := svc.Logout(context.Background()); err != nil {
		t.Fatalf("Logout() error = %v, want nil", err)
	}
	if !repo.deleted || repo.hasToken {
		t.Errorf("Logout() did not delete the stored token")
	}
}

func TestAuthService_Status(t *testing.T) {
	tests := []struct {
		name string
		repo *fakeTokenRepo
		want bool
	}{
		{"not authenticated", &fakeTokenRepo{}, false},
		{"valid token", &fakeTokenRepo{hasToken: true, token: domain.Token{AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour)}}, true},
		{"expired, no refresh token", &fakeTokenRepo{hasToken: true, token: domain.Token{AccessToken: "at", ExpiresAt: time.Now().Add(-time.Hour)}}, false},
		{"expired, has refresh token", &fakeTokenRepo{hasToken: true, token: domain.Token{AccessToken: "at", RefreshToken: "rt", ExpiresAt: time.Now().Add(-time.Hour)}}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newTestAuthService(&fakeAuthGateway{}, tt.repo, &fakeBrowser{}, &fakeCallback{})
			status, err := svc.Status(context.Background())
			if err != nil {
				t.Fatalf("Status() error = %v, want nil", err)
			}
			if status.Authenticated != tt.want {
				t.Errorf("Status().Authenticated = %v, want %v", status.Authenticated, tt.want)
			}
		})
	}
}

func TestAuthService_ValidToken(t *testing.T) {
	t.Run("not authenticated", func(t *testing.T) {
		svc := newTestAuthService(&fakeAuthGateway{}, &fakeTokenRepo{}, &fakeBrowser{}, &fakeCallback{})
		_, err := svc.ValidToken(context.Background())
		if !errors.Is(err, domain.ErrNotAuthenticated) {
			t.Fatalf("ValidToken() error = %v, want ErrNotAuthenticated", err)
		}
	})

	t.Run("still valid, no refresh call", func(t *testing.T) {
		valid := domain.Token{AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour)}
		gw := &fakeAuthGateway{}
		repo := &fakeTokenRepo{hasToken: true, token: valid}
		svc := newTestAuthService(gw, repo, &fakeBrowser{}, &fakeCallback{})

		got, err := svc.ValidToken(context.Background())
		if err != nil {
			t.Fatalf("ValidToken() error = %v, want nil", err)
		}
		if got != valid {
			t.Errorf("ValidToken() = %+v, want %+v", got, valid)
		}
		if gw.gotRefresh != "" {
			t.Errorf("RefreshToken should not have been called")
		}
	})

	t.Run("expired without refresh token", func(t *testing.T) {
		expired := domain.Token{AccessToken: "at", ExpiresAt: time.Now().Add(-time.Hour)}
		svc := newTestAuthService(&fakeAuthGateway{}, &fakeTokenRepo{hasToken: true, token: expired}, &fakeBrowser{}, &fakeCallback{})

		_, err := svc.ValidToken(context.Background())
		if !errors.Is(err, domain.ErrNotAuthenticated) {
			t.Fatalf("ValidToken() error = %v, want ErrNotAuthenticated", err)
		}
	})

	t.Run("expired, refreshes and persists", func(t *testing.T) {
		expired := domain.Token{AccessToken: "old-at", RefreshToken: "rt", ExpiresAt: time.Now().Add(-time.Hour)}
		refreshed := domain.Token{AccessToken: "new-at", ExpiresAt: time.Now().Add(time.Hour)} // no new refresh token
		gw := &fakeAuthGateway{refreshToken: refreshed}
		repo := &fakeTokenRepo{hasToken: true, token: expired}
		svc := newTestAuthService(gw, repo, &fakeBrowser{}, &fakeCallback{})

		got, err := svc.ValidToken(context.Background())
		if err != nil {
			t.Fatalf("ValidToken() error = %v, want nil", err)
		}
		if got.AccessToken != "new-at" {
			t.Errorf("ValidToken().AccessToken = %q, want %q", got.AccessToken, "new-at")
		}
		if got.RefreshToken != "rt" {
			t.Errorf("ValidToken().RefreshToken = %q, want original %q preserved", got.RefreshToken, "rt")
		}
		if gw.gotRefresh != "rt" {
			t.Errorf("RefreshToken called with %q, want %q", gw.gotRefresh, "rt")
		}
		if !repo.hasToken || repo.token.AccessToken != "new-at" {
			t.Errorf("refreshed token was not persisted, got %+v", repo.token)
		}
	})

	t.Run("refresh error propagates", func(t *testing.T) {
		wantErr := errors.New("refresh failed")
		expired := domain.Token{AccessToken: "at", RefreshToken: "rt", ExpiresAt: time.Now().Add(-time.Hour)}
		gw := &fakeAuthGateway{refreshErr: wantErr}
		svc := newTestAuthService(gw, &fakeTokenRepo{hasToken: true, token: expired}, &fakeBrowser{}, &fakeCallback{})

		_, err := svc.ValidToken(context.Background())
		if !errors.Is(err, wantErr) {
			t.Fatalf("ValidToken() error = %v, want wrapped %v", err, wantErr)
		}
	})
}
