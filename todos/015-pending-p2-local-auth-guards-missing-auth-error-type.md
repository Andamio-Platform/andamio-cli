---
status: complete
priority: p2
issue_id: "015"
tags: [code-review, composability, exit-codes, auth, apierr]
dependencies: []
---

# Local Auth Guard Checks Return Plain Error — Exit Code 1 Instead of 3

## Problem Statement

PR #15 establishes that exit code 3 = "auth required." But three commands that check for authentication *before* making API calls return `fmt.Errorf(...)` instead of `&apierr.AuthError{}`:

- `cmd/andamio/project_task.go` line ~28: `return fmt.Errorf("not authenticated. Run 'andamio user login' first")`
- `cmd/andamio/course_export.go` line ~46: similar auth check
- `cmd/andamio/course_import.go` line ~73: similar auth check

These checks run in `PreRunE` / `PersistentPreRunE` before any HTTP call is made. When they fail, `main.go` gets a plain `error`, not an `*apierr.AuthError`, so `errors.As` doesn't match and the exit code is 1 (generic error) rather than 3 (auth required).

A script testing `$?` after `andamio course export ...` when not logged in will get exit code 1, not 3 — defeating the exit-code contract this PR is trying to establish.

## Findings

- **Source**: Architecture agent (P2), agent-native reviewer (P2)
- **Location**: `cmd/andamio/project_task.go:28`, `cmd/andamio/course_export.go:46`, `cmd/andamio/course_import.go:73`

## Proposed Solutions

### Option A: Return `apierr.AuthError` from local auth guards (Recommended)

```go
// Before
if !cfg.HasUserAuth() {
    return fmt.Errorf("not authenticated. Run 'andamio user login' first")
}

// After
if !cfg.HasUserAuth() {
    return &apierr.AuthError{Message: "not authenticated. Run 'andamio user login' first"}
}
```

**Pros:** Completes the exit-code contract. Scripts get exit code 3 for all auth failures, whether local or remote.
**Cons:** Requires importing `apierr` in the command files.
**Effort:** Small (3 files, 1-line change each)
**Risk:** None — change is transparent to text-mode users; only exit code changes

### Option B: Add a helper function

```go
// in a shared file
func errNotAuthenticated() error {
    return &apierr.AuthError{Message: "not authenticated. Run 'andamio user login' first"}
}
```

**Pros:** Single source of truth for the auth error message.
**Cons:** Adds a helper for a 3-use case. May be premature.
**Effort:** Small
**Risk:** None

## Recommended Action

Option A — direct return of `apierr.AuthError` in each file. Three files, minimal change.

## Technical Details

- **Affected files**: `cmd/andamio/project_task.go`, `cmd/andamio/course_export.go`, `cmd/andamio/course_import.go`
- **PR**: #15 fix/composability-gaps

## Acceptance Criteria

- [ ] `andamio course export <id>` when not authenticated exits with code 3
- [ ] `andamio project task list <id>` when not authenticated exits with code 3
- [ ] `andamio course import <id>` when not authenticated exits with code 3
- [ ] Exit code 3 appears in `--output json` mode with `{"error":"not authenticated..."}`

## Work Log

- 2026-03-18: Flagged by architecture and agent-native agents during PR #15 review
