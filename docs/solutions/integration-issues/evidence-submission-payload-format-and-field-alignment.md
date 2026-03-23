---
title: "Fix evidence submission payload format and field name mismatches across student/contributor commands"
date: 2026-03-23
category: integration-issues
severity: high
tags:
  - evidence-submission
  - api-payload
  - field-mapping
  - tiptap-json
  - blake2b-hash
  - course-student
  - project-contributor
  - teacher-review
  - openapi-spec
modules:
  - cmd/andamio/helpers.go
  - cmd/andamio/course_student.go
  - cmd/andamio/project_contributor.go
  - cmd/andamio/course_teacher_ops.go
  - internal/cardano/sign.go
symptoms:
  - "Evidence submitted via CLI stored as hex-encoded plaintext in evidenceHash column"
  - "evidence column empty in database despite successful API responses"
  - "project contributor commands sent task_index but API required task_hash"
  - "course student submit sent course_module_code but API required slt_hash"
  - "Teacher review sent commitment_id and approve/reject but API required module_code + participant_alias and accept/refuse"
root_cause: "CLI sent raw plaintext strings for evidence fields and used incorrect field names that did not match the API's OpenAPI schema"
resolution: "Wrapped evidence in Tiptap JSON with Blake2b-256 hash, resolved on-chain identifiers via API lookups, aligned all payload fields with OpenAPI spec"
related_issues:
  - "#36"
related_docs:
  - "docs/solutions/feature-implementations/cli-course-import-markdown-to-tiptap-conversion.md"
  - "docs/solutions/logic-errors/fix-three-cli-issues-hex-encoding-lesson-merge-headless-login.md"
  - "docs/solutions/security-issues/tx-signing-code-review-witness-drop-url-validation.md"
  - "docs/solutions/integration-issues/cli-course-import-app-parity-and-payload-alignment.md"
---

# Fix evidence submission payload format and field name mismatches

## Problem

Observed on staging (2026-03-22), TX `e006877c...`:

```json
{
  "studentAlias": "badger",
  "status": "SUBMITTED",
  "evidence": "",
  "evidenceHash": "4261646765722074657374696e6720666565646261636b"
}
```

The `evidenceHash` decodes to ASCII `"Badger testing feedback"` -- the raw evidence text hex-encoded, not a hash. The `evidence` column was empty.

Three separate bugs were intertwined:

1. **Evidence format mismatch** -- all three evidence commands (`course student submit`, `course student update`, `project contributor update`) sent the `--evidence` flag as a raw plaintext string. The API expected a Tiptap JSON document (`type: object`) with a separate Blake2b-256 content hash.

2. **Field name misalignments** -- discovered via OpenAPI spec review:
   - Project contributor commands sent `task_index` (integer); API required `task_hash` (on-chain hash)
   - Course student submit sent `course_module_code`; API required `slt_hash` (on-chain module identifier)
   - Teacher review sent `commitment_id` + `approve`/`reject`; API required `course_module_code` + `participant_alias` + `accept`/`refuse`

3. **Evidence type mismatch** -- initial fix returned evidence as a serialized JSON string. The API spec defines `evidence` as `type: object`, requiring a nested JSON object (Go `map[string]interface{}`), not a string containing JSON.

## Investigation Steps

1. **Observed bad data on staging** -- TX showed empty `evidence` column, `evidenceHash` containing hex-encoded plaintext.

2. **Traced to CLI source** -- the three evidence commands in `course_student.go` and `project_contributor.go` sent `--evidence` as a raw string with no transformation.

3. **Verified API contract via OpenAPI spec**:
   ```bash
   andamio spec fetch
   jq '.definitions.SubmitAssignmentCommitmentV2Request' openapi.json
   ```
   Revealed: `evidence` is `type: object` (not string), `evidence_hash` is `type: string`, and the submit endpoint requires `slt_hash` (not `course_module_code`).

4. **Discovered field misalignments** -- systematic comparison of all CLI payloads against OpenAPI definitions revealed three additional mismatches beyond the evidence format.

