---
title: "Fix three CLI issues: hex-encode asset_name, merge existing lessons on import, headless .skey login"
date: 2026-03-21
category: logic-errors
tags:
  - hex-encoding
  - asset-name
  - token-flag
  - import
  - lessons
  - merge
  - headless-login
  - skey
  - cip-8
  - cose-sign1
  - cbor
  - ci-cd
components:
  - cmd/andamio/helpers.go
  - cmd/andamio/project_task.go
  - cmd/andamio/project_task_export.go
  - cmd/andamio/project_task_import.go
  - cmd/andamio/project_task_test.go
  - cmd/andamio/course_import.go
  - cmd/andamio/user.go
  - internal/cardano/sign.go
symptoms:
  - "--token flag with UTF-8 asset_name caused 502 errors from the API"
  - "Importing partial lesson files silently deleted existing lessons not present locally"
  - "user login required browser + CIP-30 wallet, blocking CI/CD and scripted workflows"
root_cause: "parseTokenFlags passed asset_name as UTF-8 instead of hex; updateModuleContent sent only local lessons causing replace-all deletion; no headless auth path existed"
severity: high
pr: "#34"
issues: ["#28", "#8", "#29"]
time_to_resolve: "4 hours"
---

# Fix Three CLI Issues: Hex-Encode Asset Name, Merge Existing Lessons, Headless Login

## Problem

Three open issues blocked production use of the CLI:

1. **Issue #28 — `--token` flag 502 errors.** Running `andamio project task create <id> --token "policy.MyToken=1"` sent `MyToken` as UTF-8 to the API, which expects hex-encoded asset names. The API returned 502 because it could not match the token on-chain.

2. **Issue #8 — Import deletes existing lessons.** Running `andamio course import ./module-101` with only `lesson-1.md` and `lesson-3.md` present would delete lessons 2, 4, 5, etc. The API's update endpoint uses replace-all semantics: whatever lessons array is sent replaces all existing lessons for that module.

3. **Issue #29 — No headless authentication.** `andamio user login` required opening a browser, connecting a CIP-30 wallet, and signing a nonce interactively. This made the CLI unusable in CI/CD pipelines, cron jobs, and agent workflows.

## Root Causes

| Issue | What broke | Why |
|-------|-----------|-----|
| #28 | `parseTokenFlags()` in `project_task.go` | Passed `asset_name` string directly to API payload without hex-encoding |
| #8 | `updateModuleContent()` in `course_import.go` | Built the lessons array only from local files; API interprets the array as the complete replacement set |
| #29 | `user.go` login command | Only one auth path existed (browser OAuth); no alternative for non-interactive environments |

## Solution

### Fix 1: Hex-encode asset_name in --token flag (Issue #28)

Added three helpers to `cmd/andamio/helpers.go`:

- `isHex(s string) bool` — returns true if `s` is even-length and all hex characters.
- `hexEncodeAssetName(name string) string` — hex-encodes if not already hex; passes through empty strings.
- `hexDecodeAssetName(name string) string` — attempts hex decode to UTF-8; returns original on failure.

Applied hex encoding in three locations:

| Location | Direction | Function |
|----------|-----------|----------|
| `parseTokenFlags()` in `project_task.go` | Encode on input | CLI flag values hex-encoded before API call |
| Task import in `project_task_import.go` | Encode on input | Markdown frontmatter values hex-encoded before API call |
| Task export in `project_task_export.go` | Decode on output | API hex values decoded to readable UTF-8 in exported Markdown |

**Heuristic trade-off:** `isHex` treats even-length valid hex as "already encoded." This means an asset name like `BEEF` (4 chars, valid hex) would pass through unchanged even if it was intended as UTF-8. This is documented and accepted because:
- Real-world Cardano asset names that are valid hex are already hex-encoded by convention.
- The alternative (always encoding) would double-encode assets from the API.

Test coverage in `project_task_test.go`:
- `TestParseTokenFlags` — verifies hex encoding through the flag parser.
- `TestHexEncodeAssetName` — unit tests for encode edge cases (empty, already-hex, UTF-8, emoji).
- `TestHexDecodeAssetName` — unit tests for decode edge cases (invalid hex, non-UTF-8 bytes).
- `TestHexRoundTrip` — encode then decode produces the original string.

### Fix 2: Merge existing lessons on import (Issue #8)

Changed `updateModuleContent()` in `course_import.go` to merge local files with existing API state instead of replacing wholesale.

The new logic:

1. Fetch existing module data from API (already done for metadata preservation).
2. Build `localByIndex` map from local lesson files.
3. Validate bounds: skip any `lesson-N.md` where N exceeds the module's SLT count, with a stderr warning.
4. Merge loop iterates `1..SLTCount`:
   - If a local file exists for index `i`, use it (local takes precedence).
   - Otherwise, preserve the existing API lesson for that index.
   - Print `Preserved existing lesson for SLT N (no local file)` to stderr.
5. When `SLTCount == 0` (new module with no existing SLTs), skip the merge loop and use local lessons directly. This handles the first-import case where there is nothing to merge with.

```
Before:  lessons = [local files only]         → API deletes unlisted lessons
After:   lessons = [local files + API gaps]   → API preserves all lessons
```

