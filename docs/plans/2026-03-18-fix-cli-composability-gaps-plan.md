---
title: "fix: CLI composability gaps — stdout separation, JSON modes, exit codes"
type: fix
status: completed
date: 2026-03-18
origin: docs/brainstorms/2026-03-18-composability-fixes-brainstorm.md
---

# fix: CLI composability gaps — stdout separation, JSON modes, exit codes

## Overview

A focused pass closing the known violations in the CLI's scripting contract. The
foundation is solid (no interactive prompts, `--output json` on data commands,
`isJSON` gating), but several concrete bugs undermine trust in that contract.
Fixing them costs little; leaving them makes the CLI unsafe to depend on in
scripts, CI, and agent tooling.

(see brainstorm: docs/brainstorms/2026-03-18-composability-fixes-brainstorm.md)

## Problem Statement

Six categories of violations, confirmed with exact file/line locations:

| # | Category | Files | Impact |
|---|----------|-------|--------|
| 1 | stdout leaks (progress text) | `course_create_module.go:90,117` | Poisons `--output json` pipelines |
| 2 | Empty-list messages to stdout | `course.go:142,184` | `jq` parse fails on "No X found" strings |
| 3 | Status/config commands have no JSON mode | `auth.go:49-54`, `user.go:260-308`, `config.go:50-63` | Scripts can't check auth state reliably |
| 4 | `spec paths` has no JSON mode | `spec.go:122-130` | Can't programmatically inspect endpoints |
| 5 | `spec fetch` progress leaks to stdout | `spec.go:34,72-73` | Metadata injected before file content |
| 6 | No exit code differentiation + double error printing + no JSON error envelope | `main.go:42-46` | Scripts can't branch on failure type; `--output json` gives empty stdout on failure |

## Proposed Solution

Six independent, low-risk changes plus one new shared package:

1. **Fix stdout leaks** — move two `fmt.Printf` calls to `fmt.Fprintf(os.Stderr, ...)` and add missing `!isJSON` guard.
2. **Fix empty-list messages** — `printList` emits `{"data":[]}` in JSON mode, moves text message to stderr in text mode.
3. **Add JSON to status/config commands** — marshal a defined struct when format is JSON.
4. **Add JSON to `spec paths`** — extract `summary` field from per-method OpenAPI data; return `[]{method, path, summary}` array.
5. **Fix `spec fetch` stdout leaks** — move progress prints to stderr.
6. **Exit codes + JSON error envelope** — new `internal/apierr` package with sentinel error types; `main.go` unwraps to pick exit code and emits `{"error":"..."}` to stdout in JSON mode.

## Technical Approach

### New Package: `internal/apierr`

Defines typed errors so `main.go` can map them to exit codes without importing
command packages:

```go
// internal/apierr/errors.go
package apierr

type NotFoundError struct{ Message string }
func (e *NotFoundError) Error() string { return e.Message }

type AuthError struct{ Message string }
func (e *AuthError) Error() string { return e.Message }
```

Commands (and the HTTP client) return these types when appropriate:
- HTTP 401/403 → `&apierr.AuthError{}`
- HTTP 404 → `&apierr.NotFoundError{}`
- `user exists` returning "not found" → `&apierr.NotFoundError{}`

### Updated `main.go`

```go
func main() {
    rootCmd.SilenceErrors = true   // we print errors ourselves
    rootCmd.SilenceUsage = true    // suppress usage dump on error

    if err := rootCmd.Execute(); err != nil {
        exitCode := 1
        var notFound *apierr.NotFoundError
        var authErr  *apierr.AuthError
        switch {
        case errors.As(err, &notFound):
            exitCode = 2
        case errors.As(err, &authErr):
            exitCode = 3
        }

        if output.GetFormat() == output.FormatJSON {
            fmt.Printf(`{"error":%q}`+"\n", err.Error())
        } else {
            fmt.Fprintln(os.Stderr, err)
        }
        os.Exit(exitCode)
    }
}
```

Exit code contract (globally applied, no flag needed — see brainstorm):
- `0` — success
- `1` — generic error (network, server, unexpected)
- `2` — not found
- `3` — auth required

### `course_create_module.go` fixes

```go
// Line 90 — inside !isJSON guard, wrong destination
fmt.Fprintf(os.Stderr, "Creating module %s (%s)...\n", title, code)

// Line 117 — missing guard AND wrong destination
if !isJSON {
    fmt.Fprintf(os.Stderr, "Created module: %s (%s)\n", title, code)
}
```

### `course.go` `printList` fix

```go
if !ok || len(data) == 0 {
    if output.GetFormat() == output.FormatJSON {
        fmt.Println(`{"data":[]}`)  // stdout — clean JSON, exit 0
    } else {
        fmt.Fprintln(os.Stderr, emptyMsg)  // stderr — text mode
    }
    return nil
}
```

Same pattern for the `len(modules) == 0` block at line 184.

### Status/config JSON shapes

