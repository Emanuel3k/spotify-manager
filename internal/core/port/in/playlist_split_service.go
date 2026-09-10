package in

import "context"

// YearSplitResult reports what happened for one release-year bucket during
// a split-by-year run.
type YearSplitResult struct {
	Year            int
	PlaylistID      string
	PlaylistName    string
	PlaylistCreated bool // false when an existing playlist was reused (upsert)
	TracksInYear    int  // how many source tracks belong to this year
	TracksAdded     int  // how many were newly added this run
	TracksSkipped   int  // how many were already present (upsert dedupe)
}

// PlaylistSplitService is the driving port for splitting a playlist's
// tracks into per-release-year playlists.
type PlaylistSplitService interface {
	// SplitByYear reads every track of the playlist identified by
	// playlistLink (a share URL, spotify: URI, or bare ID) and, for each
	// release year present, upserts a private playlist named after that
	// year (e.g. "2019") containing those tracks. Existing year playlists
	// and tracks already in them are left alone (upsert semantics).
	SplitByYear(ctx context.Context, playlistLink string) ([]YearSplitResult, error)
}