Metadata fields (`description`, `image_url`, `video_url`) from existing lessons are also preserved when a local file provides new `content_json` but no metadata.

### Fix 3: Headless login with .skey (Issue #29)

Added `--skey` and `--alias` flags to `user login`:

```
andamio user login --skey ./payment.skey --alias myalias
```

**Implementation in `cmd/andamio/user.go`:**

The `runUserLogin` function checks for `--skey`; if present, delegates to `runHeadlessLogin()` which implements a four-step flow:

1. `POST /api/v2/auth/login/session` — get a nonce and session ID.
2. Sign the nonce with `cardano.SignMessage()` — produces CIP-8 COSE_Sign1 + COSE_Key.
3. `POST /api/v2/auth/login/validate` — send session_id + signature + key for verification.
4. Store the returned JWT, alias, and expiry in config.

**CIP-8 signing in `internal/cardano/sign.go`:**

`SignMessage()` builds a standards-compliant CIP-8/CIP-30 message signature:

| COSE component | Structure |
|----------------|-----------|
| Protected headers | `{ 1: -8 (EdDSA), "address": blake2b-224(pubKey) }` |
| SigStructure | `["Signature1", protected, b"", message]` |
| COSE_Sign1 | CBOR Tag 18 wrapping `[protected, {}, message, signature]` |
| COSE_Key | `{ 1: 1 (OKP), 3: -8 (EdDSA), -1: 6 (Ed25519), -2: pubKey }` |

The API receives `{ session_id, signature: { signature: <hex>, key: <hex> } }` and validates against the nonce it issued.

**Key implementation detail — canonical CBOR encoding:**

The protected headers map uses `cbor.EncOptions{Sort: cbor.SortCanonical}` for deterministic byte ordering. Without this, Go's map iteration order is non-deterministic, which would produce different protected header bytes on each run. Since the signature covers the protected headers, non-deterministic ordering would cause intermittent validation failures.

**Error UX:**

`--alias` is required with `--skey`. The error message includes a discovery hint:

```
--alias is required with --skey

Check aliases with: andamio user exists <alias>
```

**JSON output support:**

When `--output json` is set, headless login prints a structured result:

```json
{
  "alias": "myalias",
  "expires_at": "2026-03-22T...",
  "key_hash": "abc123..."
}
```

All progress messages are gated with `if !isJSON` and written to stderr, following the CLI's composability rules.

## Review Findings Also Fixed

During code review of the initial implementation, four additional issues were identified and resolved:

1. **CBOR map ordering non-determinism.** The initial `SignMessage()` implementation used default CBOR encoding for the protected headers map, which does not guarantee byte ordering. Changed to `cbor.SortCanonical` to produce deterministic output required for signature verification.

2. **Dead variable.** An intermediate `coseSign1` variable was assigned but never used after refactoring the Tag 18 wrapping. Removed.

3. **SLTCount==0 edge case.** The merge loop `for i := 1; i <= existing.SLTCount; i++` produces no iterations when `SLTCount` is 0 (new module). Added a fallback that uses local lessons directly when the API reports zero SLTs.

4. **`--alias` error message.** The initial error was a bare `"--alias is required with --skey"`. Added a discovery hint pointing to `andamio user exists <alias>` so the user knows how to find valid aliases.

## Verification

```bash
# Issue #28: hex encoding round-trip
go test ./cmd/andamio/ -run TestHexEncodeAssetName -v
go test ./cmd/andamio/ -run TestHexDecodeAssetName -v
go test ./cmd/andamio/ -run TestHexRoundTrip -v
go test ./cmd/andamio/ -run TestParseTokenFlags -v

# Issue #8: import with partial lessons (manual — requires API access)
# 1. Export a module with 5 lessons
# 2. Delete lesson-2.md and lesson-4.md locally
# 3. Import — verify lessons 2 and 4 are preserved, not deleted
andamio course import ./module-101 --course-id <id> --dry-run

# Issue #29: headless login (requires .skey file and valid alias)
andamio user login --skey ./payment.skey --alias <alias>
andamio user status
andamio user login --skey ./payment.skey --alias <alias> --output json
```

## Files Changed

| File | Changes |
|------|---------|
| `cmd/andamio/helpers.go` | Added `isHex()`, `hexEncodeAssetName()`, `hexDecodeAssetName()` |
| `cmd/andamio/project_task.go` | `parseTokenFlags()` calls `hexEncodeAssetName()` |
| `cmd/andamio/project_task_export.go` | Task export calls `hexDecodeAssetName()` for readable output |
| `cmd/andamio/project_task_import.go` | Task import calls `hexEncodeAssetName()` on frontmatter values |
| `cmd/andamio/project_task_test.go` | Added hex encode/decode/round-trip and parseTokenFlags tests |
| `cmd/andamio/course_import.go` | Lesson merge logic with SLTCount fallback and bounds validation |
| `cmd/andamio/user.go` | `--skey`/`--alias` flags, `runHeadlessLogin()` function |
| `internal/cardano/sign.go` | `SignMessage()` for CIP-8 COSE_Sign1, `MessageSignResult` struct |
