# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o andamio ./cmd/andamio
./andamio --help
```

## Architecture

Go CLI using Cobra for the Andamio Protocol.

- `cmd/andamio/` - Command definitions (one file per command group: course.go, project.go, tx.go, etc.)
- `internal/config/` - Config management (~/.andamio/config.json)
- `internal/client/` - HTTP client for Andamio API

Commands register to `rootCmd` via `init()`. The `getJSON()` helper in course.go handles simple GET endpoints.

## API

- Base URLs: `https://preprod.api.andamio.io` (default), `https://mainnet.api.andamio.io`
- All paths start with `/api/v1/` or `/api/v2/`
- Auth via `X-API-Key` header
- OpenAPI spec: `./andamio spec fetch` downloads to `openapi.json`

## Adding Endpoints

1. Check available paths: `./andamio spec paths --filter <keyword>`
2. Add command using `getJSON("/api/v2/...")` pattern for simple GETs
3. Register in `init()`

## Skills

- `/getting-started` - Interactive walkthrough of CLI capabilities
