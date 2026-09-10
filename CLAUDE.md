# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```powershell
go build ./...                              # compile everything
go vet ./...                                 # static checks
gofmt -l .                                   # formatting check; must print nothing before committing
go test ./...                                # run all tests
go test ./... -v -run TestName               # run a single test by name (works across packages)
go test ./internal/core/service/... -v       # run one package's tests

go run ./cmd auth login                      # OAuth login (opens browser)
go run ./cmd auth status                     # session status
go run ./cmd auth whoami                     # print current Spotify user
go run ./cmd auth logout
go run ./cmd playlist split-by-year "<link>"

$env:LOG_LEVEL="debug"                       # PowerShell: verbose HTTP-call logging (default is info)
```

`.env` in the repo root holds `SPOTIFY_CLIENT_ID`, `SPOTIFY_CLIENT_SECRET`, `SPOTIFY_REDIRECT_URI` (see `.env.example`); it's gitignored. Config loading (`internal/config`) requires the process to run from the repo root so `.env` is found.

## Architecture

Hexagonal (ports & adapters). The rule that matters: **`internal/core` never imports `internal/adapter`**, and never imports `net/http`, `cobra`, or anything infrastructure-specific. Everything in `core` talks only to interfaces.

```
internal/core/domain/     Plain value objects + sentinel errors (domain.Token, domain.Track,
                           domain.Playlist, domain.User, domain.ErrNotAuthenticated, ...).
                           domain.ParsePlaylistID lives here too — pure logic, no I/O.

internal/core/port/in/    Driving ports: what the CLI (or any future frontend) can call.
                           One interface per use case (AuthService, ProfileService,
                           PlaylistSplitService), plus the DTOs they return (in.Status,
                           in.YearSplitResult).

internal/core/port/out/   Driven ports: what core needs from the outside world
                           (SpotifyAuthGateway, ProfileGateway, PlaylistGateway,
                           TokenRepository, BrowserOpener, CallbackListener).

internal/core/service/    Implements the port/in interfaces on top of port/out interfaces.
                           This is all the actual business logic. Services depend on other
                           services through their port/in interface, not their concrete type
                           (e.g. PlaylistSplitService takes in.AuthService and in.ProfileService,
                           not *service.AuthService) — keeps them independently testable.

internal/adapter/cli/     Driving adapter: cobra commands. Owns all terminal I/O. Deps struct
                           in root.go bundles the port/in services the CLI needs; cmd/main.go
                           constructs it. Every action's real logic lives in a plain
                           `run*(ctx, deps, out) error` function (runAuthLogin, runAuthStatus,
                           runAuthWhoami, runAuthLogout in auth.go; runPlaylistSplitByYear in
                           playlist.go) — the cobra RunE closures are thin wrappers around
                           these. interactive.go's arrow-key menu (shown when the binary runs
                           with no subcommand, via root's RunE) calls the exact same functions,
                           so the two interfaces can't drift apart. Add new commands the same
                           way: write the `run*` func first, wrap it in a cobra command, then
                           add it to interactive.go's `actions` slice.

internal/adapter/spotifyauth/   Driven adapter for accounts.spotify.com (OAuth token exchange/refresh).
internal/adapter/spotifyweb/    Driven adapter for api.spotify.com (profile, playlists). Both
                                 gateways embed a shared `client` (client.go) that centralizes
                                 auth headers, JSON error parsing and request logging.
internal/adapter/tokenstore/    TokenRepository as a JSON file under the OS user config dir.
internal/adapter/browser/       BrowserOpener via OS-native "open URL" shell-out.
internal/adapter/callback/      CallbackListener: short-lived local HTTP server that captures
                                 the OAuth redirect, then shuts itself down.

internal/config/          Loads .env + environment into a Config struct (client id/secret,
                           redirect URI, requested OAuth scopes).
internal/logging/         Builds the single *slog.Logger for the process (text handler to
                           stderr, level from LOG_LEVEL).
cmd/main.go                Composition root only — constructs every adapter and service and
                            wires them together. No business logic here.
```

**Adding a new feature follows this path every time:** add/extend a `domain` type if needed → define the `port/out` interface(s) it needs → define its `port/in` interface (the use case) → implement it in `core/service` against the interfaces → implement any new `port/out` adapter(s) → add a cobra command in `internal/adapter/cli` → wire the new service into `cli.Deps` and `cmd/main.go`.

### Logging

Every service and HTTP adapter constructor takes a `*slog.Logger` (nil-safe — nil becomes a discard logger, see `service.NewAuthService`, `spotifyweb.newClient`, etc.). Each layer tags its own logger with `.With("component", "...")`. HTTP adapters log every outgoing request/response at Debug (method, path, status, duration) and errors at Error; services log user-facing milestones at Info/Warn/Error. This is how failures get traced in production — don't replace it with `fmt.Println`.

### Testing conventions

- **Core services**: tested from an external `service_test` package against hand-rolled fakes (not a mocking library) in `internal/core/service/fakes_test.go`, shared across `auth_service_test.go`, `profile_service_test.go`, `playlist_split_service_test.go`. Add new fakes there rather than duplicating per test file.
- **HTTP adapters** (`spotifyauth`, `spotifyweb`): white-box tests (`package spotifyauth` / `package spotifyweb`, not `_test`) that spin up an `httptest.Server` and point the gateway at it by overriding its unexported `tokenURL`/`authorizeURL` (spotifyauth) or `baseURL` (spotifyweb, via the embedded `client`) field directly. This is the established seam for testing HTTP adapters in this repo — don't add exported test-only constructors for it.
- `internal/adapter/callback`: tests bind a real ephemeral port and hit it with `http.Get`, since it's a real (if short-lived) local server.

## Known Spotify Web API gotchas (read before touching `internal/adapter/spotifyweb` or `spotifyauth`)

Spotify changed the Web API for Development Mode apps in February 2026 (after most LLM training cutoffs — don't trust prior knowledge of this API without checking). This app's client ID was created after that date, so the new rules apply immediately, no grandfathering:

- Playlist item endpoints are `/playlists/{id}/items`, not `/playlists/{id}/tracks` (and the JSON key is `item`, not `track`). Creating a playlist is `POST /me/playlists`, not `POST /users/{id}/playlists`.
- `GET /playlists/{id}/items` only returns track data for playlists the current user owns or collaborates on; for any other playlist it returns zero items with no error.
- A 403 from these endpoints with **no `www-authenticate` header** means "endpoint not available to Development Mode apps", not a scope problem — a real scope/token error always carries that header.
- **Live platform bug** (still present as of Sep 2026): `POST /me/playlists` ignores the requested `public: false` and creates the playlist public anyway; a follow-up `PUT /playlists/{id}` doesn't reliably fix it either. `spotifyweb.PlaylistGateway.CreatePlaylist` already does a best-effort follow-up PUT and logs a warning — don't "fix" this by removing the workaround, and don't assume `domain.Playlist.Public` in the response is guaranteed accurate.
- Development Mode apps are capped at 5 authorized users and require the app owner to have Spotify Premium.

If you add a new Web API call, check the current [migration guide](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide) for the endpoint first rather than assuming pre-2026 shapes.