5. **Referenced frontend source** (`andamio-app-v2/src/components/tx/task-commit.tsx`) and `@andamio/core` (`src/utils/hashing/commitment-hash.ts`) to confirm the expected Tiptap JSON structure and Blake2b-256 hashing approach (normalize keys, trim strings, then hash).

6. **Discovered type mismatch** -- initial fix returned evidence as a JSON string; API spec required `type: object`, so return type was changed from `string` to `map[string]interface{}`.

## Root Cause

The CLI commands were written by inferring API payload fields from the CLI flag names (`--task-index` -> `task_index`, `--evidence` -> `evidence` as string) without consulting the OpenAPI spec. The API requires:
- On-chain identifiers (`task_hash`, `slt_hash`) instead of human-friendly identifiers (`task_index`, `course_module_code`)
- Structured evidence as a Tiptap JSON object with a separate content hash
- Specific enum values (`accept`/`refuse`, not `approve`/`reject`)

## Solution

### Core: `wrapEvidence` helper

Converts evidence text to Tiptap JSON and computes a Blake2b-256 content hash matching `@andamio/core computeCommitmentHash`:

```go
func wrapEvidence(text string) (map[string]interface{}, string, error) {
    tiptapDoc, err := markdownToTiptap(text, nil)
    normalized := normalizeForHashing(tiptapDoc)
    jsonBytes, _ := json.Marshal(normalized)
    hash := cardano.Blake2b256(jsonBytes)
    normalizedDoc, _ := normalized.(map[string]interface{})
    return normalizedDoc, hex.EncodeToString(hash), nil
}
```

Key: returns `map[string]interface{}` (not string) so it serializes as a nested JSON object in the HTTP payload.

### Normalization for deterministic hashing

Trims whitespace from strings to match `@andamio/core` behavior. Go's `json.Marshal` already sorts map keys alphabetically:

```go
func normalizeForHashing(v interface{}) interface{} {
    switch val := v.(type) {
    case string:
        return strings.TrimSpace(val)
    case map[string]interface{}:
        sorted := make(map[string]interface{}, len(val))
        for k, child := range val { sorted[k] = normalizeForHashing(child) }
        return sorted
    case []interface{}:
        out := make([]interface{}, len(val))
        for i, child := range val { out[i] = normalizeForHashing(child) }
        return out
    default:
        return v
    }
}
```

### Resolver pattern for on-chain identifiers

Two lookup functions translate user-friendly identifiers to on-chain hashes:

```go
// resolveTaskHash fetches task list, matches by task_index, returns task_hash
func resolveTaskHash(c *client.Client, projectID string, taskIndex int) (string, error)

// resolveSltHash fetches modules, matches by course_module_code, returns slt_hash
func resolveSltHash(c *client.Client, courseID, moduleCode string) (string, error)
```

Both provide actionable error messages with discovery hints on failure.

### Factory pattern for commit/delete

`runTaskHashAction` consolidates the identical commit/delete handlers that resolve `task_hash` then POST with it:

```go
func runTaskHashAction(endpoint, verb string) func(cmd *cobra.Command, args []string) error {
    return func(cmd *cobra.Command, args []string) error {
        c, taskHash, taskIndex, err := loadClientAndResolveTask(cmd)
        // ... POST {"task_hash": taskHash}
    }
}
```

### Submit/update split

The shared `runCourseStudentSubmitOrUpdate` factory was split because the two endpoints have different schemas:
- **Submit** resolves `slt_hash` from module code, sends `{course_id, slt_hash, evidence, evidence_hash}`
- **Update** uses `course_module_code` directly, sends `{course_id, course_module_code, evidence, evidence_hash}`

### Teacher review field alignment

Changed from `{course_id, commitment_id, decision: "approve"/"reject", feedback?}` to `{course_id, course_module_code, participant_alias, decision: "accept"/"refuse"}`.

## Key Decisions

