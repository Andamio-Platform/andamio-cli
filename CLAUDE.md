# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Build & Run

```bash
go build -o andamio ./cmd/andamio
./andamio --help
./andamio --version
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

## Release

```bash
# Release with auto-bumped patch version
./scripts/release.sh

# Release specific version
./scripts/release.sh 0.2.0
```

The script runs preflight checks (clean tree, on main, synced with origin, CHANGELOG entry for the target version, build passes), then tags and pushes. GitHub Actions runs GoReleaser to cross-compile and publish binaries to GitHub Releases.

`CHANGELOG.md` at the repo root is the source of truth for user-facing release notes. The `release.sh` preflight warns if no `## [$VERSION]` heading is found — maintainers should move content from `## [Unreleased]` into a new versioned heading before tagging.

Version is injected via ldflags: `-X main.version={{.Version}} -X main.commit={{.Commit}} -X main.date={{.Date}}`

## Architecture

Go CLI using Cobra for the Andamio Protocol. Dependencies: Cobra (CLI), `pkg/browser` (OAuth flow), goldmark (Markdown parsing), adrg/frontmatter (YAML frontmatter), Bursa (Cardano key loading), fxamacker/cbor/v2 (CBOR encoding), golang.org/x/crypto (Blake2b hashing).

### Package Layout

- `cmd/andamio/` — All command definitions, one file per command group. `main.go` defines `rootCmd` with global `--output` flag and version info.
- `internal/config/` — Config management. Single `Config` struct serialized to `~/.andamio/config.json`. Holds API key, base URL, user JWT, submit URL, submit headers fields. Permissions: 0600.
- `internal/client/` — HTTP client wrapping `net/http`. GET/POST/PUT. Automatically sets `X-API-Key` and `Authorization: Bearer` headers from config.
- `internal/output/` — Multi-format output (text, json, csv, markdown). Global format set via `--output` flag in `PersistentPreRunE`. Supports nested key access with dot notation.
- `internal/cardano/` — Cardano transaction signing. Loads `.skey` files via Bursa, extracts raw CBOR body bytes (preserves original encoding), signs with Blake2b-256 + ed25519, assembles VKey witnesses (merges into existing witness set).
- `internal/submit/` — HTTP client for Cardano submit APIs. Posts `application/cbor` to configurable endpoints with custom headers. Separate from the Andamio API client.

### Command Pattern

Commands register to `rootCmd` via `init()` functions in each file. Two patterns:

1. **Simple GET** — `getJSON("/api/v2/...")` helper (defined in `course.go`). Loads config, creates client, GETs path, prints via `output.PrintJSON()`.
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

