package domain

// Track is a single song as returned by the Spotify Web API, trimmed down
// to the fields the playlist-splitting use case needs.
type Track struct {
	URI         string // e.g. "spotify:track:xxxxxxxxxxxxxxxxxxxxxx"
	ID          string
	Name        string
	Artists     []string
	ReleaseYear int // 0 when unknown (e.g. missing/unparseable release date)
}

// Playlist is a Spotify playlist, trimmed down to what the CLI needs to
// display and to decide whether a playlist with a given name already
// exists (upsert).
type Playlist struct {
	ID      string
	Name    string
	OwnerID string
	Public  bool
}
