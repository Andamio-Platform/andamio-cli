---
status: pending
priority: p2
issue_id: "021"
tags: [code-review, reliability, client, conflict-detection, pr-68, blocks-pr-66]
dependencies: []
blocks: [docs/plans/2026-04-22-003-refactor-typed-register-module-envelope-plan.md]
---

# Capture real preprod gateway responses for register-module + update-module-status (409 + 200)

## Problem Statement

PR #68 migrated `isModuleAlreadyExistsError` from pure string matching to a three-gate check:
`errors.As(err, *apierr.ConflictError)` + body `"already exists"` + body `"course_module_code"`.

The type gate (`errors.As`) is load-bearing — it replaces what the body-only predicate was silently doing as a status-code proxy. The gate fires only when `internal/client.Post` returns `*apierr.ConflictError`, which happens only on HTTP 409 responses.

**The entire register-module idempotency guarantee now depends on the preprod/mainnet Andamio gateway actually returning status 409 (not 400, 422, or 500) when a client POSTs `/api/v2/course/teacher/course-module/register` with a duplicate `course_module_code`.**

No preprod smoke test was run as part of PR #68. The unit tests mock the status. If the gateway returns a different status code (wording drift, proxy rewrite, validation path firing before the conflict check), the recovery branch silently regresses: callers see `"failed to register module: ..."` on what the old predicate treated as an idempotent no-op.

Cross-reviewer consensus on this finding during ce:review of PR #68 (reliability P2 + correctness residual + project-standards residual). Documented in the plan's Risks & Dependencies table but not yet actioned.

## Affected Files

- `cmd/andamio/course_teacher_ops.go:301-311` — `isModuleAlreadyExistsError`
- `internal/client/client.go:60-72, 117-129, 166-178` — typed-error surfacing
- `docs/plans/2026-04-22-001-refactor-typed-conflict-error-plan.md` — Risks table row

## Options

### Option A (verification only, non-behavioral)

Capture a real preprod 409 response body for a duplicate `register-module` POST. Commit the raw HTTP status + body (or a sanitized fragment) as either:
- a test fixture under `internal/client/testdata/preprod-409-duplicate-module.txt`, OR
- a short appendix in `docs/COURSE-LIFECYCLE.md` under the "Already exists on register-module" troubleshooting section.

One-shot curl session to reproduce:

```bash
# Assume a course with module code 101 already exists in DRAFT
andamio course teacher register-module \
  --course-id <course-id> \
  --module-code 101 \
  --slt-hash <correct-matching-hash>
# On a SECOND run with a different user that hasn't seen the module, we need to
# capture the raw 409. Easiest is to hit the endpoint directly with curl and
# inspect Status-Line + body.
curl -i -X POST https://preprod.api.andamio.io/api/v2/course/teacher/course-module/register \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d '{"course_id":"<id>","course_module_code":"101","slt_hash":"<hash>"}' \
  | tee preprod-409-capture.txt
```

If the status line is indeed `HTTP/1.1 409 Conflict`, record it and close this todo as verified. If it's anything else (400/422/500 with a different body, or 409 with wording that doesn't contain both `"already exists"` and `"course_module_code"`), proceed to Option B.

### Option B (belt-and-braces fallback, behavioral change)

Widen `isModuleAlreadyExistsError` so the body-token match still fires on non-`ConflictError` errors whose body contains both tokens, with a stderr warning that the gateway wasn't recognized as returning 409:

```go
func isModuleAlreadyExistsError(err error) bool {
    if err == nil {
        return false
    }
    var conflict *apierr.ConflictError
    if errors.As(err, &conflict) {
        msg := strings.ToLower(conflict.Message)
        return strings.Contains(msg, "already exists") && strings.Contains(msg, "course_module_code")
    }
    // Fallback: the gateway may not have surfaced 409 for this conflict. If both
    // body tokens are present, treat as a conflict but warn the operator so the
    // gateway wording drift can be investigated.
    msg := strings.ToLower(err.Error())
    if strings.Contains(msg, "already exists") && strings.Contains(msg, "course_module_code") {
        fmt.Fprintf(os.Stderr, "warning: conflict detected via body-token fallback; gateway did not surface HTTP 409. Please report this wording/status drift.\n")
        return true
    }
    return false
}
```

Trade-off: preserves pre-PR-#68 idempotency across gateway status drift at the cost of potential false positives on pathological 5xx bodies that happen to contain both tokens.

**Preferred:** Option A first. Only do Option B if Option A reveals the gateway doesn't return 409.

## Acceptance

- [ ] Real preprod 409 body captured and recorded (fixture file OR doc appendix).
- [ ] If 409 is confirmed with the expected body tokens, close this todo as "verified — no code change needed."
- [ ] If the gateway returns a different status or different wording, implement Option B (with stderr warning) and add a test case to `TestIsModuleAlreadyExistsError` covering the fallback path.
- [ ] If Option B is implemented, update the plan's `Risks & Dependencies` table to mark the mitigation as "delivered" and update `isModuleAlreadyExistsError`'s doc comment to mention the fallback.

## Context

- **Plan:** `docs/plans/2026-04-22-001-refactor-typed-conflict-error-plan.md` (status: completed for the type-gate refactor itself; this todo tracks the deferred verification).
- **ce:review run artifact:** `.context/compound-engineering/ce-review/2026-04-22-pr68-conflict-error/findings.md`
- **Origin PR:** https://github.com/Andamio-Platform/andamio-cli/pull/68
- **Origin issue:** https://github.com/Andamio-Platform/andamio-cli/issues/64
