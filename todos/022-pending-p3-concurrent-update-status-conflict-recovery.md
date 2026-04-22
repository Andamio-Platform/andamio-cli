---
status: pending
priority: p3
issue_id: "022"
tags: [code-review, reliability, concurrency, register-module, pr-68-followup]
dependencies: []
---

# Concurrent update-status 409 Not Caught in register-module Recovery

## Problem Statement

PR #68 migrated register-module's conflict detection to a typed-error check via `isModuleAlreadyExistsError`. That predicate catches 409s on the **initial** register POST. It does NOT catch 409s on the **follow-up** `postUpdateModuleStatus` call inside the recovery branch.

Scenario: two agents race `register-module --course-id X --module-code 101 --slt-hash <h>` on the same module:

1. Agent A's register POST succeeds (200) → module advanced to APPROVED → Agent A exits 0.
2. Agent B's register POST returns 409 → recovery branch triggered.
3. Agent B's `lookupTeacherModule` finds module in DRAFT (read happens after Agent A's 200 is committed but before the teacher-list cache refreshes) OR finds it in APPROVED.
4. If it finds DRAFT (stale read): Agent B calls `postUpdateModuleStatus(APPROVED, sltHash)`. The gateway rejects because the module is no longer in DRAFT — likely returning 409 or a status-mismatch error.
5. Agent B surfaces: `"module 101 exists in DRAFT with matching hash, but advancing to APPROVED failed: <gateway error>"` — a hard, non-zero exit.

The desired end state (module APPROVED) has already been reached. The race loser should treat the update-status 409 as "already done" and exit 0 via the `already_registered` envelope, not fail.

This pre-existed from PR #63 but became more visible post-PR-#68 because the typed-error boundary now cleanly distinguishes conflict-class errors from other errors.

Found during ce:review interactive re-review of PR #68 (reliability P3, confidence 0.70).

## Affected Files

- `cmd/andamio/course_teacher_ops.go:247-252` — the DRAFT-advance branch in `registerOrRecoverModule`

## Proposed Fix

Wrap the `postUpdateModuleStatus` call inside the DRAFT branch with conflict-aware recovery. On update-status error:

1. If the error is a `*apierr.ConflictError` whose body indicates the module is no longer in DRAFT (or a similar "status already past DRAFT" wording), re-invoke `lookupTeacherModule` to read the current status.
2. If the re-read shows APPROVED / PENDING_TX / ON_CHAIN with matching hash, fold the result into the existing `already_registered` envelope — treat as success, exit 0.
3. If the re-read shows a different slt_hash or an unexpected status, surface the original `advancing to APPROVED failed: ...` error as today.

Alternative (simpler): extract a second `isStatusTransitionConflict` predicate and apply the same loop. Needs explicit testing.

## Acceptance

- [ ] A new test case in `TestRegisterOrRecoverModule` simulating a concurrent race: register returns 409, lookup returns DRAFT, update-status returns 409 (simulating another agent having advanced the module). Assert the envelope returns `action: "already_registered"` with the current gateway status.
- [ ] Handler logic re-reads status when update-status returns a conflict-class error on the DRAFT branch.
- [ ] Envelope `status` field reflects the current gateway state, not the expected APPROVED.
- [ ] Existing `TestRegisterOrRecoverModule` subtests continue to pass unchanged.

## Context

- **ce:review run artifact:** `.context/compound-engineering/ce-review/2026-04-22-pr68-conflict-error/findings.md`
- **Origin PR:** https://github.com/Andamio-Platform/andamio-cli/pull/68 (finding surfaced during interactive re-review after the autofix pass)
- **Related:** `todos/021-pending-p2-verify-gateway-409-for-duplicate-module.md` — if the gateway's 409 wording for status-transition conflicts is captured as part of the P2 preprod verification, use that body format for the test fixture here too.
