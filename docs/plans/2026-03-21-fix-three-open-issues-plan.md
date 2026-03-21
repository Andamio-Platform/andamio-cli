---
title: "Fix three open issues: headless login, hex-encode asset_name, ON_CHAIN lesson creation"
type: fix
status: completed
date: 2026-03-21
origin: docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md
---

# Fix Three Open Issues

Addresses all three open GitHub issues in a single implementation pass, ordered by complexity (simplest first).

## Overview

| # | Issue | Type | Effort | Files |
|---|-------|------|--------|-------|
| 28 | CLI should hex-encode asset_name in --token flag | Bug fix | Small | 3 files |
| 8 | ON_CHAIN import skips new lessons for empty SLT slots | Bug fix | Medium | 1-2 files |
| 29 | Headless login: authenticate with .skey | Feature | Large | 2-3 files + new dependency |

---

## Issue #28: Hex-encode asset_name in --token flag

### Problem Statement

The `--token` flag accepts `"policy_id,asset_name,quantity"` and passes `asset_name` as-is to the API. The DB API validates with `isValidHex(assetName, 2, 64)`, so passing `"XP"` (UTF-8) causes a 502. The correct value is `"5850"` (hex-encoded).

### Proposed Solution

Auto-detect and hex-encode `asset_name` in `parseTokenFlags()`. If the value is already valid hex, pass through. If it's human-readable text, hex-encode it.

### Technical Details

**Detection logic:** A value is "already hex" if it consists entirely of `[0-9a-fA-F]` characters AND has even length. Otherwise, treat as UTF-8 and hex-encode.

**Affected files:**

1. `cmd/andamio/project_task.go` — `parseTokenFlags()` (line 328): Add hex encoding after `assetName := strings.TrimSpace(parts[1])`

```go
// Hex-encode asset_name if not already hex (Cardano expects hex on-chain)
if assetName != "" && !isHex(assetName) {
    assetName = hex.EncodeToString([]byte(assetName))
}
```

2. `cmd/andamio/project_task_import.go` — After parsing frontmatter tokens (line 249): Apply same encoding to `fm.Tokens[i].AssetName`

3. `cmd/andamio/project_task_export.go` — Task export (line 182-201): Decode hex asset_name back to human-readable in exported YAML frontmatter, so round-trip works: `export (hex→text) → edit → import (text→hex)`

**Helper function** (in `helpers.go`):
```go
func isHex(s string) bool {
    if len(s) == 0 || len(s)%2 != 0 {
        return false
    }
    _, err := hex.DecodeString(s)
    return err == nil
}
```

**Edge cases:**
- Empty `asset_name` → pass through (empty is valid for ADA-only policies)
- Already-hex value like `"5850"` → detected as hex, passed through unchanged
- Ambiguous values (e.g., `"ABCD"`) → detected as hex (even length, valid hex chars). This is correct: if someone passes literal "ABCD" meaning the text string, they should be aware it looks like hex. Document this in help text.
- Unicode characters → hex-encoded as UTF-8 bytes

**Test updates:** `cmd/andamio/project_task_test.go` — Add test cases for hex encoding, pass-through, and round-trip.

### Acceptance Criteria

- [x] `--token "722c...,XP,50"` sends `asset_name: "5850"` to API
- [x] `--token "722c...,5850,50"` sends `asset_name: "5850"` (pass-through)
- [x] `--token "722c...,,50"` sends empty asset_name (pass-through)
- [x] Task export decodes hex to human-readable in YAML
- [x] Task import from YAML hex-encodes non-hex asset names
- [x] Existing tests updated, new test cases added
- [x] Help text updated: `"policy_id,asset_name,quantity"` → note that asset_name is auto-hex-encoded

---

## Issue #8: ON_CHAIN upsert skips new lessons for empty SLT slots

### Problem Statement

When re-importing a module that is `ON_CHAIN`, lesson files for SLT slots that previously had no lesson are silently skipped. Only existing lessons get updated. The user sees `Lessons: 3 found, Lessons updated: 2` with no explanation of why lesson 3 was dropped.

### Root Cause (from SpecFlow analysis)

The bug is **not** that the API rejects new lessons. The bug is the **replace-all semantic** of the lessons array combined with partial local files.

The API contract states: "array items (lessons, slts) replace the full entity." When a user re-imports with only new lesson files (e.g., `lesson-4.md` and `lesson-5.md`), the CLI sends `lessons: [{slt_index: 4, ...}, {slt_index: 5, ...}]`. The API interprets this as "replace all lessons with these 2" — **deleting the existing lessons for SLTs 1-3**.

The actual scenario from the issue (only 2 of 3 lesson files present) suffers the same problem: the CLI sends only the files on disk, and the API replaces the full set.

