# Copilot Instructions

## Build & Test

```bash
go build -o spotify-yearly-discoveries ./cmd/spotify-yearly-discoveries  # build main binary
go test ./...                                                            # run all tests
go test ./cmd/spotify-yearly-discoveries/ -run TestServeConfig           # run a single test
```

## Architecture

This is a Go application with two modes of operation, sharing core logic:

- **Batch mode** (`make run`): Scans the user's Spotify playlists for songs released in a given year, filters by library status, and populates a target playlist. Configured entirely via environment variables (see `env` file).
- **Web mode** (`make serve` / `-web` flag): OAuth-based web UI that lets users browse yearly discoveries interactively without modifying playlists. Uses `//go:embed` for static assets from `cmd/spotify-yearly-discoveries/web/`.

### Code layout

- `cmd/spotify-yearly-discoveries/` — Main binary. `main.go` has batch logic, `server.go` has web server + OAuth + in-memory session/cache management, `playlists.go` has shared playlist-fetching used by both modes.
- `cmd/get-token/` — Standalone helper that runs a local OAuth flow on port 8888 to obtain and save a Spotify token for batch mode.
- `internal/randutil/` — Crypto-random hex string helper used by both commands.
- `infra/` — Azure Bicep templates for Container Apps deployment.

### Spotify API interaction

All Spotify API calls go through [`zmb3/spotify/v2`](https://github.com/zmb3/spotify/v2). The API paginates results (100 items per page for playlist tracks, 50 for user playlists). Library-check calls (`UserHasTracks`) are batched at 50 tracks. Both batch and web modes handle pagination with offset-based loops that check `page.Next == ""` to stop.

## Conventions

- **Structured logging**: Use `log/slog` throughout — never `fmt.Println` or `log.Printf` for operational output.
- **Environment-driven config**: All runtime configuration comes from environment variables (see `env` file template). No config files or CLI flags beyond `-web` and `-port`.
- **Embedded static assets**: Web UI files in `cmd/spotify-yearly-discoveries/web/` are embedded at compile time via `//go:embed`. Changes to HTML/CSS take effect on rebuild, not at runtime.
- **Tests use `httptest`**: Server tests use `httptest.NewRequest`/`httptest.NewRecorder` rather than starting a real HTTP server. Test functions follow table-driven style with `t.Run` subtests.
