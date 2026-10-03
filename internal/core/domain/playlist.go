package domain

// Artist is a track's performer, as returned by the Spotify Web API.
// ID is kept (not just Name) because names alone aren't a reliable way to
// filter tracks by artist — two different artists can share a display name.
type Artist struct {
	ID   string
	Name string
}

// Track is a single song as returned by the Spotify Web API, trimmed down
// to the fields the playlist-splitting/artist-filtering use cases need.
type Track struct {
	URI         string // e.g. "spotify:track:xxxxxxxxxxxxxxxxxxxxxx"
	ID          string
	Name        string
	Artists     []Artist
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
