---
status: complete
priority: p2
issue_id: "014"
tags: [code-review, go-idioms, client, error-handling]
dependencies: []
---

# `fmt.Errorf("%s", msg)` Should Be `errors.New(msg)` in client.go

## Problem Statement

`client.go` uses `fmt.Errorf("%s", msg)` in three places for the generic (non-typed) error fallthrough:

```go
return fmt.Errorf("%s", msg)  // lines 68, 123, 170
```

`msg` is already a `string`. Using `fmt.Errorf("%s", msg)` to format a string into a new error is redundant and triggers a `go vet` / `staticcheck` warning: `SA1006: redundant call to fmt.Sprintf`. The idiomatic Go form when constructing an error from an existing string (with no wrapping) is `errors.New(msg)`.

This will generate noise if a vet/lint pass is added to CI.

## Findings

- **Source**: Code quality agent (P2)
- **Location**: `internal/client/client.go:68`, `internal/client/client.go:123`, `internal/client/client.go:170`

## Proposed Solutions

### Option A: Replace all three with `errors.New(msg)` (Recommended)

```go
// Before
return fmt.Errorf("%s", msg)

// After
return errors.New(msg)
```

Add `"errors"` to the import block.

**Pros:** Idiomatic. Silences staticcheck SA1006. 3-line change across 3 locations.
**Cons:** None.
**Effort:** Trivial
**Risk:** None — identical runtime behavior

## Recommended Action

Option A.

## Technical Details

- **Affected files**: `internal/client/client.go:68`, `:123`, `:170`
- **PR**: #15 fix/composability-gaps

## Acceptance Criteria

- [ ] No `fmt.Errorf("%s", ...)` in client.go
- [ ] `go vet ./...` passes without SA1006 warnings
- [ ] `errors` package imported in client.go

## Work Log

- 2026-03-18: Flagged by code-quality agent during PR #15 review
