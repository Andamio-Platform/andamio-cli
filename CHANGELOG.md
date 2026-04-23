# Changelog

All notable changes to `andamio-cli` are documented in this file.

The format follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/). Dates use `YYYY-MM-DD`. Version numbers follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html) — but note that the CLI is pre-1.0 and small breaking changes may ship in minor versions. **Breaking changes to the `--output json` envelope shape are called out explicitly** so scripts and agents can audit their integrations before upgrading.

## [Unreleased]

### Added
- `CHANGELOG.md` at the repo root as the source of truth for user-facing release notes (#67).
- `andamio --version --output json` emits `{version, commit, built}` as structured JSON. `commit` is the 7-character short form matching the plain-text `--version` output; `built` is the verbatim ldflag-injected timestamp (RFC3339 UTC for goreleaser builds; `"unknown"` for dev builds without ldflags). Plain-text `--version` output now uses the same 7-character short commit for both release and dev builds (#67).
- `scripts/release.sh` preflight check that warns when the target version has no heading in `CHANGELOG.md` before tagging (#67).
- `apierr.ConflictError` typed error for HTTP 409 responses. Surfaced by `internal/client.Get`/`Post`/`Put` so callers can use `errors.As(err, &conflict)` instead of string-matching gateway error bodies (#64, #68).
- `apierr.ServerError` typed error for HTTP 5xx responses. Carries the raw status code so retry classifiers can branch on server-side failure without parsing error strings (#65).
- `apierr.BackpressureError` typed error for HTTP 408/425/429. Carries the parsed `Retry-After` hint when present so retry backoff can honor server-supplied pacing (#65).
- `Client.PostWithRetry` for opt-in bounded retries on idempotent POST calls. Retries transient network errors, 5xx responses, and 408/425/429 backpressure signals (honoring `Retry-After` when present); never retries 4xx semantic failures (401/403/404/409). Default schedule: 3 attempts total with exponential backoff + ±20% jitter (#65).
- `Client.SetOnRetry` for registering a callback fired between retry attempts. Lets the cobra layer log "retrying..." progress to stderr without `internal/client` taking a dependency on `internal/output` (#65).
- `RegisterModuleEnvelope` typed struct is the single source of truth for the `course teacher register-module` `--output json` contract. Replaces three hand-rolled `map[string]interface{}` literals that previously drifted against the docstring and Long help. Scripts should branch on `.action`; `.status` and `.slt_hash` now reflect gateway canonical values on the `already_registered` branch (#66).

### Changed
- **Breaking (`--output json` consumers of `course teacher register-module`):** the response is now wrapped in an envelope `{action, status, slt_hash, advanced_from, response}`. Gateway fields that were previously returned at the top level are now nested under `.response`. Scripts that branch on gateway fields must read them under `.response.*`. The `action` field (`"registered"` / `"advanced"` / `"already_registered"`) is the new branching key (#57, #63).
- `course teacher register-module` is now idempotent on `slt_hash` match. A duplicate-module 409 from the gateway no longer fails: if the existing module is in `DRAFT` with a matching hash, it is advanced to `APPROVED`; if already `APPROVED`/`PENDING_TX`/`ON_CHAIN` with a matching hash, it exits 0 as a no-op; hash mismatches still exit non-zero and point at `delete-module` (#57, #63).
- `isModuleAlreadyExistsError` migrated from pure string matching to a three-gate check: `errors.As(*apierr.ConflictError)` + `"already exists"` body substring + `"course_module_code"` body substring. Closes out the tech-debt TODO left by #63 and applies Prevention Strategy #2 from `docs/solutions/feature-implementations/cli-course-module-management-commands.md` (#64, #68).
- `internal/client` methods (`Get`/`Post`/`Put`) now take `context.Context` as their first argument. Cobra handlers pass `cmd.Context()` so Ctrl-C cancels in-flight gateway requests instead of waiting for the 30s wall-clock timeout. Root-level `signal.NotifyContext` wiring in `main.go` ensures every subcommand inherits a cancellable context (#65).
- `register-module` recovery now tolerates transient gateway failures. The teacher-modules-list lookup that runs after a 409 conflict retries up to 3 times on 5xx or network errors before giving up. Users may see a brief "retrying..." note on stderr during recovery (suppressed in `--output json` mode); scripts that strictly parse stderr content should note the possible new lines (#65).
- **Cosmetic JSON ordering (`course teacher register-module` with `--output json`):** envelope keys now emit in declaration order (`action, status, slt_hash, advanced_from, response`) rather than alphabetical order (`action, advanced_from, response, slt_hash, status`). Consumers parsing JSON with `jq` / `JSON.parse` see identical data; byte-for-byte output-diff consumers will see a diff. Same five keys, same types, same null semantics (#66).
- `course teacher register-module` `--output json` envelope's `slt_hash` on the `already_registered` branch now reflects the canonical stored hash (from the teacher modules list) rather than echoing the supplied input. A caller that registers with `"ABC123"` against a module stored as `"abc123"` now sees `.slt_hash == "abc123"` in the envelope, matching what `course modules --output json` returns for the same module. The `registered` and `advanced` branches continue to fall back to the supplied hash when the gateway response is minimal (today's observed behavior) — the asymmetry is documented; full gateway-state population on those branches lights up automatically when real preprod fixtures land (#66, todo #021).

## [0.11.2] - 2026-04-08

### Fixed
- `slt_hash` lookup now finds the hash when it's nested under `content.course_module_code` in the API response, restoring correct behavior for courses with merged on-chain modules.
- Task content larger than 64 bytes is now serialized with `PlutusTx` chunked CBOR encoding, matching the contract-side expectation for long task payloads.

## [0.11.1] - 2026-04-07

### Fixed
- Lesson, intro, and assignment fetches now use the teacher endpoint for draft modules. The user-scoped GET endpoints silently return empty for unpublished content; the teacher endpoint returns both draft and on-chain modules.

## [0.11.0] - 2026-04-06

### Added
- Tables are now supported when importing lesson/assignment Markdown into Tiptap — multi-column content renders correctly in the app after import.

### Fixed
- `project_credential_claim` TX builds now correctly resolve `task_hash` when the task was registered from an on-chain source.

### Changed
- `draft-before-mint` workflow: `course import --create` explicitly marks new modules as `DRAFT` until the on-chain mint is confirmed, replacing the previous implicit flow. `task_hash` metadata is now attached automatically during draft creation.

## [0.10.2] - 2026-04-03

### Added
- `project task compute-hash` and `course credential compute-hash` commands for computing task/SLT hashes locally without requiring authentication.
- `compute-hash` is also accessible as a verification step in the `verify-hash` flows.

## Earlier releases

Release notes for `v0.10.1` and earlier are available on GitHub Releases: <https://github.com/Andamio-Platform/andamio-cli/releases>. Tags go back to `v0.5.0` (2026-03-20). Noteworthy themes across the early releases:

- **v0.10.0 → v0.10.1** (2026-03-31 → 2026-04-01) — project task lifecycle improvements, hash-related fixes.
- **v0.9.x** (2026-03-24 → 2026-03-27) — project task management commands, commitment flows.
- **v0.8.x** (2026-03-23) — `andamio course export`/`import` improvements, `course create` scaffolding.
- **v0.6.0 → v0.7.0** (2026-03-21 → 2026-03-23) — early content-import work, goldmark/Tiptap converter.
