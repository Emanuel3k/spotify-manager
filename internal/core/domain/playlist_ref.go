package domain

import (
	"fmt"
	"net/url"
	"strings"
)

// playlistIDLength is the fixed length of a Spotify base62 object id.
const playlistIDLength = 22

// ParsePlaylistID extracts a playlist ID out of anything a user might
// reasonably paste: a share link (https://open.spotify.com/playlist/ID,
// including locale-prefixed paths like /intl-pt/playlist/ID, and one
// carrying a "?si=..." tracking param), a Spotify URI
// (spotify:playlist:ID), or a bare ID.
func ParsePlaylistID(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", fmt.Errorf("playlist link is empty")
	}

	if id, ok := strings.CutPrefix(trimmed, "spotify:playlist:"); ok {
		id, _, _ = strings.Cut(id, "?")
		if isValidPlaylistID(id) {
			return id, nil
		}
		return "", fmt.Errorf("invalid spotify playlist uri: %q", input)
	}

	if u, err := url.Parse(trimmed); err == nil && u.Host != "" {
		segments := strings.Split(strings.Trim(u.Path, "/"), "/")
		for i, seg := range segments {
			if seg == "playlist" && i+1 < len(segments) && isValidPlaylistID(segments[i+1]) {
				return segments[i+1], nil
			}
		}
		return "", fmt.Errorf("no playlist id found in url: %q", input)
	}

	if isValidPlaylistID(trimmed) {
		return trimmed, nil
	}

	return "", fmt.Errorf("could not parse a playlist id from %q", input)
}

func isValidPlaylistID(s string) bool {
	if len(s) != playlistIDLength {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}
