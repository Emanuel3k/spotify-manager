package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

func newPlaylistByArtistCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "by-artist <playlist-link> <artist-name>",
		Short: "Create a playlist with one artist's tracks out of another playlist",
		Long: "Finds every track by the given artist inside the given playlist and upserts\n" +
			"a private playlist (named after the artist) containing them. If a playlist\n" +
			"with that name already exists it's reused, and tracks already in it are left\n" +
			"alone — running this command again is safe.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPlaylistByArtist(cmd.Context(), deps, cmd.OutOrStdout(), args[0], args[1])
		},
	}
}

// runPlaylistByArtist holds the scriptable subcommand's logic: it resolves
// artistName to an artist id (case-insensitive exact match against the
// artists actually present in the playlist) and then defers to the same
// service call the interactive picker uses.
func runPlaylistByArtist(ctx context.Context, deps Deps, out io.Writer, playlistRef, artistName string) error {
	artists, err := deps.ArtistPlaylist.ListArtists(ctx, playlistRef)
	if err != nil {
		return err
	}

	for _, a := range artists {
		if strings.EqualFold(a.Name, artistName) {
			return createArtistPlaylist(ctx, deps, out, playlistRef, a.ID, a.Name)
		}
	}
	return fmt.Errorf("no artist named %q found in that playlist", artistName)
}

// runInteractiveArtistPlaylist drives the picker flow described by the
// user: choose one of your playlists, choose an artist present in it, then
// create/upsert a playlist with just that artist's tracks from it.
func runInteractiveArtistPlaylist(ctx context.Context, deps Deps, out io.Writer) error {
	playlists, err := deps.ArtistPlaylist.ListOwnPlaylists(ctx)
	if err != nil {
		return err
	}
	if len(playlists) == 0 {
		fmt.Fprintln(out, "Você não tem nenhuma playlist.")
		return nil
	}

	playlistItems := make([]string, len(playlists))
	for i, p := range playlists {
		playlistItems[i] = p.Name
	}
	pIdx, ok, err := pickFromList("Escolha uma playlist", playlistItems)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	chosenPlaylist := playlists[pIdx]

	fmt.Fprintf(out, "Buscando artistas em %q...\n", chosenPlaylist.Name)
	artists, err := deps.ArtistPlaylist.ListArtists(ctx, chosenPlaylist.ID)
	if err != nil {
		return err
	}
	if len(artists) == 0 {
		fmt.Fprintln(out, "Nenhum artista encontrado nessa playlist.")
		return nil
	}

	artistItems := make([]string, len(artists))
	for i, a := range artists {
		plural := "faixas"
		if a.TrackCount == 1 {
			plural = "faixa"
		}
		artistItems[i] = fmt.Sprintf("%s (%d %s)", a.Name, a.TrackCount, plural)
	}
	aIdx, ok, err := pickFromList("Escolha um artista", artistItems)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	chosenArtist := artists[aIdx]

	return createArtistPlaylist(ctx, deps, out, chosenPlaylist.ID, chosenArtist.ID, chosenArtist.Name)
}

func createArtistPlaylist(ctx context.Context, deps Deps, out io.Writer, playlistRef, artistID, artistName string) error {
	result, err := deps.ArtistPlaylist.CreateFromArtist(ctx, playlistRef, artistID, artistName)
	if err != nil {
		return err
	}

	status := "playlist existente"
	if result.PlaylistCreated {
		status = "playlist criada"
	}
	fmt.Fprintf(out, "%s (%s, id %s): +%d faixas adicionadas, %d já presentes\n",
		result.PlaylistName, status, result.PlaylistID, result.TracksAdded, result.TracksSkipped)
	return nil
}
