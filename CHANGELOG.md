# Changelog

All notable changes to `andamio-cli` are documented in this file.

The format follows [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/). Dates use `YYYY-MM-DD`. Version numbers follow [Semantic Versioning](https://semver.org/spec/v2.0.0.html) — but note that the CLI is pre-1.0 and small breaking changes may ship in minor versions. **Breaking changes to the `--output json` envelope shape are called out explicitly** so scripts and agents can audit their integrations before upgrading.

## [Unreleased]

### Added
- `CHANGELOG.md` at the repo root as the source of truth for user-facing release notes (#67).
- `andamio --version --output json` emits `{version, commit, built}` as structured JSON. Plain-text `--version` output is unchanged (#67).
- `scripts/release.sh` preflight check that warns when the target version has no heading in `CHANGELOG.md` before tagging (#67).
- `apierr.ConflictError` typed error for HTTP 409 responses. Surfaced by `internal/client.Get`/`Post`/`Put` so callers can use `errors.As(err, &conflict)` instead of string-matching gateway error bodies (#64, #68).

### Changed
- **Breaking (`--output json` consumers of `course teacher register-module`):** the response is now wrapped in an envelope `{action, status, slt_hash, advanced_from, response}`. Gateway fields that were previously returned at the top level are now nested under `.response`. Scripts that branch on gateway fields must read them under `.response.*`. The `action` field (`"registered"` / `"advanced"` / `"already_registered"`) is the new branching key (#57, #63).
- `course teacher register-module` is now idempotent on `slt_hash` match. A duplicate-module 409 from the gateway no longer fails: if the existing module is in `DRAFT` with a matching hash, it is advanced to `APPROVED`; if already `APPROVED`/`PENDING_TX`/`ON_CHAIN` with a matching hash, it exits 0 as a no-op; hash mismatches still exit non-zero and point at `delete-module` (#57, #63).
- `isModuleAlreadyExistsError` migrated from pure string matching to a three-gate check: `errors.As(*apierr.ConflictError)` + `"already exists"` body substring + `"course_module_code"` body substring. Closes out the tech-debt TODO left by #63 and applies Prevention Strategy #2 from `docs/solutions/feature-implementations/cli-course-module-management-commands.md` (#64, #68).

## [0.11.2] - 2026-04-08

### Fixed
- `slt_hash` lookup now finds the hash when it's nested under `content.course_module_code` in the API response, restoring correct behavior for courses with merged on-chain modules.
- Task content larger than 64 bytes is now serialized with `PlutusTx` chunked CBOR encoding, matching the contract-side expectation for long task payloads.

## [0.11.1] - 2026-04-07

### Fixed
- Lesson, intro, and assignment fetches now use the teacher endpoint for draft modules. The user-scoped GET endpoints silently return empty for unpublished content; the teacher endpoint returns both draft and on-chain modules.

## [0.11.0] - 2026-04-06

### Added
- `project task compute-hash` and `course credential compute-hash` commands for computing task/SLT hashes locally without requiring authentication.
- `compute-hash` is also now accessible as a verification step in the `verify-hash` flows.
- Tables are now supported when importing lesson/assignment Markdown into Tiptap — multi-column content renders correctly in the app after import.

### Fixed
- `project_credential_claim` TX builds now correctly resolve `task_hash` when the task was registered from an on-chain source.

### Changed
- `draft-before-mint` workflow: `course import --create` explicitly marks new modules as `DRAFT` until the on-chain mint is confirmed, replacing the previous implicit flow. `task_hash` metadata is now attached automatically during draft creation.

## Earlier releases

Release notes for `v0.11.0` and earlier are available on GitHub Releases: <https://github.com/Andamio-Platform/andamio-cli/releases>. Tags go back to `v0.5.0` (2026-03-20). Noteworthy themes across the early releases:

- **v0.10.x** (2026-03-31 → 2026-04-03) — `compute-hash` groundwork, project task lifecycle improvements.
- **v0.9.x** (2026-03-24 → 2026-03-27) — project task management commands, commitment flows.
- **v0.8.x** (2026-03-23) — `andamio course export`/`import` improvements, `course create` scaffolding.
- **v0.6.0 → v0.7.0** (2026-03-21 → 2026-03-23) — early content-import work, goldmark/Tiptap converter.