**Key insight:** `updateModuleContent()` (line 1118) builds lessons only from `data.Lessons` (local files). The `existing.Lessons` map (fetched from API at line 1094) is consulted only for metadata fallback within each local lesson — never to preserve lessons that have no local file.

### Proposed Solution

**Merge existing API lessons with local file lessons.** For each SLT index from 1 to `existing.SLTCount`:
- If a local lesson file exists for that index → use it (create or update)
- If no local file exists but an existing lesson exists on the API → include the existing lesson unchanged (preserve)
- If neither exists → skip (no lesson for that SLT)

This ensures the lessons array sent to the API always includes the full set, preventing silent deletion.

**Implementation in `updateModuleContent()`** (course_import.go:1116):

```go
// Build merged lessons: local files take precedence, existing API lessons fill gaps
localByIndex := map[int]map[string]interface{}{}
for _, lesson := range data.Lessons {
    l := map[string]interface{}{
        "slt_index":    lesson.Index,
        "content_json": lesson.TiptapJSON,
    }
    // ... title and metadata as before
    localByIndex[lesson.Index] = l
}

var lessons []map[string]interface{}
for i := 1; i <= existing.SLTCount; i++ {
    if local, ok := localByIndex[i]; ok {
        lessons = append(lessons, local)  // from disk
    } else if existingLesson, ok := existing.Lessons[i]; ok {
        lessons = append(lessons, existingLesson)  // preserve from API
    }
}
```

**Additional improvements:**

1. **Bounds validation**: Warn and skip lesson files with index > `SLTCount`
2. **Accurate reporting**: Distinguish "updated" (local file replaced existing), "created" (local file for empty SLT), and "preserved" (existing lesson kept, no local file)
3. **Design decision**: For ON_CHAIN modules, always merge (safe). For DRAFT modules, also merge — deleting lessons should require an explicit action, not happen accidentally from a partial file set

### Affected files

- `cmd/andamio/course_import.go` — `updateModuleContent()` (line 1116+): Add pre/post validation and accurate reporting
- `cmd/andamio/course_import.go` — Summary output section: Update counts to distinguish created vs updated

### Acceptance Criteria

- [x] Re-importing with new lesson files for empty SLT slots either creates them or warns clearly
- [x] Import summary distinguishes "updated" vs "created" vs "skipped" lessons
- [x] If API rejects new lessons, a clear warning with guidance is shown
- [x] `--output json` includes the breakdown in structured format
- [x] No regression on existing import flows (DRAFT modules, full lesson sets)

---

## Issue #29: Headless login with .skey

### Problem Statement

`andamio user login` requires a browser + CIP-30 wallet extension. This blocks CI/CD, scripted test scenarios, agents, and multi-wallet testing. The issue references CF demo prep where 5 funded test wallets couldn't authenticate without 5 browser sessions.

### Context from Brainstorm

Found brainstorm from 2026-03-18: CLI Wallet Signing. The brainstorm covers transaction signing (already implemented as `tx sign`). Key decisions carried forward:
- `.skey` files as the key format (standard `cardano-cli` envelope)
- Bursa for key loading (already a dependency)
- No stored key state — `--skey` passed on every invocation
- Ed25519 signing via Go stdlib (see brainstorm: `docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md`)

### Proposed Solution

Add `--skey` and `--alias` flags to `user login`. When `--skey` is provided, use a headless CIP-8 flow instead of browser-based auth.

**Flow:**

```
1. andamio user login --skey ./payment.skey --alias otter
2. CLI loads .skey via Bursa → gets ed25519 private key + public key
3. CLI derives key hash (blake2b-224 of public key) — already have blake2b224()
4. CLI calls POST /v2/auth/login/session → gets { nonce }
5. CLI signs nonce using CIP-8 message signing format
6. CLI calls POST /v2/auth/login/complete with { key_hash, public_key, signature, alias }
7. API verifies signature, returns { jwt, expires_at, alias, user_id }
8. CLI stores JWT in config (same as browser flow)
```

### Technical Details

**CIP-8 Message Signing:**

CIP-8 (message signing) differs from transaction signing:
- Transaction signing: Blake2b-256(tx_body_bytes) → ed25519.Sign(hash)
- CIP-8 signing: Build a `SigStructure` CBOR payload, then ed25519.Sign(Blake2b-256(SigStructure))

The `SigStructure` for CIP-8 `Signature1`:
```cbor
["Signature1", protected_headers, external_aad, payload]
```

Where:
- `protected_headers`: CBOR map with `{1: -8}` (EdDSA algorithm) and `"address"` header with key hash
- `external_aad`: empty bytes
- `payload`: the nonce bytes

