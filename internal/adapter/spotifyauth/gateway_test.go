package spotifyauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// newTestGateway builds a Gateway pointed at an httptest.Server instead of
// the real Spotify endpoints. White-box (same package) so it can set the
// unexported url fields directly, without adding test-only exports to the
// public API.
func newTestGateway(t *testing.T, srv *httptest.Server) *Gateway {
	t.Helper()
	g := New("client-id", "client-secret", "http://127.0.0.1:8080/callback", nil)
	g.tokenURL = srv.URL
	g.authorizeURL = srv.URL + "/authorize"
	return g
}

func TestGateway_BuildAuthorizeURL(t *testing.T) {
	g := New("my-client-id", "secret", "http://127.0.0.1:8080/callback", nil)

	got := g.BuildAuthorizeURL("the-state", []string{"scope-a", "scope-b"})

	u, err := url.Parse(got)
	if err != nil {
		t.Fatalf("BuildAuthorizeURL returned an invalid URL: %v", err)
	}
	q := u.Query()
	if q.Get("client_id") != "my-client-id" {
		t.Errorf("client_id = %q, want %q", q.Get("client_id"), "my-client-id")
	}
	if q.Get("response_type") != "code" {
		t.Errorf("response_type = %q, want %q", q.Get("response_type"), "code")
	}
	if q.Get("redirect_uri") != "http://127.0.0.1:8080/callback" {
		t.Errorf("redirect_uri = %q, want %q", q.Get("redirect_uri"), "http://127.0.0.1:8080/callback")
	}
	if q.Get("state") != "the-state" {
		t.Errorf("state = %q, want %q", q.Get("state"), "the-state")
	}
	if q.Get("scope") != "scope-a scope-b" {
		t.Errorf("scope = %q, want %q", q.Get("scope"), "scope-a scope-b")
	}
}

func TestGateway_ExchangeCode_Success(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "AT",
			"token_type":    "Bearer",
			"scope":         "user-read-email",
			"expires_in":    3600,
			"refresh_token": "RT",
		})
	}))
	defer srv.Close()

	g := newTestGateway(t, srv)
	before := time.Now()

	token, err := g.ExchangeCode(context.Background(), "the-code")
	if err != nil {
		t.Fatalf("ExchangeCode() error = %v, want nil", err)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("client-id:client-secret"))
	if gotAuth != wantAuth {
		t.Errorf("Authorization header = %q, want %q", gotAuth, wantAuth)
	}
	if !strings.Contains(gotBody, "grant_type=authorization_code") || !strings.Contains(gotBody, "code=the-code") {
		t.Errorf("request body = %q, missing expected form fields", gotBody)
	}

	if token.AccessToken != "AT" || token.RefreshToken != "RT" || token.Scope != "user-read-email" {
		t.Errorf("token = %+v, unexpected fields", token)
	}
	if token.ExpiresAt.Before(before.Add(3599*time.Second)) || token.ExpiresAt.After(before.Add(3601*time.Second)) {
		t.Errorf("ExpiresAt = %v, want roughly %v", token.ExpiresAt, before.Add(3600*time.Second))
	}
}

func TestGateway_RefreshToken_SendsRefreshGrant(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "NEWAT", "expires_in": 3600})
	}))
	defer srv.Close()

	g := newTestGateway(t, srv)
	token, err := g.RefreshToken(context.Background(), "the-refresh-token")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v, want nil", err)
	}
	if token.AccessToken != "NEWAT" {
		t.Errorf("AccessToken = %q, want %q", token.AccessToken, "NEWAT")
	}
	if !strings.Contains(gotBody, "grant_type=refresh_token") || !strings.Contains(gotBody, "refresh_token=the-refresh-token") {
		t.Errorf("request body = %q, missing expected form fields", gotBody)
	}
}

func TestGateway_TokenEndpoint_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":             "invalid_grant",
			"error_description": "Invalid authorization code",
		})
	}))
	defer srv.Close()

	g := newTestGateway(t, srv)
	_, err := g.ExchangeCode(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("ExchangeCode() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error = %v, want it to mention invalid_grant", err)
	}
}
