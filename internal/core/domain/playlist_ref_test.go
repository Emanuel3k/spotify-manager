package domain_test

import (
	"testing"

	"github.com/Emanuel3k/spotify-manager/internal/core/domain"
)

func TestParsePlaylistID(t *testing.T) {
	const id = "37i9dQZF1DXcBWIGoYBM5M" // 22 chars, valid shape

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{"share url", "https://open.spotify.com/playlist/" + id, id, false},
		{"share url with tracking param", "https://open.spotify.com/playlist/" + id + "?si=abc123def456", id, false},
		{"share url with locale prefix", "https://open.spotify.com/intl-pt/playlist/" + id, id, false},
		{"http scheme", "http://open.spotify.com/playlist/" + id, id, false},
		{"spotify uri", "spotify:playlist:" + id, id, false},
		{"bare id", id, id, false},
		{"surrounded by whitespace", "  " + id + "\n", id, false},
		{"empty", "", "", true},
		{"whitespace only", "   ", "", true},
		{"unrelated url", "https://open.spotify.com/album/" + id, "", true},
		{"too short id", "https://open.spotify.com/playlist/short", "", true},
		{"garbage", "not a playlist link at all", "", true},
		{"spotify uri wrong type", "spotify:album:" + id, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := domain.ParsePlaylistID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePlaylistID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParsePlaylistID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
