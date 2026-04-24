---
title: Gateway returned 400 where CLI's typed-error gate expected 409 — silent idempotency failure
date: "2026-04-24"
category: integration-issues
module: course_teacher_ops, internal_client
problem_type: integration_issue
component: tooling
symptoms:
  - "course teacher register-module never took the idempotent recovery path on a duplicate course_module_code"
  - "isModuleAlreadyExistsError returned false in production even when the gateway body contained 'already exists' + 'course_module_code'"
  - "Users running register-module twice saw 'failed to register module: API error 400: ...' instead of the expected advance-DRAFT-to-APPROVED idempotent success"
root_cause: wrong_api
resolution_type: code_fix
severity: high
related_components:
  - apierr
  - client
tags:
  - api-contract
  - typed-errors
  - status-code-drift
  - gateway-api
  - idempotency
  - register-module
  - go
---

# Gateway Status-Code Drift: 409 Expected, 400 Returned

## Problem

PR #68 (2026-04-22) refactored `isModuleAlreadyExistsError` from pure
body-string matching to a typed `*apierr.ConflictError` gate backed by
`errors.As`. The design assumed HTTP 409 for duplicate `course_module_code`
registration — a reasonable REST convention and the pattern the CLI's error
surface was built around.

**But the andamio-api gateway returns HTTP 400, not 409, for `DUPLICATE_CODE`.**
Verified at `andamio-api/internal/handlers/v2/merged_handlers/merged_handlers.go:881-892`:

```go
if regErr, ok := err.(*orchestration.RegisterModuleError); ok {
    switch regErr.Code {
    case "COURSE_NOT_FOUND", "MODULE_NOT_FOUND":
        return writeErrorEnvelope(c, fiber.StatusNotFound, regErr.Message)
    case "DUPLICATE_CODE":
        // ↓ Expected: fiber.StatusConflict (409)
        // Actual:   fiber.StatusBadRequest (400)
        return writeErrorEnvelope(c, fiber.StatusBadRequest, regErr.Message)
    default:
        return writeErrorEnvelope(c, fiber.StatusBadGateway, regErr.Message)
    }
}
```

`internal/client/client.go:statusError` only constructs `*apierr.ConflictError`
for HTTP 409; for 400 it returns a plain `errors.New(msg)`. Consequence: the
`errors.As(err, &conflict)` gate in `isModuleAlreadyExistsError` **never
matched in production**. Every duplicate-code POST took the "not a recognized
conflict → fail" branch, silently defeating the idempotency guarantee the
refactor was meant to preserve.

## Symptoms

- Running `andamio course teacher register-module` with the same `--module-code`
  a second time surfaced `failed to register module: API error 400:
  course_module_code already exists in this course` instead of a clean
  `action: already_registered` JSON envelope or `advance DRAFT → APPROVED`
  message.
- CI pipelines and agent scripts relying on register-module being idempotent
  (running the command once per re-deploy, expecting no-ops on re-runs) saw
  the command flip from exit 0 to exit 1 on duplicate inputs.
- Unit tests continued to pass: they constructed `*apierr.ConflictError`
  directly rather than routing through `statusError`, so the gate fired
  correctly in tests and silently missed in production.

## What Didn't Work

