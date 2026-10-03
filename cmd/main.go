// Command spotify-manager is a personal CLI for managing a Spotify account.
// This file is composition-root only: it loads configuration, constructs
// every adapter, wires them into the core services through their ports, and
// hands the result to the CLI. No business logic belongs here.
package main

import (
	"fmt"
	"os"

	"github.com/Emanuel3k/spotify-manager/internal/adapter/browser"
	"github.com/Emanuel3k/spotify-manager/internal/adapter/callback"
	"github.com/Emanuel3k/spotify-manager/internal/adapter/cli"
	"github.com/Emanuel3k/spotify-manager/internal/adapter/spotifyauth"
	"github.com/Emanuel3k/spotify-manager/internal/adapter/spotifyweb"
	"github.com/Emanuel3k/spotify-manager/internal/adapter/tokenstore"
	"github.com/Emanuel3k/spotify-manager/internal/config"
	"github.com/Emanuel3k/spotify-manager/internal/core/service"
	"github.com/Emanuel3k/spotify-manager/internal/logging"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	log := logging.New()

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	authGateway := spotifyauth.New(cfg.ClientID, cfg.ClientSecret, cfg.RedirectURI, log)

	tokenRepo, err := tokenstore.NewFileStore()
	if err != nil {
		return err
	}

	browserOpener := browser.New()

	callbackListener, err := callback.New(cfg.RedirectURI)
	if err != nil {
		return err
	}

	authService := service.NewAuthService(authGateway, tokenRepo, browserOpener, callbackListener, cfg.Scopes, log)

	profileGateway := spotifyweb.New(log)
	profileService := service.NewProfileService(authService, profileGateway, log)

	playlistGateway := spotifyweb.NewPlaylistGateway(log)
	playlistSplitService := service.NewPlaylistSplitService(authService, profileService, playlistGateway, log)
	artistPlaylistService := service.NewArtistPlaylistService(authService, profileService, playlistGateway, log)

	root := cli.NewRootCmd(cli.Deps{
		Auth:           authService,
		Profile:        profileService,
		PlaylistSplit:  playlistSplitService,
		ArtistPlaylist: artistPlaylistService,
	})

	return root.Execute()
}