Three auth slots coexist in config:
- **API Key** (`auth login --api-key`) — stored in `config.api_key`, sent as `X-API-Key` header. Read-only access.
- **User JWT** (`user login`) — browser-based wallet signing flow: starts ephemeral local HTTP server, opens browser to `{appURL}/auth/cli?redirect_uri=...&state=...`, user connects Cardano wallet and signs nonce, receives JWT via callback. CSRF protection via random state parameter. Required for edit operations on course/project commands. Headless variant: `user login --skey --alias --address`.
- **Developer JWT** (`dev login`) — headless CIP-30 signature-verified login (andamio-api #410). Two-step flow: `POST /v2/auth/developer/login/session` opens a 5-min session keyed to `(alias, wallet_address)` and returns a nonce; the CLI signs the nonce locally with `internal/cardano.SignMessage`; `POST /v2/auth/developer/login/complete` submits the signature and receives a 60-minute RS256 JWT plus a 30-day single-use rotation refresh token. The dev JWT is required for `/v2/keys` and other developer-portal endpoints — the gateway's `developerJWTAuth` middleware does not accept wallet/user JWTs and vice versa. Distinct config slot (`dev_jwt` + `dev_refresh_token`) so the two JWTs don't clobber each other. `dev refresh` rotates without re-signing (uses the refresh token); a 401 from refresh clears the dev slot and instructs re-login. `dev logout` clears the entire dev slot whenever **either** `dev_jwt` **or** `dev_refresh_token` is persisted (the durable 30-day refresh token gets cleared even when the 60-min JWT is empty). Override at runtime via `ANDAMIO_DEV_JWT` and/or `ANDAMIO_DEV_REFRESH_TOKEN` env vars (parallel to `ANDAMIO_JWT` for the user slot — the refresh-token override is the path for ephemeral CI/CD agents that want to rotate without committing tokens to the image). **Ephemeral by design:** env-sourced credentials (`ANDAMIO_JWT` / `ANDAMIO_DEV_JWT` / `ANDAMIO_DEV_REFRESH_TOKEN`) are NOT persisted to disk on `Save` — `Load` snapshots the env values and `Save` strips fields whose current value still matches the snapshot. Rotation works normally: `dev refresh` mutates the in-memory token to the gateway-rotated value (which differs from the snapshot) and that new value IS persisted, so subsequent CLI commands in the same job pick it up. The legacy lookup-only `/v2/auth/developer/account/login` is intentionally not used — it returns 410 Gone behind the gateway's kill-switch flag and does not prove wallet ownership.

The app URL is derived from the API URL by replacing `.api.` with `.app.` in the hostname.

## Complete Command Reference

### Global Flags
- `-o, --output` — Output format: text (default), json, csv, markdown
- `-h, --help` — Help for any command
- `--version` — Print version with commit hash and build date. With `--output json` emits `{version, commit, built}` as structured JSON; plain-text format is preserved when `--output` is absent or `text`.

### auth — API key management
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `auth login --api-key <key>` | local | none | Store API key |
| `auth status` | local | none | Check API key status |

### config — CLI configuration
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `config show` | local | none | Show current config |
| `config set-url <url>` | local | none | Switch environment |
| `config set-submit-url <url>` | local | none | Set Cardano submit API URL |
| `config set-submit-header <key> <value>` | local | none | Persist a submit API header (e.g., Blockfrost project_id) |
| `config remove-submit-header <key>` | local | none | Remove a persisted submit header |

### user — Wallet auth and user info
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `user login` | browser flow | wallet | Authenticate via browser wallet signing, stores JWT |
| `user login --skey <path> --alias <name>` | `/v2/auth/login/session` + `/v2/auth/login/validate` | api-key | Headless CIP-8 login for CI/CD |
| `user logout` | local | none | Clear stored JWT |
| `user status` | local | none | Show auth status (API key + JWT + session remaining) |
| `user me` | `/api/v1/user/me` | either | Current user info |
| `user usage` | `/api/v1/user/usage` | either | User usage stats |
| `user exists <alias>` | `/api/v2/user/exists/{alias}` | none | Check if alias is taken |

### course — Course content
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `course list` | `/api/v2/course/user/courses/list` | either | List courses |
| `course get <id>` | `/api/v2/course/user/course/get/{id}` | either | Course details |
| `course modules <id>` | `/api/v2/course/user/modules/{id}` | either | List modules |
| `course slts <id> <module>` | `/api/v2/course/user/slts/{id}/{module}` | either | List SLTs in module |
| `course lesson <id> <module> <slt-index>` | `/api/v2/course/user/lesson/{id}/{module}/{slt}` | either | Lesson content. slt-index must be a positive integer |
| `course assignment <id> <module>` | `/api/v2/course/user/assignment/{id}/{module}` | either | Module assignment |
| `course intro <id> <module>` | `/api/v2/course/user/introduction/{id}/{module}` | either | Module introduction |
| `course owner list` | `/v2/course/owner/courses/list` | jwt | List courses you own |
| `course owner create --course-id <id> --pending-tx-hash <hash>` | `/v2/course/owner/course/create` | jwt | Create off-chain course record (after on-chain creation). `--title`, `--description`, `--image-url`, `--video-url`, `--category`, `--public` |
| `course owner update --course-id <id>` | `/v2/course/owner/course/update` | jwt | Update course metadata. Only changed flags sent |
| `course owner register --course-id <id> --title <t>` | `/v2/course/owner/course/register` | jwt | Register on-chain course with off-chain metadata. `--title` required |
| `course owner teachers --course-id <id>` | `/v2/course/owner/teachers/update` | jwt | Add/remove teachers. `--add` (repeatable), `--remove` (repeatable) |
| `course teacher register-module` | `/v2/course/teacher/course-module/register` | jwt | Register module from chain. Idempotent on hash match: DRAFT advances to APPROVED; APPROVED/PENDING_TX/ON_CHAIN are no-ops. `--course-id`, `--module-code`, `--slt-hash`. `--output json` returns an envelope — see `register-module --help` for the shape. |
| `course teacher publish-module` | `/v2/course/teacher/course-module/publish` | jwt | Publish module. `--course-id`, `--module-code` |
| `course teacher delete-module` | `/v2/course/teacher/course-module/delete` | jwt | Delete module. `--course-id`, `--module-code` |
| `course teacher update-module-status` | `/v2/course/teacher/course-module/update-status` | jwt | Update module status. `--course-id`, `--module-code`, `--status` |
| `course teacher review` | `/v2/course/teacher/assignment-commitment/review` | jwt | Review commitment. `--course-id`, `--module-code`, `--participant-alias`, `--decision` (accept/refuse) |
| `course teacher commitments` | `/v2/course/teacher/assignment-commitments/list` | jwt | List pending reviews. `--course-id` |
| `course student courses` | `/v2/course/student/courses/list` | jwt | List enrolled courses |
| `course student credentials` | `/v2/course/student/credentials/list` | jwt | List earned credentials |
| `course student commitments` | `/v2/course/student/assignment-commitments/list` | jwt | List assignment commitments |
| `course student commitment` | `/v2/course/student/assignment-commitment/get` | jwt | Get commitment. `--course-id`, `--slt-hash` (required), `--module-code` (optional) |
| `course student create` | `/v2/course/student/commitment/create` | jwt | Enroll in module. `--course-id`, `--module-code` |
| `course student submit` | `/v2/course/student/commitment/submit` | jwt | Submit evidence. `--course-id`, `--module-code`, `--evidence` or `--evidence-file` (Markdown) |
| `course student update` | `/v2/course/student/commitment/update` | jwt | Update evidence. `--course-id`, `--module-code`, `--evidence` or `--evidence-file` (Markdown) |
| `course student leave` | `/v2/course/student/commitment/leave` | jwt | Leave commitment. `--course-id`, `--module-code`, `--pending-tx-hash` |
| `course student claim` | `/v2/course/student/commitment/claim` | jwt | Claim credential. `--course-id`, `--module-code`, `--pending-tx-hash` |
| `course credential verify-hash <course-id>` | `/api/v2/course/user/modules/{id}` | either | Verify credential hashes match computed SLT hashes |
| `course credential compute-hash` | local | none | Compute SLT hash from `--slt` flags or `--file` (outline.md). No auth required |

### project — Project data
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `project list` | `/api/v2/project/user/projects/list` | either | List projects |
| `project get <id>` | `/api/v2/project/user/project/{id}` | either | Project details |
| `project owner list` | `/v2/project/owner/projects/list` | jwt | List projects you own |
| `project owner create --project-id <id> --pending-tx-hash <hash>` | `/v2/project/owner/project/create` | jwt | Create project. `--title`, `--description`, `--image-url`, `--video-url`, `--category`, `--public` |
| `project owner update --project-id <id>` | `/v2/project/owner/project/update` | jwt | Update project metadata. Only changed flags sent |
| `project owner register --project-id <id> --title <t>` | `/v2/project/owner/project/register` | jwt | Register on-chain project with off-chain metadata. `--title` required |
| `project tasks <project-id>` | `/v2/project/user/tasks/list` | either | List tasks (public view) |
| `project manager commitments --project-id <id>` | `/v2/project/manager/commitments/list` | jwt | List task commitments — pending and assessed (with evidence). v2.3 returns the union; filter via `jq` on `--output json` |
| `project manager qualified-contributors --project-id <id>` | `/v2/project/manager/contributors/get-qualified` | jwt | List aliases qualified to commit (holds every prerequisite SLT). Capped at 500; JSON surfaces `truncated`. |
| `project contributor list` | `/v2/project/contributor/projects/list` | jwt | List contributor projects |
| `project contributor commitments` | `/v2/project/contributor/commitments/list` | jwt | List task commitments |
| `project contributor commitment` | `/v2/project/contributor/commitment/get` | jwt | Get commitment. `--project-id`, `--task-index` |
| `project contributor commit` | `/v2/project/contributor/commitment/create` | jwt | Commit to task. `--project-id`, `--task-index` |
| `project contributor update` | `/v2/project/contributor/commitment/update` | jwt | Update evidence. `--project-id`, `--task-index`, `--evidence` or `--evidence-file` (Markdown) |
| `project contributor delete` | `/v2/project/contributor/commitment/delete` | jwt | Delete commitment. `--project-id`, `--task-index` |
| `project task list <project-id>` | `/v2/project/manager/tasks/list` | jwt | List tasks (manager) |
| `project task get <index> --project-id <id>` | `/v2/project/manager/tasks/list` | jwt | Get task by index (filters from list) |
| `project task create <project-id>` | `/v2/project/manager/task/create` | jwt | Create task. Flags: --title, --lovelace, --expiration, --github-issue |
| `project task update <index> --project-id <id>` | `/v2/project/manager/task/update` | jwt | Update task fields. --project-id required |
| `project task delete <index> --project-id <id>` | `/v2/project/manager/task/delete` | jwt | Delete draft task. --project-id required |
| `project task export <project-id>` | `/v2/project/manager/tasks/list` | jwt | Export tasks to tasks/<slug>/ as Markdown |
| `project task import <project-id>` | `/v2/project/manager/task/create,update` | jwt | Import tasks from Markdown files. --dry-run supported |
| `project task verify-hash <project-id>` | `/v2/project/user/tasks/list` | either | Verify task hashes match computed hashes (diagnostic) |
| `project task compute-hash` | local | none | Compute task hash from `--content`, `--lovelace`, `--expiration`, `--token` flags or `--file`. No auth required |

### tx — Transactions
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `tx run <endpoint> --skey <path> --tx-type <type>` | build+sign+submit+register+poll | jwt | Full lifecycle: build, sign, submit, register, poll. `--body`/`--body-file`, `--no-wait`, `--timeout`, `--metadata`, `--instance-id` |
| `tx build <endpoint> --body <json>` | POST to `/api/v2/tx/*` | jwt | Build unsigned transaction via API. `--body-file` for file input |
| `tx sign --tx <hex> --skey <path>` | local | none | Sign unsigned tx with local .skey file. `--tx-file` for file input |
| `tx submit --tx <hex>` | configurable submit API | none | Submit signed tx to Cardano network. `--submit-url`, `--submit-header` |
| `tx register --tx-hash <hash> --tx-type <type>` | `/api/v2/tx/register` | jwt | Register submitted tx for tracking. `--instance-id` optional |
| `tx pending` | `/api/v2/tx/pending` | either | Pending transactions |
| `tx types` | `/api/v2/tx/types` | either | Transaction types |
| `tx status <hash>` | `/api/v2/tx/status/{hash}` | either | Transaction status |

### apikey — API key info
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `apikey usage` | `/api/v2/apikey/developer/usage/get` | api-key + dev-jwt | Key usage stats. Dual-credential surface — requires both `auth login --api-key` and `dev login` |
| `apikey profile` | `/api/v2/apikey/developer/profile/get` | api-key + dev-jwt | Key profile. Same dual-credential requirement as `apikey usage` |

### dev — Developer-portal authentication and operations
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `dev login --skey <path> --alias <name> --address <bech32>` | `/v2/auth/developer/login/session` + `/v2/auth/developer/login/complete` | api-key | Headless CIP-30 signature-verified developer login. Mints a 60-min RS256 developer JWT + 30-day rotation refresh token. Required for `/v2/keys` and other developer-portal endpoints. |
| `dev refresh` | `/v2/auth/developer/token/refresh` | dev-jwt-rotation (refresh token) | Rotate the developer JWT using the stored refresh token. Single-use rotation server-side; both tokens update atomically. 401 → re-run `dev login`. |
| `dev logout` | local | none | Clear entire dev slot (JWT, refresh token, alias, ID, tier, key hash). Does not affect user JWT. |
| `dev status` | local | none | Show developer auth status — JWT expiry, refresh-token expiry, tier. JSON envelope surfaces `jwt_expires_at` / `refresh_token_expires_at` / `*_expired` / `*_remaining_seconds` for scriptable branching. `*_remaining_seconds` is always present (no `omitempty`): zero means "sub-second remaining" (refresh now); branch on `*_expired` to disambiguate "fully expired" from "not parseable". Branch on `dev_authenticated` first. |
| `dev keys list` | `GET /v2/keys` | dev-jwt | List developer API keys across mainnet + preprod, unified. JSON passes through gateway `{keys: [...]}` envelope. |
| `dev keys create --name <label> --environment <mainnet\|preprod>` | `POST /v2/keys` | dev-jwt | Create a developer API key. Raw key value returned **exactly once** — text mode emits raw key on stdout + WARNING + metadata on stderr (so `\| pbcopy` captures key alone); JSON mode includes `key` field AND ALSO emits the WARNING on stderr so a human running `--output json` interactively still sees the one-time-use disclaimer (scripts pipe `2>/dev/null`). Errors: 422 `invalid_environment`, 429 `tier_limit_exceeded`, 503 `preprod_routing_disabled`/`preprod_unavailable` — stable error codes preserved verbatim for script branching. |
| `dev keys delete <id>` | `DELETE /v2/keys/{id}` | dev-jwt | Revoke a developer API key by local UUID. 204 No Content on success. Malformed ids rejected client-side (UUID-format gate, error `invalid developer key id`) before reaching the gateway — closes URL-injection class (`?`, `..`, empty `$ID`). 404 covers both unknown ids and ids owned by other developers (gateway threat-model: indistinguishable). |

### spec — OpenAPI spec
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `spec fetch` | `/api/v1/docs/doc.json` | none | Download OpenAPI spec to openapi.json |
| `spec paths [--filter <pattern>]` | local/remote | none | List API endpoints |

## API

- Base URLs: `https://preprod.api.andamio.io` (default), `https://mainnet.api.andamio.io`
- All paths start with `/api/v1/` or `/api/v2/`
- Auth via `X-API-Key` header (read access) and/or `Authorization: Bearer <jwt>` (edit access)
- OpenAPI spec: `andamio spec fetch` downloads to `openapi.json`

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

## Composability Rules

All commands must work without a TTY. **Never read from stdin in command handlers.**

1. **No interactive pickers.** If a required argument is omitted, return an error that tells the user how to discover valid values (e.g., `"Run 'andamio project list --output json'"`). Cobra's `ExactArgs(N)` enforces this at the framework level.

2. **Progress to stderr.** Use `fmt.Fprintf(os.Stderr, ...)` for all human-readable status/progress messages. Gate with `if !isJSON` to suppress them when `--output json` is set.

3. **Data to stdout only.** Structured output (tables, JSON, CSV, Markdown) goes to `os.Stdout` via the `output` package. Nothing else touches stdout.

4. **Required args are required.** Use `cobra.ExactArgs(N)` and `MarkFlagRequired`. Never use `MaximumNArgs` for arguments the command cannot function without.

5. **`--output json` is the scripting surface.** All list/get commands must support it with stable JSON schemas. This is what scripts, agents, and pipes consume.

The two-step composable pattern:
```bash
# 1. Discover IDs
PROJECT_ID=$(andamio project list --output json | jq -r '.data[0].project_id')

# 2. Use them directly — no prompts, no TTY needed
andamio project task list "$PROJECT_ID" --output json | jq '.data[].content.title'
andamio project task create "$PROJECT_ID" --title "..." --lovelace 5000000 --expiration 2026-06-01
```

## Adding Endpoints

1. Check available paths: `andamio spec paths --filter <keyword>`
2. Add command using `getJSON("/api/v2/...")` pattern for simple GETs, or the full config→client→output pattern for lists or POST/PUT
3. Register in `init()`

## Workflow Guides

| Guide | Location | Covers |
|-------|----------|--------|
| TX Lifecycle | `docs/TX-LIFECYCLE.md` | 5-step pipeline, terminal states, recovery procedures, all 17 TX types |
| Course Lifecycle | `docs/COURSE-LIFECYCLE.md` | Course creation, module import, SLT hashes, publishing, student enrollment |
| Project Lifecycle | `docs/PROJECT-LIFECYCLE.md` | Project creation, task management, contributor workflow, assessments |
| Solutions Index | `docs/solutions/` | Documented solutions to past problems (bugs, patterns, workflow learnings), organized by category with YAML frontmatter (`tags`, `problem_type`). Relevant when implementing or debugging in documented areas. |

## Planned Features

- **Content Sync** (`sync pull`/`sync push`/`sync status`) — Bidirectional course content sync with conflict detection. Design in `docs/PLAN-content-sync.md`.

## Cross-Repo Context

This CLI is part of the Andamio developer toolchain:

| Repo | Relationship |
|------|-------------|
| **andamio-docs** (`andamio-docs`) | CLI docs live at `content/docs/guides/developers/cli/`. 6 pages covering install, auth, courses, import/export. |
| **andamio-lesson-coach-v2** (`andamio-lesson-coach-v2`) | Creates course content that this CLI reads and will eventually sync. Compiles modules to import-ready format. |
| **andamio-app-template** (`andamio-app-template`) | Forkable Next.js starter. CLI and template are parallel developer entry points — CLI for terminal users, template for UI builders. |
| **andamio-api** (`andamio-api`) | Go gateway that serves all endpoints this CLI consumes. Base URLs: preprod.api.andamio.io, mainnet.api.andamio.io. |

The developer journey: get API key → install CLI or fork template → explore courses → use coach to create content → push back via CLI.

## Skills

- `/getting-started` — Interactive walkthrough of CLI capabilities for new developers
- `/release` — Cut a new release with preflight checks
