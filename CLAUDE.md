# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o andamio ./cmd/andamio
./andamio --help
```

No tests exist yet. No linter configuration.

## Architecture

Go CLI using Cobra for the Andamio Protocol. Minimal dependency set: Cobra for CLI, `pkg/browser` for OAuth flow.

### Package Layout

- `cmd/andamio/` - All command definitions live here as top-level files (one per command group). `main.go` defines `rootCmd` with a global `--output` flag.
- `internal/config/` - Config management. Single `Config` struct serialized to `~/.andamio/config.json`. Holds API key, base URL, and user JWT fields.
- `internal/client/` - HTTP client wrapping `net/http`. Supports GET/POST/PUT. Automatically sets `X-API-Key` and `Authorization: Bearer` headers from config.
- `internal/output/` - Multi-format output (text, json, csv, markdown). Global format set via `--output` flag in `PersistentPreRunE`.

### Command Pattern

Commands register to `rootCmd` via `init()` functions in each file. Most commands follow one of two patterns:

1. **Simple GET** — use `getJSON("/api/v2/...")` helper (defined in `course.go`). Loads config, creates client, GETs path, prints via `output.PrintJSON()`.
2. **List with formatting** — load config, create client, GET, extract `data` array, call `output.PrintList(items, titleKey, idKey)` with dot-notation keys for nested fields.

### Auth Flow

Two auth methods coexist in config:
- **API Key** (`auth login --api-key`) — stored in `config.api_key`, sent as `X-API-Key` header
- **User JWT** (`user login`) — browser-based OAuth-like flow: starts ephemeral local HTTP server, opens browser to `{appURL}/auth/cli?redirect_uri=...&state=...`, receives JWT via callback query params. CSRF protection via random state parameter.

The app URL is derived from the API URL by replacing `.api.` with `.app.` in the hostname.

## API

- Base URLs: `https://preprod.api.andamio.io` (default), `https://mainnet.api.andamio.io`
- All paths start with `/api/v1/` or `/api/v2/`
- Auth via `X-API-Key` header (read access) and/or `Authorization: Bearer <jwt>` (edit access)
- OpenAPI spec: `./andamio spec fetch` downloads to `openapi.json`

## Adding Endpoints

1. Check available paths: `./andamio spec paths --filter <keyword>`
2. Add command using `getJSON("/api/v2/...")` pattern for simple GETs, or the full config→client→output pattern for lists or POST/PUT
3. Register in `init()`

## Skills

- `/getting-started` - Interactive walkthrough of CLI capabilities
