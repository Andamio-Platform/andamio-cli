---
status: pending
priority: p3
issue_id: "023"
tags: [code-review, reliability, internal-client, truncation, pr-68-followup]
dependencies: []
---

# Error Body Truncation Can Hide Three-Gate Predicate Tokens

## Problem Statement

`internal/client/client.go` truncates non-2xx response bodies at `maxErrorBodySize = 500` characters before wrapping them into typed errors (including `*apierr.ConflictError`). The three-gate predicate in `isModuleAlreadyExistsError` does `strings.Contains` against the truncated `Message`:

```go
msg := strings.ToLower(conflict.Message)
return strings.Contains(msg, "already exists") && strings.Contains(msg, "course_module_code")
```

If the gateway ever prefixes a 409 body with verbose context — a request ID, validation breakdown, stack-like metadata, or nested error envelope — such that the `"course_module_code"` or `"already exists"` token lands past byte 500, the `truncateErrorBody` output will contain neither required token (or only one), the predicate returns false, and `register-module` idempotency silently regresses to `"failed to register module: ..."`.

Today's preprod 409 bodies are short enough that this is theoretical. Post-PR-#68 the interaction is load-bearing: the type gate (`errors.As(*ConflictError)`) confirms the conflict; the body gates narrow which 409 it is. Both body gates depend on the tokens surviving truncation.

Found during ce:review interactive re-review of PR #68 (cross-reviewer consensus — reliability 0.62 + correctness residual 0.55, merged confidence 0.72).

## Affected Files

- `internal/client/client.go:20` — `maxErrorBodySize = 500` constant
- `internal/client/client.go:187-193` — `truncateErrorBody` implementation
- `internal/client/client.go:62, 119, 168` — the three call sites that construct typed errors from truncated bodies
- `cmd/andamio/course_teacher_ops.go:306-307` — the predicate that consumes the truncated `Message`

## Options

### Option A (minimal, low effort)

Raise `maxErrorBodySize` from 500 to 2048. Rationale: error bodies don't drive the happy path; 2 KB of log spam beats a silent idempotency regression. The truncation was introduced to prevent log flooding / info leakage — 2 KB still respects that goal for typical payloads.

### Option B (preserve raw body separately on typed errors)

Introduce a `RawBody` field on typed errors (`NotFoundError`, `AuthError`, `ConflictError`) that holds the untruncated response body. `Message` continues to use the truncated display string. Predicates that need the full body for programmatic checks read `RawBody` directly:

```go
type ConflictError struct {
    Message string
    RawBody string // full response body, untruncated, for programmatic parsing
}
```

Requires updating `internal/client/client.go` error construction to capture both, and `isModuleAlreadyExistsError` to prefer `RawBody` when non-empty.

Behavioral change — slightly larger, but cleaner separation of display vs. parseable content.

### Option C (do nothing)

Accept the theoretical risk. Rely on the P2 preprod capture (todo #021) to confirm actual body sizes. Downgrade urgency unless the capture shows bodies approaching or exceeding 500 bytes.

**Preferred:** Option C unless todo #021 surfaces a body > 400 bytes (gives margin for gateway wording drift). If it does, Option A is the cheap fix.

## Acceptance

- [ ] Complete todo #021 first to capture a real 409 body and measure its size.
- [ ] If the captured body is > 400 bytes: apply Option A (raise `maxErrorBodySize` to 2048) with a comment explaining the interaction with the three-gate predicate.
- [ ] If Option A is applied: add a test to `internal/client/client_test.go` covering `truncateErrorBody` at the new boundary, AND a test to `TestIsModuleAlreadyExistsError` with a ConflictError whose body is 1900+ bytes and whose tokens sit past byte 500.
- [ ] If the captured body is < 400 bytes: close this todo as "verified — truncation boundary not load-bearing for current gateway."

## Context

- **ce:review run artifact:** `.context/compound-engineering/ce-review/2026-04-22-pr68-conflict-error/findings.md`
- **Dependency:** `todos/021-pending-p2-verify-gateway-409-for-duplicate-module.md` — blocks Option A/C decision on the actual body size measurement.
- **Origin PR:** https://github.com/Andamio-Platform/andamio-cli/pull/68 (surfaced during interactive re-review after the autofix pass).
