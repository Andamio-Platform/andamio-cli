---
title: "fix: Evidence submitted as hex-encoded plaintext instead of Tiptap JSON + hash"
type: fix
status: completed
date: 2026-03-23
---

# fix: Evidence submitted as hex-encoded plaintext instead of Tiptap JSON + hash

## Overview

The CLI's evidence submission commands send raw plaintext strings to the API. The API hex-encodes this text and stores it in `evidenceHash`, leaving the `evidence` column empty. The frontend sends evidence as Tiptap JSON with a proper content hash. The CLI needs to match the frontend's behavior: wrap evidence in Tiptap JSON, compute a Blake2b-256 content hash, and send both fields.

Ref: [GitHub Issue #36](https://github.com/andamio-platform/andamio-cli/issues/36)

## Problem Statement

Observed on staging (2026-03-22), TX `e006877c...`:

```json
{
  "studentAlias": "badger",
  "status": "SUBMITTED",
  "evidence": "",
  "evidenceHash": "4261646765722074657374696e6720666565646261636b"
}
```

`evidenceHash` decodes to ASCII: `"Badger testing feedback"` — the raw evidence text hex-encoded, not a hash.

**Root cause**: Three CLI commands send the `--evidence` flag value as a raw string. No Tiptap wrapping, no hash computation.

| Command | File | Lines |
|---------|------|-------|
| `course student submit` | `cmd/andamio/course_student.go` | 218-253 |
| `course student update` | `cmd/andamio/course_student.go` | 218-253 (shared handler) |
| `project contributor update` | `cmd/andamio/project_contributor.go` | 171-204 |

## Proposed Solution

**Approach: Fix the REST endpoints (not TX pipeline switch).**

Rationale:
- REST endpoints exist for convenience without wallet signing. Students may not have `.skey` files.
- Adding `--skey` to evidence submission would be a UX regression.
- The API should accept properly formatted `evidence` (Tiptap JSON) and `evidence_hash` (hex string) in the REST payload.
- If the REST endpoint doesn't accept `evidence_hash`, that's an API-side fix — the CLI should send the correct data regardless.

**Implementation steps:**

### Step 1: Export `blake2b256`

Rename `blake2b256` to `Blake2b256` in `internal/cardano/sign.go:169`. Update the one internal caller at line 65. This function is general-purpose (not Cardano-specific) and already in a package the CLI imports.

### Step 2: Create evidence wrapping helper

In `cmd/andamio/helpers.go` (or a new `evidence.go`), add:

```go
// wrapEvidence converts evidence text to Tiptap JSON and computes its content hash.
// Input is treated as Markdown and converted via markdownToTiptap.
// Returns (tiptapJSONString, blake2b256HexHash, error).
func wrapEvidence(text string) (string, string, error)
```

Logic:
1. Call `markdownToTiptap(text, nil)` to convert evidence text (treating as Markdown — supports URLs, lists, code blocks)
2. `json.Marshal` the Tiptap document (deterministic, compact JSON)
3. Compute `cardano.Blake2b256(jsonBytes)` and hex-encode
4. Return the JSON string and hex hash

Using `markdownToTiptap` over a simple paragraph wrapper because:
- Evidence often contains URLs (should become clickable links)
- Matches `--content-file` precedent in `project task create`
- The converter already handles all edge cases (unicode, special chars, GFM)

### Step 3: Update `runCourseStudentSubmitOrUpdate`

In `cmd/andamio/course_student.go:218-253`:

```go
evidence, _ := cmd.Flags().GetString("evidence")

// Wrap evidence as Tiptap JSON + compute hash
tiptapJSON, evidenceHash, err := wrapEvidence(evidence)
if err != nil {
    return fmt.Errorf("failed to format evidence: %w", err)
}

payload := map[string]interface{}{
    "course_id":          courseID,
    "course_module_code": moduleCode,
    "evidence":           tiptapJSON,
    "evidence_hash":      evidenceHash,
}
```

### Step 4: Update `runProjectContributorUpdate`

Same pattern in `cmd/andamio/project_contributor.go:171-204`.

### Step 5: Add `--evidence-file` flag

Follow the `--content-file` pattern from `project_task.go:627-642`:

```go
cmd.Flags().String("evidence-file", "", "Path to evidence file (Markdown)")
```

If set, read file contents and use as evidence text (mutually exclusive with `--evidence`). This was planned but never implemented — multi-line evidence with shell escaping is fragile.

### Step 6: Unit tests

In a new `cmd/andamio/evidence_test.go`:

| Test case | Input | Assertion |
|-----------|-------|-----------|
| Plain text | `"My evidence"` | Valid Tiptap doc with paragraph node, 64-char hex hash |
| URL | `"https://github.com/user/repo"` | Tiptap doc with link or autolink node |
| Markdown list | `"- item 1\n- item 2"` | Tiptap doc with bulletList node |
| Unicode | `"Evidence with emoji and CJK"` | Valid JSON, deterministic hash |
| Special chars | `"He said \"hello\" and \\ backslash"` | Properly escaped in JSON |
| Determinism | Same input twice | Identical hash both times |

## Technical Considerations

### Hash computation determinism

Go's `json.Marshal` produces compact JSON with alphabetically sorted map keys. The frontend uses JavaScript's `JSON.stringify` which preserves insertion order. These produce different byte sequences for the same logical document, meaning **hashes will not match cross-platform**.

This is acceptable: the hash is stored alongside the content for integrity checking, not for cross-platform verification. Each submission computes and stores its own hash. If cross-platform hash matching becomes required later, both sides need to agree on a canonical serialization (e.g., JCS — JSON Canonicalization Scheme).

### API contract uncertainty

The REST endpoints' exact field names need verification. The plan assumes `evidence` (Tiptap JSON string) and `evidence_hash` (hex string), snake_case. If the API uses different names:

```bash
# Verify by inspecting OpenAPI spec
andamio spec fetch && jq '.paths["/api/v2/course/student/commitment/submit"]' openapi.json

# Or test directly
curl -X POST https://preprod.api.andamio.io/api/v2/course/student/commitment/submit \
  -H "Authorization: Bearer $JWT" \
  -d '{"course_id":"test","course_module_code":"test","evidence":"{}","evidence_hash":"abc"}'
```

### Pre-formatted Tiptap JSON passthrough

If `--evidence` value is already valid Tiptap JSON (`{"type":"doc","content":[...]}`), should we detect and pass through? **Decision: No.** Always convert. The evidence commands are the convenience layer. Users with pre-formatted Tiptap can use the raw API via `tx run` or direct HTTP.

### Existing broken evidence data

Evidence submitted with older CLI versions has hex-encoded plaintext in `evidenceHash` and empty `evidence`. Fix does NOT include data migration. Teachers can ask students to re-submit via `course student update` after upgrading. Release notes should mention this.

## System-Wide Impact

- **API surface parity**: Three commands affected (`course student submit`, `course student update`, `project contributor update`). All use the same wrapping logic.
- **Error propagation**: `markdownToTiptap` errors bubble up as `"failed to format evidence"`. No silent failures.
- **State lifecycle risks**: None — this is a payload formatting fix, not a state change. The API handles persistence.
- **Integration test scenarios**: Submit evidence via CLI, verify `evidence` column contains valid Tiptap JSON and `evidenceHash` contains a 64-char hex hash (not hex-encoded plaintext).

## Acceptance Criteria

- [x] `course student submit --evidence "text"` sends Tiptap JSON in `evidence` field
- [x] `course student submit --evidence "text"` sends Blake2b-256 hex hash in `evidence_hash` field
- [x] `course student update` — same behavior
- [x] `project contributor update --evidence "text"` — same behavior
- [x] `--evidence-file path.md` reads file and converts Markdown to Tiptap JSON
- [x] `--evidence` and `--evidence-file` are mutually exclusive
- [x] `blake2b256` exported as `Blake2b256` from `internal/cardano`
- [x] Unit tests for `wrapEvidence`: plain text, URL, markdown, unicode, determinism
- [x] `--output json` returns full API response (no change to output contract)
- [x] Existing commands without evidence (`course student create`, `leave`, `claim`) unchanged

## Success Metrics

- Evidence submitted via CLI is visible in the teacher assessment UI (not hex gibberish)
- `evidenceHash` contains a proper 64-char Blake2b-256 hex hash
- `evidence` column contains valid Tiptap JSON document

## Dependencies & Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| API rejects `evidence_hash` field | Medium | Verify via OpenAPI spec or test call before implementing. If rejected, file API issue. |
| Hash doesn't match frontend computation | Low | Acceptable — hash is stored per-submission, not verified cross-platform |
| `markdownToTiptap` edge case on short evidence | Low | Unit tests cover plain text, URLs, lists |
| Existing broken data confuses teachers | Medium | Document in release notes, recommend re-submit |

## Sources & References

### Internal References
- Current evidence handler: `cmd/andamio/course_student.go:218-253`
- Project contributor handler: `cmd/andamio/project_contributor.go:171-204`
- `blake2b256` function: `internal/cardano/sign.go:169-173`
- `markdownToTiptap` converter: `cmd/andamio/course_import.go:652`
- TX metadata pattern: `cmd/andamio/tx_run.go:298-300`
- `--content-file` precedent: `cmd/andamio/project_task.go:627-642`

### Learnings Applied
- `docs/solutions/feature-implementations/cli-course-import-markdown-to-tiptap-conversion.md` — Tiptap node mapping, GFM extension requirement
- `docs/solutions/security-issues/tx-signing-code-review-witness-drop-url-validation.md` — Never silently discard errors

### External References
- Frontend reference: `andamio-app-v2/src/components/tx/task-commit.tsx` (from issue #36)
- Staging TX: `e006877c7e6c0e800fce9054781eda79b715151a95fe5f9f139808c0f4d823e2`

### Related Work
- GitHub Issue: #36
