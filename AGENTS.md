# AGENTS.md

Guidance for AI coding agents (Claude Code, Cursor, Aider, Codex CLI, etc.) working in this repository. Full details live in [CLAUDE.md](CLAUDE.md) and [README.md](README.md) — read those before making non-trivial changes. This file is the short version for agents that only look for `AGENTS.md`.

## Quick facts

- Go 1.26, module `github.com/Emanuel3k/spotify-manager`.
- Hexagonal architecture: `internal/core` (domain/ports/services) must never import `internal/adapter` or any HTTP/CLI/filesystem package. Everything in `core` depends on interfaces (`internal/core/port/{in,out}`) only.
- Build/test: `go build ./...`, `go vet ./...`, `gofmt -l .` (must be empty), `go test ./...`.
- Entry point / composition root: `cmd/main.go`. Adding a feature means touching `domain` → `port/out` → `port/in` → `core/service` → `adapter` → `adapter/cli` → wire it in `cmd/main.go`, in that order.
- Every service/HTTP-adapter constructor takes a `*slog.Logger` (nil-safe). Use it for anything worth tracing; don't add `fmt.Println` debugging.
- Tests use hand-rolled fakes (`internal/core/service/fakes_test.go`) for core services and `httptest.Server` white-box tests for HTTP adapters — no mocking library in this repo.

## Critical gotcha before touching Spotify API code

Spotify changed the Web API for Development Mode apps in **February 2026**. If your training/knowledge predates that, do not trust it for this API: playlist endpoints moved from `/playlists/{id}/tracks` to `/playlists/{id}/items` (JSON key `track` → `item`), and creating a playlist moved from `POST /users/{id}/playlists` to `POST /me/playlists`. A 403 with no `www-authenticate` header on these calls means "endpoint unavailable in Development Mode", not a scope bug. There's also a live Spotify-side bug where `POST /me/playlists` ignores `public: false` — see `internal/adapter/spotifyweb/playlist_gateway.go` for the existing workaround, and the [migration guide](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide) before adding any new endpoint call.

Secrets: `.env` (client id/secret, gitignored) must exist in the repo root for anything that talks to Spotify to work; `.env.example` documents the shape.
