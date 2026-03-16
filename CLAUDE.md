# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Run

```bash
go build -o andamio ./cmd/andamio
./andamio --help
```

Versioned release build:
```bash
go build -ldflags "-X main.version=0.1.0 -X main.commit=$(git rev-parse --short HEAD) -X main.date=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o andamio ./cmd/andamio
```

Install to PATH:
```bash
cp andamio /usr/local/bin/andamio
```

No linter configuration. Tests exist for export/import conversion functions.

## Architecture

Go CLI using Cobra for the Andamio Protocol. Dependencies: Cobra (CLI), `pkg/browser` (OAuth flow), goldmark (Markdown parsing), adrg/frontmatter (YAML frontmatter).

### Package Layout

- `cmd/andamio/` - All command definitions live here as top-level files (one per command group). `main.go` defines `rootCmd` with a global `--output` flag and versioning via ldflags.
- `internal/config/` - Config management. Single `Config` struct serialized to `~/.andamio/config.json`. Holds API key, base URL, and user JWT fields.
- `internal/client/` - HTTP client wrapping `net/http`. Supports GET/POST/PUT. Automatically sets `X-API-Key` and `Authorization: Bearer` headers from config.
- `internal/output/` - Multi-format output (text, json, csv, markdown). Global format set via `--output` flag in `PersistentPreRunE`.

### Command Pattern

Commands register to `rootCmd` via `init()` functions in each file. Most commands follow one of two patterns:

1. **Simple GET** — use `getJSON("/api/v2/...")` helper (defined in `course.go`). Loads config, creates client, GETs path, prints via `output.PrintJSON()`.
2. **List with formatting** — load config, create client, GET, extract `data` array, call `output.PrintList(items, titleKey, idKey)` with dot-notation keys for nested fields.

### Export/Import Pattern (complex commands)

Export and import are the two complex commands. They follow a different pattern:

1. **Teacher endpoints only** — use `POST /v2/course/teacher/course-modules/list` (not user GET endpoints). Teacher endpoints return draft + on-chain modules with full content inline.
2. **Structured output** — `ExportResult`/`ImportResult` structs for `--output json`; progress messages suppressed in JSON mode via `output.GetFormat()` checks.
3. **H1 title extraction** — lesson/intro/assignment files: first `# Heading` becomes the `title` field, remainder becomes `content_json`. Matches app behavior.
4. **Metadata preservation** — import fetches existing module state before updating, merges `title`, `description`, `image_url`, `video_url` from existing data.
5. **Image upload** — new images in `assets/` are uploaded to `{appURL}/api/upload` (multipart/form-data with JWT), CDN URLs added to manifest.
6. **Image manifest** — `.image-manifest.json` maps local filenames to CDN URLs. Updated on disk after uploads so future imports don't re-upload.
7. **SLT locking** — import checks module status; skips sending SLTs for non-DRAFT modules to avoid `SLT_LOCKED` errors.
8. **Tiptap node types** — standalone images use `imageBlock` (with `width: "600"`, `align: "center"` attrs), not `image`. Matches app's `markdown-to-tiptap.ts`.
9. **Goldmark TextBlock** — tight list items use `ast.TextBlock`, not `ast.Paragraph`. Both are handled identically in the converter.

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

### Key Endpoints for Export/Import

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/v2/course/teacher/courses/list` | POST | Get course list (for slug derivation) |
| `/v2/course/teacher/course-modules/list` | POST | Get all modules with full content (draft + on-chain) |
| `/v2/course/teacher/course-module/update` | POST | Atomic update of module + SLTs + lessons + intro + assignment |
| `{appURL}/api/upload` | POST | Upload image to GCS CDN (multipart/form-data) |

### API Payload Structure (course-module/update)

```
{
  course_id, course_module_code, title,
  slts: [{slt_index, slt_text}],           // Only when DRAFT
  lessons: [{slt_index, title, content_json, description?, image_url?, video_url?}],
  introduction: {title, content_json, description?},
  assignment: {title, content_json, description?, image_url?, video_url?}
}
```

Omitted top-level fields = unchanged. But array items (lessons, slts) replace the full entity — must include all fields to preserve them.

## Adding Endpoints

1. Check available paths: `./andamio spec paths --filter <keyword>`
2. Add command using `getJSON("/api/v2/...")` pattern for simple GETs, or the full config→client→output pattern for lists or POST/PUT
3. Register in `init()`

## Skills

- `/getting-started` - Interactive walkthrough of CLI capabilities
