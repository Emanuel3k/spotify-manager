// Package cli implements the primary/driving adapter: it translates
// terminal commands into calls against the core's driving ports
// (internal/core/port/in). It owns cobra and all terminal I/O; the core
// never imports this package.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/Emanuel3k/spotify-manager/internal/core/port/in"
)

// Deps bundles the driving ports the CLI needs. As new features land they
// add their own service here (e.g. Playlists in.PlaylistService) without
// touching how auth is wired.
type Deps struct {
	Auth          in.AuthService
	Profile       in.ProfileService
	PlaylistSplit in.PlaylistSplitService
}

// NewRootCmd builds the top-level "spotify-manager" command tree. Every
// action is also reachable as a scriptable subcommand (e.g.
// `spotify-manager auth login`, for automation and muscle memory); running
// the binary with no subcommand instead launches an interactive arrow-key
// menu over the same actions (see interactive.go), so day-to-day use
// doesn't require remembering flags.
func NewRootCmd(deps Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "spotify-manager",
		Short:         "A personal command-line manager for your Spotify account",
		SilenceUsage:  true,
		SilenceErrors: false,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInteractiveMenu(cmd.Context(), deps, cmd.OutOrStdout())
		},
	}

	root.AddCommand(newAuthCmd(deps))
	root.AddCommand(newPlaylistCmd(deps))

	return root
}