| Decision | Rationale |
|----------|-----------|
| Fix REST endpoints, not switch to TX pipeline | Students may not have `.skey` files. Adding `--skey` would be a UX regression. |
| Normalize before hashing | Frontend trims strings before hashing. Without normalization, trivial whitespace differences produce different hashes. |
| Return `map[string]interface{}` not string | API defines `evidence` as `type: object`. A string would be double-serialized. |
| Always convert via markdownToTiptap (no passthrough) | Evidence commands are the convenience layer. Raw Tiptap can go via `tx run`. |
| Cross-platform hash parity is a non-goal | Go sorts keys alphabetically, JS preserves insertion order. Hash is stored per-submission for integrity, not cross-platform verification. |

## Prevention Strategies

### Common Mistake Patterns

| Mistake | What Happened | Prevention |
|---------|---------------|------------|
| Guessing field names from CLI flags | `--task-index` mapped to `task_index`, API wanted `task_hash` | Always check OpenAPI spec `requestBody` schema |
| Sending strings where API expects objects | `evidence` sent as serialized JSON string | Check schema `type` property; `type: object` = `map[string]interface{}` |
| Guessing enum values | Used `approve`/`reject`, API wanted `accept`/`refuse` | Extract enum values from OpenAPI schema |
| Friendly identifiers instead of wire identifiers | Sent `course_module_code`, API wanted `slt_hash` | Add resolver when CLI flag != API field |
| Skipping `url.PathEscape` on path segments | `resolveSltHash` interpolated raw courseID into URL path | Every user-supplied path segment MUST use `url.PathEscape()` |
| Matching frontend code instead of API spec | Initial fix tried to match frontend serialization | API spec is the contract, frontend is a reference implementation |

### Pre-implementation Checklist

For any new command handler:

```bash
# 1. Fetch and inspect the endpoint schema
andamio spec fetch
jq '.definitions.<RequestSchemaName>' openapi.json

# 2. Extract: required fields, field names, field types, enum values
# 3. Write as a comment block above the handler
# 4. Verify url.PathEscape on all dynamic path segments
# 5. Use wrapEvidence() for evidence fields (not raw strings)
```

### Spec Alignment Verification

```bash
# Compare spec across API versions
cp openapi.json openapi-old.json
andamio spec fetch
diff <(jq -S '.definitions' openapi-old.json) <(jq -S '.definitions' openapi.json)
```

## Test Coverage

Six unit tests in `evidence_test.go` covering `wrapEvidence`:

| Test | Input | Assertion |
|------|-------|-----------|
| PlainText | `"My evidence"` | Valid Tiptap doc, 64-char hex hash |
| URL | `"https://github.com/..."` | Valid Tiptap doc |
| MarkdownList | `"- item 1\n- item 2"` | Contains `bulletList` node |
| Unicode | CJK characters | Valid JSON, valid hash |
| SpecialChars | Quotes and backslashes | Survives JSON round-trip |
| Determinism | Same input twice | Identical hashes |

**Recommended additions:** golden test vector matching `@andamio/core` output, resolver tests with mocked HTTP client, `readEvidenceFlag` mutual exclusivity tests.

## Related Documentation

- [Markdown to Tiptap conversion](../feature-implementations/cli-course-import-markdown-to-tiptap-conversion.md) -- the `markdownToTiptap()` function used by `wrapEvidence`
- [Hex encoding and API field alignment](../logic-errors/fix-three-cli-issues-hex-encoding-lesson-merge-headless-login.md) -- prior hex-encoding fix and API field name mismatch pattern
- [TX signing code review](../security-issues/tx-signing-code-review-witness-drop-url-validation.md) -- `Blake2b256` function origin, "never discard crypto errors" rule
- [Course import API parity](../integration-issues/cli-course-import-app-parity-and-payload-alignment.md) -- same principle: CLI must produce the same payload as the web app
- [Export/import round-trip](../logic-errors/export-import-round-trip-title-preservation.md) -- empty array deletion semantics, variable shadowing patterns