**Trusting the typed-error design.** The original PR #68 plan's `Risks &
Dependencies` table explicitly flagged this: "Gateway returns a non-409
status code for a module-code conflict ... the new predicate's `errors.As`
gate silently returns `false` and register-module loses its idempotency
guarantee for that path." The mitigation was written as "Before merging,
capture a real preprod 409 response body" — but the verification step was
never performed. The PR merged on the assumption that 409 was the contract.

**Unit tests with hand-built errors.** `TestIsModuleAlreadyExistsError` fed
`*apierr.ConflictError{Message: "..."}` values directly into the predicate
and asserted `true`. This proved the logic correct for the error *type* the
predicate expected but said nothing about whether the gateway actually
produced that type. The gap is a classic mocks-lie-about-integration trap:
the unit test locked in an assumption about the external contract rather
than the contract itself.

**Code review.** Three reviewers saw PR #68. None flagged the unverified
contract assumption, because the change looked locally correct and the risk
was written down in the plan — making it feel handled.

## Solution

Layer a body-token fallback path onto `isModuleAlreadyExistsError`. Strict
409-via-`errors.As` remains the primary gate. When it misses, inspect
`err.Error()` for the body token pair (`"already exists"` AND
`"course_module_code"`); if both present, treat as a conflict and emit a
stderr warning about the status drift so operators see the wire reality.

```go
// Before (PR #68)
func isModuleAlreadyExistsError(err error) bool {
    if err == nil {
        return false
    }
    var conflict *apierr.ConflictError
    if !errors.As(err, &conflict) {
        return false  // ← 400 responses ended up here, silent failure
    }
    msg := strings.ToLower(conflict.Message)
    return strings.Contains(msg, "already exists") &&
           strings.Contains(msg, "course_module_code")
}

// After (todo #021)
func isModuleAlreadyExistsError(err error) bool {
    if err == nil {
        return false
    }
    var conflict *apierr.ConflictError
    if errors.As(err, &conflict) {
        msg := strings.ToLower(conflict.Message)
        return strings.Contains(msg, "already exists") &&
               strings.Contains(msg, "course_module_code")
    }
    // Fallback: body tokens match but the typed-error gate didn't fire
    // (gateway returned 400/500/etc). Accept the match and warn so the
    // drift is visible.
    msg := strings.ToLower(err.Error())
    if strings.Contains(msg, "already exists") &&
       strings.Contains(msg, "course_module_code") {
        if output.GetFormat() != output.FormatJSON {
            fmt.Fprintln(os.Stderr,
                "warning: duplicate course_module_code detected via body-token "+
                "fallback; gateway did not surface HTTP 409. Please report "+
                "the status/wording drift to the andamio-api team.")
        }
        return true
    }
    return false
}
```

Captured the actual vs expected gateway contract as an in-repo fixture:
`internal/client/testdata/preprod-duplicate-module-response.md`.

## Why This Works

The fallback path converts silent failure into visible drift. Three
guarantees hold:

1. **When the gateway is eventually fixed to return 409**, the strict
   `errors.As` path fires first and the fallback warning stops appearing.
   No CLI change is required on that day — the strict path was always the
   intended contract; it just wasn't the real one.
2. **When the gateway reworks the body wording** in a way that drops either
   token, both paths miss and the CLI fails loudly with the original error,
   which is correct behavior — an unrecognized response shouldn't silently
   route through idempotency recovery.
3. **When both tokens match but the error came from an unrelated 5xx**
   (the documented false-positive risk), the idempotency recovery path
   proceeds, but `lookupTeacherModule` — the next step in
   `registerOrRecoverModule` — re-fetches the teacher module list by code
   and verifies hash + status before mutating anything. A pathological
   body-token match on a real internal error ends in "module not found
   in teacher list", not in state corruption.

## Prevention

### 1. Verify external contracts against the counterparty's source

When building a typed-error gate — or any client-side predicate conditioned
on an external status code / body shape — **read the gateway code** (or
capture a real response) before merging. In the andamio toolchain the
gateway source is checked out alongside the CLI at
`~/projects/01-projects/andamio-api/`, so verification is a grep away:

```bash
grep -rn "StatusConflict\|StatusBadRequest\|fiber.Status" \
    ~/projects/01-projects/andamio-api/internal/handlers/v2/ | \
    grep -i "<the-error-code-name>"