**`auth status`** (`auth.go:49-54`):
```json
{ "api_key_set": true, "base_url": "https://preprod.api.andamio.io" }
```

**`config show`** (`config.go:50-63`):
```json
{ "base_url": "https://preprod.api.andamio.io", "api_key_set": true }
```

**`user status`** (`user.go:260-308`):
```json
{
  "api_key_set": true,
  "base_url": "https://preprod.api.andamio.io",
  "user_authenticated": true,
  "user_alias": "jmk",
  "user_id": "...",
  "session_expires_at": "2026-04-01T00:00:00Z",
  "session_expired": false,
  "session_remaining_seconds": 1234567
}
```

Each status command: detect `output.GetFormat() == output.FormatJSON`, marshal the struct, print to stdout; else fall through to existing text output unchanged.

### `spec paths` JSON shape

```json
[
  { "method": "GET", "path": "/api/v2/course/user/courses/list", "summary": "List courses" },
  ...
]
```

In text mode, also improve from `GET /path` to `GET /path — <summary>` for better readability.

Fix requires reading `methodsMap[method].(map[string]interface{})["summary"]` — data is already present in the parsed OpenAPI JSON, just discarded today.

### `spec fetch` stderr fix

`spec.go` lines 34, 72-73: change `fmt.Printf(...)` to `fmt.Fprintf(os.Stderr, ...)`, gate with `if !isJSON`.

## Acceptance Criteria

- [x] `andamio course create-module ... --output json | jq .` — no text noise before JSON
- [x] `andamio course list --output json | jq '.data | length'` — returns `0` on empty list, not a parse error
- [x] `andamio auth status --output json | jq -e '.api_key_set'` — exits 0 when key is set
- [x] `andamio user status --output json | jq -e '.user_authenticated'` — machine-readable auth state
- [x] `andamio config show --output json | jq -r '.base_url'` — returns URL string
- [x] `andamio spec paths --output json | jq '.[0].summary'` — returns summary string
- [x] `andamio course get nonexistent-id --output json` — stdout: `{"error":"..."}`, exit code mapped by HTTP status
- [x] `andamio course list` (no API key) — exit code 3 on 401/403
- [x] Generic errors (network down, etc.) — exit code 1
- [x] All existing text-mode output unchanged and human-readable
- [x] No double-printing of errors to stderr
- [x] Composability smoke tests all pass:
  ```bash
  andamio user status --output json | jq .         # clean JSON
  andamio user status 2>/dev/null | head -5        # no hang
  andamio course list --output json 2>/dev/null    # no stderr in stdout
  ```

## File Change Map

| File | Change |
|------|--------|
| `internal/apierr/errors.go` | **NEW** — `NotFoundError`, `AuthError` types |
| `cmd/andamio/main.go` | Add `SilenceErrors/SilenceUsage`; unwrap typed errors for exit codes; JSON error envelope |
| `cmd/andamio/course_create_module.go` | Lines 90, 117 — stderr + `!isJSON` guard |
| `cmd/andamio/course.go` | Lines 142, 184 — JSON empty list / stderr message |
| `cmd/andamio/auth.go` | Lines 49-54 — JSON mode for `auth status` |
| `cmd/andamio/user.go` | Lines 260-308 — JSON mode for `user status` |
| `cmd/andamio/config.go` | Lines 50-63 — JSON mode for `config show` |
| `cmd/andamio/spec.go` | Lines 34, 72-73 (fetch stderr), 122-130 (paths JSON mode) |
| `internal/client/client.go` | Return `apierr.AuthError` on 401/403, `apierr.NotFoundError` on 404 |

## Dependencies & Risks

- **No new dependencies.** All changes use stdlib + existing `output` package.
- **`SilenceErrors = true`** changes when Cobra prints error text — verify no command currently relies on Cobra's auto-print to appear on stderr. (Low risk: our commands all use `RunE` and return errors.)
- **HTTP client error wrapping** is the only spread change (touches response handling in `client.go`). Keep it narrow: only wrap 401/403/404. All other status codes stay as generic `fmt.Errorf`.
- **Typed errors in commands** — only `user exists` and HTTP responses need to return `NotFoundError`/`AuthError`. Most commands don't need changes to their error returns.

## Sources

- **Origin brainstorm:** [docs/brainstorms/2026-03-18-composability-fixes-brainstorm.md](../brainstorms/2026-03-18-composability-fixes-brainstorm.md) — Key decisions carried forward: empty-list to stderr/JSON, exit codes 0/1/2/3 globally applied, `{"error":"message"}` JSON envelope, `spec paths` path+method+summary only.
- Composability rules: `CLAUDE.md` — Composability Rules section
- Non-interactive architecture learnings: `docs/solutions/architecture/non-interactive-cli-stdin-picker-removal.md`
- Output package pattern: `docs/solutions/feature-implementations/cli-output-format-flag.md`
- Exact file/line findings: repo research (2026-03-18)
