package spotifyweb

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestProfileGateway(srv *httptest.Server) *ProfileGateway {
	g := New(nil)
	g.baseURL = srv.URL
	return g
}

func TestProfileGateway_CurrentUser_Success(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "u1",
			"display_name": "Someone",
			"email":        "someone@example.com",
			"product":      "premium",
		})
	}))
	defer srv.Close()

	g := newTestProfileGateway(srv)
	user, err := g.CurrentUser(context.Background(), "my-access-token")
	if err != nil {
		t.Fatalf("CurrentUser() error = %v, want nil", err)
	}
	if gotAuth != "Bearer my-access-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer my-access-token")
	}
	if gotPath != "/v1/me" {
		t.Errorf("request path = %q, want %q", gotPath, "/v1/me")
	}
	if user.ID != "u1" || user.DisplayName != "Someone" || user.Email != "someone@example.com" || user.Product != "premium" {
		t.Errorf("user = %+v, unexpected fields", user)
	}
}

func TestProfileGateway_CurrentUser_ErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"status": 401, "message": "The access token expired"},
		})
	}))
	defer srv.Close()

	g := newTestProfileGateway(srv)
	_, err := g.CurrentUser(context.Background(), "expired-token")
	if err == nil {
		t.Fatal("CurrentUser() error = nil, want an error")
	}
}
