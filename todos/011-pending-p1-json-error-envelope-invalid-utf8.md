---
status: complete
priority: p1
issue_id: "011"
tags: [code-review, composability, json, main]
dependencies: []
---

# JSON Error Envelope Uses `%q` — Invalid JSON for Control Characters

## Problem Statement

`main.go` builds the JSON error envelope using Go's `%q` format verb:

```go
fmt.Printf(`{"error":%q}`+"\n", err.Error())
```

`%q` produces a **Go-quoted string literal**, not a JSON string. The two formats diverge for control characters: Go `%q` emits `\a`, `\b`, `\f`, `\v`, octal `\NNN`, and hex `\xNN` escape sequences that are not valid JSON escape sequences. If an API server returns a 500/4xx body containing a non-printable byte (common in localized error messages, HTML error pages, or UTF-8 content near the 500-byte truncation boundary), the resulting output will be syntactically invalid JSON. Any downstream `jq` pipeline will fail silently or with a parse error.

Additionally, `truncateErrorBody` truncates at a byte offset (`s[:500]`), which can split a multi-byte UTF-8 rune. The resulting invalid UTF-8 sequence is then passed through `%q`, which emits `\xNN` escapes — again, not valid JSON.

**Impact:** `andamio <cmd> --output json | jq .error` will silently fail or produce a parse error whenever an API error response contains a non-printable byte or multi-byte UTF-8 near the truncation boundary. This is the exact scripting surface we're trying to make reliable.

## Findings

- **Source**: Code quality agent, security agent (both independently flagged)
- **Location**: `cmd/andamio/main.go:65`
- **Related**: `internal/client/client.go:180-186` (byte-boundary truncation compounds the issue)

## Proposed Solutions

### Option A: Use `encoding/json.Marshal` (Recommended)

```go
if output.GetFormat() == output.FormatJSON {
    b, _ := json.Marshal(map[string]string{"error": err.Error()})
    fmt.Println(string(b))
}
```

**Pros:** Uses the same JSON encoder as every other command. Guarantees valid JSON. Handles all Unicode correctly. Consistent with `output.PrintJSON` pattern used elsewhere.
**Cons:** Two lines instead of one.
**Effort:** Small
**Risk:** None

### Option B: Add `encoding/json` import and use `json.Marshal` on a struct

```go
type errEnvelope struct {
    Error string `json:"error"`
}
b, _ := json.Marshal(errEnvelope{Error: err.Error()})
fmt.Println(string(b))
```

**Pros:** Typed, consistent with other response structs in the codebase.
**Cons:** Adds a type only used in one place.
**Effort:** Small
**Risk:** None

### Option C: Fix `truncateErrorBody` to truncate at rune boundaries

Fix the truncation function to use `utf8.RuneCountInString` and iterate by rune. This prevents the invalid-UTF-8 root cause but does not fix the `%q` issue.

**Pros:** Also fixes text-mode output.
**Cons:** Does not fix the `%q` JSON problem. Both fixes are needed.
**Effort:** Small
**Risk:** None

## Recommended Action

Option A. Also fix `truncateErrorBody` to truncate at rune boundaries (see `client.go:180-186`).

## Technical Details

- **Affected files**: `cmd/andamio/main.go:65`, `internal/client/client.go:180-186`
- **PR**: #15 fix/composability-gaps

## Acceptance Criteria

- [ ] `{"error": "some message with \u0007 control char"}` is valid JSON
- [ ] `andamio course get bad-id --output json | jq -e .error` succeeds without parse error
- [ ] Error message is produced using `encoding/json`, not `fmt.Printf` with `%q`
- [ ] `truncateErrorBody` truncates at a valid UTF-8 rune boundary

## Work Log

- 2026-03-18: Flagged by code-quality and security review agents during PR #15 review
