package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newPlaylistCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "playlist",
		Short: "Manage playlists",
	}

	cmd.AddCommand(newPlaylistSplitByYearCmd(deps))
	cmd.AddCommand(newPlaylistByArtistCmd(deps))

	return cmd
}

func newPlaylistSplitByYearCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "split-by-year <playlist-link>",
		Short: "Split a playlist's tracks into new playlists named by release year",
		Long: "Reads every track of the given playlist and, for each release year present,\n" +
			"upserts a private playlist named after that year (e.g. \"2019\") containing\n" +
			"those tracks. If a year playlist already exists it's reused, and tracks\n" +
			"already in it are left alone — running this command again is safe.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlaylistSplitByYear(cmd.Context(), deps, cmd.OutOrStdout(), args[0])
		},
	}
}

// runPlaylistSplitByYear holds the actual command logic, decoupled from
// cobra so the interactive menu (internal/adapter/cli/interactive.go) can
// call the exact same behavior the scriptable subcommand uses.
func runPlaylistSplitByYear(ctx context.Context, deps Deps, out io.Writer, playlistLink string) error {
	results, err := deps.PlaylistSplit.SplitByYear(ctx, playlistLink)
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Fprintln(out, "No tracks with a known release year were found; nothing to do.")
		return nil
	}

	totalAdded := 0
	for _, r := range results {
		status := "existing playlist"
		if r.PlaylistCreated {
			status = "created playlist"
		}
		fmt.Fprintf(out, "%s (%s, id %s): %d tracks this year, +%d added, %d already present\n",
			r.PlaylistName, status, r.PlaylistID, r.TracksInYear, r.TracksAdded, r.TracksSkipped)
		totalAdded += r.TracksAdded
	}
	fmt.Fprintf(out, "\nDone: %d year playlists touched, %d tracks added in total.\n", len(results), totalAdded)

	return nil
}