```

Don't accept "the plan flagged this risk" as a substitute for verification.
The whole point of a plan's `Risks` table is to surface items that need
active checking — a flagged item is an open task, not a closed one.

### 2. When a gateway contract is known-unstable, layer a visible fallback

If strict type-matching is the right long-term design but the gateway
hasn't caught up yet, layer a body-token fallback with a stderr warning
that's visible to operators. Document it as a bridge that removes itself
when upstream is fixed. Pattern:

```go
// Strict path — primary, matches the intended contract.
if errors.As(err, &typedErr) {
    // ... predicate ...
}
// Fallback — bridges the gap while upstream catches up. Emit a warning
// so the drift is visible instead of silent.
if matchesByBody(err) {
    fmt.Fprintln(os.Stderr, "warning: ... please report the drift.")
    return true
}
```

### 3. Unit tests for predicates conditioned on external errors need
**end-to-end tests** that route through `statusError`

Today's `TestIsModuleAlreadyExistsError` table tests the predicate with
hand-built `*apierr.ConflictError` literals. That's useful for logic
coverage, but it can't detect a contract mismatch — the test locks in the
assumption, not the contract. For predicates gated on status-code-derived
types, add at least one test that drives a real HTTP response through
`httptest.NewServer` → `client.Get` → predicate, asserting the end-to-end
wiring. The existing `client_test.go:TestClient_StatusCodeToTypedError`
is the right shape; predicates should have a sibling integration test.

### 4. Resolve plan-Risks items before the PR merges, not after

Plan-time risk tables are the best place to surface "what could break
this design" — but they're only valuable if the mitigations are
*delivered*, not just *listed*. A `Risks & Dependencies` row that says
"Before merging, capture a real response" is an acceptance criterion, not
a nice-to-have. Don't merge with it outstanding; track it as a follow-up
todo so it actually gets done (as happened here: todo #021 caught and
closed the verification gap PR #68 left open).

## Related

### Sibling fixes in the same batch

Two smaller ergonomic fixes shipped alongside this one, both expressed as
"normalize at the boundary that consumers expect":

- **`--version --output JSON` case sensitivity** (todo #024): Cobra's
  `--version` path bypasses `PersistentPreRunE`, so `output.SetFormat`
  (which lowercases) never ran, and `--output JSON` silently fell
  through to text. Fix: `strings.EqualFold` in `buildVersionOutput`.
  The pattern: any fallback path that reads flags directly must
  normalize them the same way the primary path does, or the two will
  drift.

- **`release.sh` CHANGELOG preflight** (todo #026): `grep -qF "## [$VERSION]"`
  couldn't detect the "maintainer accumulated bullets under
  `## [Unreleased]` but forgot to rename the heading" failure mode —
  the exact failure the preflight was built to prevent. Fix: awk-based
  inspection of the Unreleased block; hard-exit when non-empty and no
  versioned heading present. The pattern: release-gate checks need to
  assert on the *semantic* state of the CHANGELOG (promoted vs not),
  not just the *syntactic* presence of a heading.

### Related docs

- `docs/solutions/integration-issues/cli-api-payload-mismatches.md` —
  sibling integration class: payload-field drift between CLI and
  gateway. Different dimension (field names vs status codes), same
  root lesson ("verify against the contract the gateway actually
  enforces, not the one we wish it enforced").
- `docs/solutions/architecture/typed-output-envelope-with-gateway-state-fallbacks.md` —
  complementary pattern for gateway-state discrepancies in *success*
  responses. This doc covers *error* responses.
- `docs/solutions/architecture/go-retry-classifier-and-backoff-patterns.md` —
  `apierr.BackpressureError` shares the same typed-error-from-status-code
  pattern that this bug exposed a weakness in. The retry classifier is
  more defensible because it combines status match with body-hint
  parsing already.

### Todos resolved

- `021-pending-p2-verify-gateway-409-for-duplicate-module.md` →
  resolved on 2026-04-24 via PR #76.
- `024-pending-p2-version-flag-output-validation.md` → resolved in
  the same PR.
- `026-pending-p2-release-preflight-detects-unpromoted-unreleased.md`
  → resolved in the same PR.
