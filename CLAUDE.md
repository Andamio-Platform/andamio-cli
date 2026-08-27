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

No linter configuration.

## Testing

```bash
go test ./...          # full suite
go test ./cmd/andamio  # command layer, where the contract guards live
```

**The test suite is the contract.** There are ~375 test functions. Most of the guarantees described in this file are enforced by a specific test, not by convention — before changing behavior in these areas, read the guard first:

| Invariant | Guarded by |
|-----------|-----------|
| Exit-code / `kind` mapping (see [Failure Contract](#failure-contract)) | `cmd/andamio/exitcode_test.go` |
| Retired 1.0 commands: exit 4, hidden, message text, every invocation shape | `cmd/andamio/retired_test.go` |
| No source file calls a retired route | `TestNoSourceFileCallsARetiredRoute` in `retired_test.go` |
| Unknown command exits non-zero with clean stdout | `cmd/andamio/unknown_test.go` |
| Local JWT expiry handling at all four enforcement points | `cmd/andamio/expired_jwt_test.go` |
| Export/import Markdown ↔ Tiptap conversion | `course_export_test.go`, `course_import_test.go` |
| Commitment hash parity with the protocol | `commitment_hash_parity_test.go` |

If you are adding a rule that must not regress, add a case to the relevant file above rather than building a separate mechanism.

## Release

```bash
# Release with auto-bumped patch version
./scripts/release.sh

# Release specific version
./scripts/release.sh 0.2.0
```

The script runs preflight checks (clean tree, on main, synced with origin, CHANGELOG entry for the target version, build passes), then tags and pushes. GitHub Actions runs GoReleaser to cross-compile and publish binaries to GitHub Releases.

`CHANGELOG.md` at the repo root is the source of truth for user-facing release notes, and since 1.0 that is literally true rather than aspirational: `.github/workflows/release.yml` runs `scripts/changelog-section.sh <version>` to extract the `## [VERSION]` section into `release-notes.md` and passes it to `goreleaser release --release-notes`. Before this, GoReleaser generated the GitHub release body from commit subjects — which for a release whose headline change is a removal would lead with that removal, telling the people least affected that the tool is being wound down.

The `release.sh` preflight checks that the section is **extractable**, not merely that the heading exists: a `## [$VERSION]` heading with nothing under it would otherwise publish a blank release body. Maintainers should move content from `## [Unreleased]` into a new versioned heading before tagging.

The `changelog:` block in `.goreleaser.yml` remains as the fallback for a local `goreleaser release` run without the flag.

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

### Degraded reads (206 Partial Content)

`client.Get` accepts the whole 2xx range. The gateway's merged read endpoints answer **206** with the normal `data` plus `meta.warning` when one backend is unavailable; that is a success (exit 0) with degraded data, not an error. `warnMetaWarning` in `helpers.go` prints the warning on stderr in every mode except JSON, where the envelope (including `meta`) passes through on stdout — `getJSON`, `printList` and `printListPost` all route through it. Exception: non-empty **list** commands emit a bare array in JSON mode (`todos/038`) with no slot for `meta`, so a degraded list keeps that shape and warns on stderr in JSON mode too — the shape must never depend on gateway health. Guarded by `TestExitCodes_206PartialContentIsSuccess` (#157).

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
- **Developer JWT** (`dev login`) — supports two modes. **Browser mode** (default, `dev login` with no args) opens `{appURL}/auth/dev-cli` and waits for a wallet-signed nonce from Eternl/Lace/Nami via an ephemeral localhost callback — the typical developer journey, since browser wallets don't expose `.skey` files. The browser-flow callback is **POST + JSON** (`Content-Type: application/json`) per `andamio-app-v2#699`'s `DevCliSuccessPayload` / `DevCliErrorPayload`, with an `OPTIONS` preflight serving CORS + `Access-Control-Allow-Private-Network: true` so Chrome's PNA spec permits the HTTPS-origin → 127.0.0.1 POST. The listener enforces an exact-string `Origin` allow-list (derived from `cfg.BaseURL` via `.api.`→`.app.` swap) on browser-originated POSTs; loopback diagnostics without an `Origin` header are still accepted. The user-login browser flow (`/auth/cli`) deliberately stays on **GET + query params** (lower-sensitivity payload, no 30-day refresh token); the two flows have intentionally different wire formats. **Headless mode** (`dev login --skey --alias --address`) signs locally for CI/CD, ops, and devkit. Both call andamio-api's CIP-30 signature-verified login endpoints (#410). Two-step flow: `POST /v2/auth/developer/login/session` opens a 5-min session keyed to `(alias, wallet_address)` and returns a nonce; the CLI signs the nonce locally with `internal/cardano.SignMessage`; `POST /v2/auth/developer/login/complete` submits the signature and receives a 60-minute RS256 JWT plus a 30-day single-use rotation refresh token. The dev JWT is required for `/v2/keys`, `/api/v2/apikey/developer/*`, and other developer-portal endpoints — the gateway's `developerJWTAuth` middleware does not accept wallet/user JWTs and vice versa. These surfaces are **dual-credential**: the gateway's `V2AuthMiddleware` requires `X-API-Key` and the inner `developerJWTAuth` requires `Authorization: Bearer <devJWT>`. The CLI's `devKeysClient` helper (`cmd/andamio/dev_keys.go`) is the shared routing for any dual-credential dev-portal surface — preserves `APIKey`, promotes `DevJWT` into the JWT slot, both headers ride on the wire. `dev keys` and `apikey usage`/`profile` both route through it; new dev-portal commands should too. Distinct config slot (`dev_jwt` + `dev_refresh_token`) so the two JWTs don't clobber each other. `dev refresh` rotates without re-signing (uses the refresh token); a 401 from refresh clears the dev slot and instructs re-login. `dev logout` clears the entire dev slot whenever **either** `dev_jwt` **or** `dev_refresh_token` is persisted (the durable 30-day refresh token gets cleared even when the 60-min JWT is empty). Override at runtime via `ANDAMIO_DEV_JWT` and/or `ANDAMIO_DEV_REFRESH_TOKEN` env vars (parallel to `ANDAMIO_JWT` for the user slot — the refresh-token override is the path for ephemeral CI/CD agents that want to rotate without committing tokens to the image). **Ephemeral by design:** env-sourced credentials (`ANDAMIO_JWT` / `ANDAMIO_DEV_JWT` / `ANDAMIO_DEV_REFRESH_TOKEN`) are NOT persisted to disk on `Save` — `Load` snapshots the env values and `Save` strips fields whose current value still matches the snapshot. Rotation works normally: `dev refresh` mutates the in-memory token to the gateway-rotated value (which differs from the snapshot) and that new value IS persisted, so subsequent CLI commands in the same job pick it up. The legacy lookup-only `/v2/auth/developer/account/login` is intentionally not used — it returns 410 Gone behind the gateway's kill-switch flag and does not prove wallet ownership.

**Local JWT expiry handling (#134).** `internal/config/jwt.go` decodes a token's `exp` claim locally (payload base64 only, no signature verification — the decoded value drives a send/don't-send decision; the gateway stays the authority) with a conservative 30s skew: expired means `now >= exp - 30s`, so the CLI never sends a token the gateway might already reject. Four enforcement points, in request order: (1) `client.New` drops a locally-expired **user-slot** JWT from its own field snapshot (config is never mutated, so no `Save` can persist the clear) and prints one stderr warning per process — resettable `warnOnce`, emitted in every output mode, env-aware wording (`ANDAMIO_JWT` vs `user login`); (2) `requireUserAuth` (helpers.go) fails fast with exit 3 / `kind: auth` + expiry timestamp on all JWT-required commands — `jwtAuthPreRunE` parents AND the seven hand-rolled PreRunEs (`course export/import/import-all/create-module`, `tx build/register/run`); (3) `devKeysClient` fail-fasts on an expired **dev** JWT with a `dev refresh` hint *before* promoting it into the shared UserJWT slot — ordering is load-bearing: it guarantees the client-level drop can never silently strip a dev JWT on a dual-credential surface (the 0.12.x regression class); (4) either-auth course reads route teacher-vs-user endpoints via `HasFreshUserAuth`, not `HasUserAuth`, so an expired JWT + API key lands on the user endpoint and succeeds. **Fail open on undecodable tokens everywhere** — non-JWT strings (incl. the `"test-jwt"` test fixtures) are sent as-is, never treated as expired. Login flows are self-healing: the browser guard treats expired as unauthenticated, and headless login builds its client from a cfg copy with `UserJWT` blanked (login never needs prior user auth — this covers even tokens the CLI cannot decode). Headless login persists `jwt_expires_at` from the decoded `exp`; `user status` falls back to decoding `exp` when the stored field is empty and computes `session_expired` with the same skew predicate as enforcement (probe and enforcement must agree).

The app URL is derived from the API URL by replacing `.api.` with `.app.` in the hostname.

## The 1.0 Surface

Andamio CLI 1.0 is scoped to the roles that **author work and assess it**: course Owners and Teachers, project Managers. The learner and contributor command surface (`course student *`, `project contributor *`) was removed in 1.0 — learners and contributors use the Andamio app, which signs and submits their work in one flow.

Removed commands are not deleted from the routing table. `cmd/andamio/retired.go` holds a registry of retired paths, each registered as a **hidden** cobra stub that returns a typed `apierr.RemovedCommandError` (exit 4) naming the removal and the replacement. One registry drives stub registration, message text, and the regression guards. Adding a future retirement is one table entry.

Two properties are load-bearing and easy to break:

- Stubs use `FParseErrWhitelist{UnknownFlags: true}`, **not** `DisableFlagParsing`. The latter also skips the root's persistent `--output` flag, so `course student claim --output json` would emit plain text instead of a JSON error envelope.
- Stubs must not inherit an auth `PersistentPreRunE`. Both retired groups gated on `jwtAuthPreRunE`; inheriting it would greet an unauthenticated caller with "not authenticated" instead of the removal notice.

`TestNoSourceFileCallsARetiredRoute` walks the AST of every non-test source file and fails on a string literal referencing `/course/student/` or `/project/contributor/`. It inspects literals rather than raw text so comments explaining a removal stay legal.

**Enforcement lives in `cmd/andamio/retired_test.go`** — all of the above, including the two load-bearing properties, plus every retired path in every invocation shape. Both properties are silent failures if broken: nothing else in the build notices. Note that the retired stubs are `Hidden: true`, so any tooling built on `cobra/doc.GenMarkdownTree` cannot see them and cannot guard them.

### 1.0 release status

**1.0 is merged to `main` but not tagged.** The latest release is `v0.13.3`, which still ships the learner/contributor surface. 1.0 declares a hard requirement on **Andamio API 2.5 or later** and mainnet has not cut over (`andamio-ops#189`), so the tag is gated on that cutover. See the `## [1.0.0]` CHANGELOG entry and README for the supported-version statement. Two consequences worth holding onto: the published contract today is 0.13.x, not what is on `main`; and because 2.5 carries a contract naming change, the cutover is the moment this CLI meets renamed gateway fields (see `todos/031`).

## Failure Contract

*Enforced by `cmd/andamio/exitcode_test.go`. The `kind` strings below are a public API — scripts branch on them. Changing one is a breaking change and will fail that test.*

Every failure carries an exit code **and**, under `--output json`, a `kind` field. Both derive from `apierr.Kind`, the single mapper, so they cannot drift apart. `Kind` unwraps through `fmt.Errorf("%w")` and `ReportedError` — the command layer wraps liberally, and a mapper that only inspected the top-level error would classify nearly everything as generic.

| Exit | `kind` | When |
|------|--------|------|
| 0 | — | Success, including an empty but valid result set |
| 1 | `error` / `server` / `backpressure` / `canceled` | Unexpected, 5xx, retry-later, interrupted |
| 2 | `not_found` | 404 |
| 3 | `auth` | No credentials, or 401/403 |
| 4 | `removed_command` | Retired in 1.0 |
| 5 | `unreachable` | Request never reached the service |
| 6 | `conflict` | 409 |
| 7 | `tier_limit` | Plan does not permit the action; remedy is billing-side. Classified by body code `tier_limit_exceeded` on any 4xx (429 today, 403 after product-circle#304), before the status switch. Never retried |

**An empty result is exit 0 with an empty collection, not an error.** This is what keeps "nothing found", "not permitted" (3) and "could not reach the service" (5) distinguishable. Do not "fix" `printList` to return an error on empty.

`apierr.NetworkError` wraps transport failures in `internal/client`. It deliberately excludes context cancellation and deadline expiry — an operator pressing Ctrl-C is not the service being down. It implements `Unwrap()` so the retry classifier still reaches the underlying `net.Error`; removing that silently disables retries on connection failures.

Exit codes 0–3 predate 1.0 and are fixed. `conflict` moved from 1 to 6 in 1.0. `tier_limit` (7) is new in 1.0: the decoder in `internal/client.statusError` runs before the status switch, reads the raw body tolerantly (nested `{"error":{"code":…}}` or flat `{"error":"…"}`), and matches the code exactly — the gateway's `keys_viewmodels.ErrCodeTierLimitExceeded` is a CLI contract. Uncoded 429s (rate limits, monthly/daily quotas) remain `backpressure` until the gateway codes them. The CLI-authored remedy line for `dev keys create` is added at the command layer (`withTierLimitRemedy` in `helpers.go`) in every mode except JSON, so the JSON `error` value stays the gateway's message.

## Complete Command Reference

### Global Flags
- `-o, --output` — Output format: text (default), json, csv, markdown
- `-h, --help` — Help for any command
- `--version` — Print version with commit hash and build date. With `--output json` emits `{version, commit, built}` as structured JSON; plain-text format is preserved when `--output` is absent or `text`.
- `andamio help exit-codes` — Help topic documenting the exit-code and kind contract above (`cmd/andamio/exitcodes_help.go`).

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
| `course owner teachers --course-id <id> --alias <owner> --skey <path>` | `/v2/tx/course/owner/teachers/manage` | jwt | Add/remove teachers. **On-chain transaction** (`teachers_update`) — runs the full build→sign→submit→register→poll lifecycle, so it needs `--skey`, `--alias` (yours) and a configured submit URL. `--add` / `--remove` (repeatable), `--no-wait`, `--timeout` |
| `course teacher register-module` | `/v2/course/teacher/course-module/register` | jwt | Register module from chain. Idempotent on hash match: DRAFT advances to APPROVED; APPROVED/PENDING_TX/ON_CHAIN are no-ops. `--course-id`, `--module-code`, `--slt-hash`. `--output json` returns an envelope — see `register-module --help` for the shape. |
| `course teacher publish-module` | `/v2/course/teacher/course-module/publish` | jwt | Publish module. `--course-id`, `--module-code`. Warns on stderr only when the response shows the module is not linked on-chain (`module_status` not `ON_CHAIN` and no `slt_hash`) — the response is a `CourseModuleEntity`, which never carries `source` (#158) |
| `course teacher delete-module` | `/v2/course/teacher/course-module/delete` | jwt | Delete module. `--course-id`, `--module-code` |
| `course teacher update-module-status` | `/v2/course/teacher/course-module/update-status` | jwt | Update module status. `--course-id`, `--module-code`, `--status` |
| `course teacher review` | `/v2/course/teacher/assignment-commitment/review` | jwt | Review commitment. `--course-id`, `--module-code`, `--participant-alias`, `--decision` (accept/refuse) |
| `course teacher commitments` | `/v2/course/teacher/assignment-commitments/list` | jwt | List pending reviews. `--course-id` |
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
| `project manager qualified-contributors --project-id <id>` | `/v2/project/manager/contributors/get-qualified` | jwt | List aliases qualified to commit (holds every prerequisite SLT). Capped at 500; JSON passes the gateway `data` payload through in snake_case (`project_id`, `aliases`, `total_count`, `truncated`, `status`). Wire shape pinned by `internal/client/testdata/v2-5-qualified-contributors-response.json` — the first decoder used camelCase tags and zeroed every field (#90). |
| `project task list <project-id>` | `/v2/project/manager/tasks/list` | jwt | List tasks (manager) |
| `project task get <index> --project-id <id>` | `/v2/project/manager/tasks/list` | jwt | Get task by index (filters from list) |
| `project task create <project-id>` | `/v2/project/manager/task/create` | jwt | Create task. Flags: --title, --lovelace, --expiration, --github-issue |
| `project task update <index> --project-id <id>` | `/v2/project/manager/task/update` | jwt | Update task fields. --project-id required |
| `project task delete <index> --project-id <id>` | `/v2/project/manager/task/delete` | jwt | Delete draft task. --project-id required |
| `project task export <project-id>` | `/v2/project/manager/tasks/list` | jwt | Export tasks to tasks/<slug>/ as Markdown |
| `project task import <project-id>` | `/v2/project/manager/task/create,update` | jwt | Import tasks from Markdown files. --dry-run supported |
| `project task verify-hash <project-id>` | `/v2/project/user/tasks/list` | either | Verify task hashes match computed hashes (diagnostic) |
| `project task compute-hash` | local | none | Compute task hash from `--content`, `--lovelace`, `--expiration`, `--token` flags or `--file`. No auth required |

### teacher — Assessment (the agent-drivable path)
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `teacher courses` | `/v2/course/teacher/courses/list` | jwt | List courses where you are a teacher |
| `teacher assignments list [--course <id>]` | `/api/v2/course/teacher/assignment-commitments/list` | jwt | List commitments awaiting review. Without `--course`, returns the lightweight summary (no nested `content`) |
| `teacher assignments get <course-id> <module-code> <student-alias>` | same, filtered client-side | jwt | Read one submission |
| `teacher assessment build` | `/api/v2/tx/course/teacher/assignments/assess` | jwt | Build an assessment transaction **without signing or submitting**. `--course-id`, `--alias` (teacher), `--decision <student>=<accept\|refuse>` (repeatable) or `--decisions-file` |

**Evidence decoding.** `teacher assignments list`/`get` add `content.evidence_text` — the submission rendered as Markdown via `tiptapToMarkdown`, the same converter course export uses. It is a **sibling** of `content.evidence`, never a replacement: the raw Tiptap document is hash-bearing (the on-chain commitment hash is computed over the normalized form) and must round-trip byte-for-byte. `evidence_text` is *absent*, not empty-string, when there is no evidence. `get` routes through `fetchTeacherAssignmentsList` so the two commands cannot diverge.

**No `--status` filter**, deliberately. `internal/client/testdata/v2-3-manager-commitments-list-response.md` records the decision to prefer `jq` on the JSON envelope over CLI-layer filtering, and to filter on `task_outcome` presence over enum-string matching since the enum grows new transient values. That decision applies here too.

**Assessment payload naming is a trap.** Top-level `alias` is the **teacher**; `assignment_decisions[].alias` is the **student**. Two fields, same name, two different people. There is no `module_code` at any level — the protocol derives the module from the on-chain commitment. Schema reference: `.claude/skills/assess-assignment/SKILL.md`.

**Inspection is a request-echo, not a CBOR decode.** `AssessmentBuildEnvelope` carries the decision set alongside the unsigned transaction so a reviewer sees both from one command, instead of trusting an agent's separate prose summary of what opaque CBOR encodes. It proves what was *asked for*, not what the gateway *built*. Decoding the transaction needs Plutus datum interpretation and is out of scope for 1.0 — the limitation is stated in the command help and the type doc, not papered over.

Duplicate aliases are **rejected**, not last-wins: two conflicting outcomes for one learner means the caller lost track of its own decision set, and silently picking one would put a credential decision on-chain that nobody made.

### manager — Project manager role group
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `manager projects` | `/api/v2/project/manager/projects/list` | jwt | List projects where you are a manager |

**Top-level `manager` is deliberate, not a stray.** It parallels top-level `teacher`: both answer "what do I hold this role on?" and are the entry point to a role's workflow. The nested `project manager *` subgroup (commitments, qualified-contributors) operates *within* one project you already know the ID of. `cmd/andamio/project_manager_ops.go` records the decision — "the existing top-level `manager` command stays as-is" — so don't consolidate them without revisiting that. There is no `project manager list`; `manager projects` is the only way to discover the projects you manage.

### token — Native asset token registry
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `token list` | `/api/v2/token/user/tokens/list` | either | List registered tokens available as task rewards |

Supplies the `policy_id` / `asset_name` values for `project task create --token "<policy_id>,<asset_name>,<quantity>"`. Text output is a ticker/policy/asset/decimals table; `--output json` passes the gateway envelope through. Tolerates both `{data: [...]}` and a bare array from the gateway.

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
| `dev login` (no args) | browser flow → `/v2/auth/developer/login/session` + `/v2/auth/developer/login/complete` | api-key | **Browser-wallet developer login.** Opens browser to `{appURL}/auth/dev-cli`, user connects Eternl/Lace/Nami and signs in-browser; CLI receives JWT + 30-day refresh token via ephemeral localhost callback. Same flow used to claim API keys at app.andamio.io. Requires `auth login --api-key` first (dual-credential surface). 5-min timeout. |
| `dev login --skey <path> --alias <name> --address <bech32>` | `/v2/auth/developer/login/session` + `/v2/auth/developer/login/complete` | api-key | Headless CIP-30 signature-verified developer login (CI/CD, ops, devkit). Mints a 60-min RS256 developer JWT + 30-day rotation refresh token. All three flags required. Required for `/v2/keys` and other developer-portal endpoints. |
| `dev refresh` | `/v2/auth/developer/token/refresh` | dev-jwt-rotation (refresh token) | Rotate the developer JWT using the stored refresh token. Single-use rotation server-side; both tokens update atomically. 401 → re-run `dev login`. |
| `dev logout` | local | none | Clear entire dev slot (JWT, refresh token, alias, ID, tier, key hash). Does not affect user JWT. |
| `dev status` | local | none | Show developer auth status — JWT expiry, refresh-token expiry, tier. JSON envelope surfaces `jwt_expires_at` / `refresh_token_expires_at` / `*_expired` / `*_remaining_seconds` for scriptable branching. `*_remaining_seconds` is always present (no `omitempty`): zero means "sub-second remaining" (refresh now); branch on `*_expired` to disambiguate "fully expired" from "not parseable". Branch on `dev_authenticated` first. |
| `dev keys list` | `GET /v2/keys` | dev-jwt | List developer API keys across mainnet + preprod, unified. JSON passes through gateway `{keys: [...]}` envelope. |
| `dev keys create --name <label> --environment <mainnet\|preprod>` | `POST /v2/keys` | dev-jwt | Create a developer API key. Raw key value returned **exactly once** — text mode emits raw key on stdout + WARNING + metadata on stderr (so `\| pbcopy` captures key alone); JSON mode includes `key` field AND ALSO emits the WARNING on stderr so a human running `--output json` interactively still sees the one-time-use disclaimer (scripts pipe `2>/dev/null`). Errors: 422 `invalid_environment`, 429 `tier_limit_exceeded` (→ exit 7 / kind `tier_limit`, classified by code so a 403 classifies the same; text modes add a remedy line naming `dev keys list` / `dev keys delete <id>` / upgrade), 503 `preprod_routing_disabled`/`preprod_unavailable` — stable error codes preserved verbatim for script branching. |
| `dev keys delete <id>` | `DELETE /v2/keys/{id}` | dev-jwt | Revoke a developer API key by local UUID. 204 No Content on success. Malformed ids rejected client-side (UUID-format gate, error `invalid developer key id`) before reaching the gateway — closes URL-injection class (`?`, `..`, empty `$ID`). 404 covers both unknown ids and ids owned by other developers (gateway threat-model: indistinguishable). |

### spec — OpenAPI spec
| Command | Endpoint | Auth | Description |
|---------|----------|------|-------------|
| `spec fetch` | `/openapi/swagger.json` | none | Download OpenAPI spec to openapi.json |
| `spec paths [--filter <pattern>]` | local/remote | none | List API endpoints. Serves a local `openapi.json` when present and warns on stderr with its age (every output mode); falls back to `/openapi/swagger.json` |

## API

- Base URLs: `https://preprod.api.andamio.io` (default), `https://mainnet.api.andamio.io`
- Application paths start with `/api/v1/` or `/api/v2/`. The rendered OpenAPI
  document is the one exception: it is served at `/openapi/swagger.json`, outside
  the versioned prefixes (andamio-api#652 removed the previous
  `/api/v1/docs/doc.json` on 2026-07-28).
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
| Concepts | `CONCEPTS.md` | Shared domain vocabulary — SLTs, course modules, module status, content-derived identity. Relevant when orienting to the codebase or discussing domain concepts. |

## Planned Features

- **Content Sync** (`sync pull`/`sync push`/`sync status`) — Bidirectional course content sync with conflict detection. Design in `docs/PLAN-content-sync.md`.

## Cross-Repo Context

This CLI is part of the Andamio developer toolchain:

| Repo | Relationship |
|------|-------------|
| **andamio-docs** (`andamio-docs`) | CLI docs live at `content/docs/apps-tooling/cli/` — `index.mdx` (the command reference) and `import-format.mdx`. Note the API transaction specs under `public/yaml/transactions/v2/course/student/**` and `.../project/contributor/**` are **not** stale: 1.0 retired the CLI commands, not the gateway routes. Post-1.0 update tracked in `Andamio-Platform/andamio-docs#64`. |
| **andamio-lesson-coach-v2** (`andamio-lesson-coach-v2`) | Creates course content that this CLI reads and will eventually sync. Compiles modules to import-ready format. |
| **andamio-app-template** (`andamio-app-template`) | Forkable Next.js starter. CLI and template are parallel developer entry points — CLI for terminal users, template for UI builders. |
| **andamio-api** (`andamio-api`) | Go gateway that serves all endpoints this CLI consumes. Base URLs: preprod.api.andamio.io, mainnet.api.andamio.io. |

The developer journey: get API key → install CLI or fork template → explore courses → use coach to create content → push back via CLI.

## Skills

- `/getting-started` — Interactive walkthrough of CLI capabilities for new developers
- `/release` — Cut a new release with preflight checks
