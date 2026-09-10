package domain_test

import (
	"testing"
	"time"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

func TestToken_IsExpired(t *testing.T) {
	tests := []struct {
		name  string
		token domain.Token
		want  bool
	}{
		{"zero value", domain.Token{}, true},
		{"future expiry", domain.Token{AccessToken: "at", ExpiresAt: time.Now().Add(time.Hour)}, false},
		{"past expiry", domain.Token{AccessToken: "at", ExpiresAt: time.Now().Add(-time.Hour)}, true},
		{"within skew window", domain.Token{AccessToken: "at", ExpiresAt: time.Now().Add(10 * time.Second)}, true},
		{"just past skew window", domain.Token{AccessToken: "at", ExpiresAt: time.Now().Add(time.Minute)}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.token.IsExpired(); got != tt.want {
				t.Errorf("IsExpired() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestToken_IsZero(t *testing.T) {
	tests := []struct {
		name  string
		token domain.Token
		want  bool
	}{
		{"zero value", domain.Token{}, true},
		{"has access token", domain.Token{AccessToken: "at"}, false},
		{"has refresh token only", domain.Token{RefreshToken: "rt"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.token.IsZero(); got != tt.want {
				t.Errorf("IsZero() = %v, want %v", got, tt.want)
			}
		})
	}
}