**New function** in `internal/cardano/sign.go`:
```go
func SignMessage(message []byte, privKey ed25519.PrivateKey, pubKey ed25519.PublicKey) (signature, publicKeyHex, keyHashHex string, err error)
```

**Command changes** in `cmd/andamio/user.go`:

```go
// Add flags to existing loginCmd
userLoginCmd.Flags().String("skey", "", "Path to .skey file for headless authentication")
userLoginCmd.Flags().String("alias", "", "Andamio alias (required with --skey)")
```

In `runUserLogin`, branch on `--skey`:
```go
skeyPath, _ := cmd.Flags().GetString("skey")
if skeyPath != "" {
    return runHeadlessLogin(cfg, skeyPath, alias, isJSON)
}
// ... existing browser flow
```

**API dependency (MUST VERIFY FIRST):** The headless flow requires `POST /v2/auth/login/session` and `POST /v2/auth/login/complete` endpoints. Per the issue: "The API already has the nonce → sign → verify → JWT flow." Run `andamio spec fetch && andamio spec paths --filter auth` before implementing. If endpoints don't exist, this issue is blocked on API work.

**CI/CD consideration:** Support `--output json` on the login result so CI scripts can capture the JWT: `JWT=$(andamio user login --skey ./key.skey --alias dev1 --output json | jq -r .jwt)`. Config file is still written, but the JSON output enables env-var-based workflows.

### Affected files

- `cmd/andamio/user.go` — Add `--skey`/`--alias` flags, branch to headless flow, new `runHeadlessLogin()` function
- `internal/cardano/sign.go` — New `SignMessage()` function for CIP-8 message signing
- `internal/cardano/sign_test.go` — Tests for CIP-8 signing with known test vectors

### Edge Cases

- **Already authenticated**: Current browser flow exits with "Already authenticated as X. Run logout first." **Headless flow should bypass this check** — CI/CD scripts run on a schedule and can't call logout first. When `--skey` is provided, silently replace existing auth.
- **Missing alias**: `--alias` is required with `--skey`. Error: `"--alias is required with --skey"`.
- **Invalid .skey file**: Bursa's `LoadKeyFromFile` already returns clear errors.
- **API returns error on nonce request**: Wrap with `"failed to get login nonce: %w"`.
- **Nonce expired between request and signing**: Unlikely (signing is instant), but handle API error gracefully.
- **Wrong key for alias**: API will reject — surface the API error message.
- **Override with `--force`**: Consider adding `--force` to re-authenticate without logout. Or just: if `--skey` is provided and already authed, proceed anyway (overwrite).

### Acceptance Criteria

- [x] `andamio user login --skey ./payment.skey --alias dev1` authenticates without browser
- [x] JWT stored in config, `andamio user status` shows the session
- [x] `--output json` returns `{ "alias": "dev1", "expires_at": "..." }`
- [x] Progress messages to stderr, JWT never printed to stdout
- [x] Error messages are actionable: wrong key, expired nonce, API errors
- [x] CIP-8 signing produces valid signature that API accepts
- [x] Existing browser flow unchanged when `--skey` not provided
- [x] Works without a TTY (fully headless for CI/agents)

---

## Implementation Order

1. **#28 (hex-encode)** — Small, isolated fix. Ship first.
2. **#8 (ON_CHAIN lessons)** — Requires investigation to determine if API-side or CLI-side. Fix what's fixable, document what isn't.
3. **#29 (headless login)** — Largest. Depends on API endpoints existing. Validate first, then implement.

## Sources

- **Origin brainstorm:** [docs/brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md](../brainstorms/2026-03-18-cli-wallet-signing-brainstorm.md) — Key decisions: .skey via Bursa, no stored keys, ed25519 stdlib signing
- **Learnings:** docs/solutions/feature-implementations/project-task-token-flag.md — StringArray flag pattern for tokens
- **Learnings:** docs/solutions/integration-issues/cli-course-import-app-parity-and-payload-alignment.md — SLT locking, teacher endpoints, metadata preservation
- **Learnings:** docs/solutions/logic-errors/export-import-round-trip-title-preservation.md — Empty arrays replace all (API contract)
- **Learnings:** docs/solutions/security-issues/tx-signing-code-review-witness-drop-url-validation.md — CBOR error handling, never silently drop
- GitHub issues: #28, #8, #29
- Signing primitives: `internal/cardano/sign.go` — `LoadSigningKey`, `blake2b224`, `blake2b256`
- Auth flow: `cmd/andamio/user.go:96-236` — Browser-based login
- Token parsing: `cmd/andamio/project_task.go:317-365` — `parseTokenFlags()`
- Import logic: `cmd/andamio/course_import.go:1116-1172` — `updateModuleContent()`
