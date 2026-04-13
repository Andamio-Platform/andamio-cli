---
title: "fix: clean DRAFT → APPROVED path when module already exists"
type: fix
status: completed
date: 2026-04-13
---

# fix: clean DRAFT → APPROVED path when module already exists

## Overview

Issue #57 reports that there is no clean CLI path to advance a module from `DRAFT → APPROVED → ON_CHAIN` when the module was created by `course import --create` (DRAFT) and then minted on-chain. Two sub-problems were reported:

1. `register-module` errors with `course_module_code already exists` when the DB record was created by `import --create`.
2. `update-module-status --status APPROVED` requires `slt_hash`, which the CLI did not expose.

Sub-problem (2) was already fixed in commit `621a48a` ("feat: draft-before-mint workflow and auto task_hash metadata"), which added `--slt-hash` to `update-module-status`. This plan closes the remaining gap on sub-problem (1) and tightens the documentation so the recovery path is discoverable.

## Problem Frame

A teacher who follows the documented happy path (Step 5: `update-module-status --status PENDING_TX` *before* submitting `modules_manage` TX) does not hit this issue — gateway batch-confirm advances the module to ON_CHAIN. But teachers who:

- Submit `modules_manage` while the DB module is still DRAFT, or
- Re-mint modules after a delete/recover cycle (the issue reporter's case — 6 modules on course `5f74e419…dec9e5`),

end up with a DB module in `DRAFT` whose on-chain counterpart already exists and is `source: merged`. The only existing CLI lever to leave DRAFT is `register-module`, which the gateway rejects on duplicate code. The user is stuck without a CLI path forward, even though the workaround (`update-module-status --status APPROVED --slt-hash <h>`) now exists.

The CLI should make `register-module` work as a recovery tool when the existing DB module is consistent with the on-chain one, and make the recovery path discoverable from the troubleshooting docs.

## Requirements Trace

- R1. `register-module` succeeds (and produces an APPROVED module) when the existing DB module is in DRAFT and its `slt_hash` matches the user-supplied `--slt-hash`.
- R2. `register-module` fails fast with an actionable message when the existing DB module's `slt_hash` does not match the supplied `--slt-hash` (do not silently mutate state).
- R3. `register-module` is a no-op (with a clear message) when the existing DB module is already APPROVED / PENDING_TX / ON_CHAIN with a matching `slt_hash`.
- R4. `--output json` returns a stable envelope on all three branches. Shape: `{action: "registered"|"advanced"|"already_registered", status: "<current-status>", slt_hash: "<supplied>", advanced_from: "DRAFT"|null}`. Scripts key off `action`.
- R5. `docs/COURSE-LIFECYCLE.md` documents the DRAFT → APPROVED recovery path that uses `update-module-status --status APPROVED --slt-hash`.

## Scope Boundaries

- **Out of scope**: changing gateway behavior (`course-module/register` semantics on the server). The fix is CLI-side composition over existing endpoints.
- **Out of scope**: making `publish-module` auto-advance DRAFT → APPROVED → ON_CHAIN. Conflating publish with a status transition risks silent state changes; recovery should remain explicit through `register-module` (which already implies "advance to APPROVED").
- **Out of scope**: changing `import --create` to skip module creation when the module is already on-chain. That is a separate concern (`docs/PLAN-content-sync.md`) and not what #57 asks for.
- **Out of scope**: any changes to project task lifecycle.

## Context & Research

### Relevant Code and Patterns

- `cmd/andamio/course_teacher_ops.go:122-157` — `runCourseTeacherRegisterModule`. Today posts a single payload and returns the raw error on conflict.
- `cmd/andamio/course_teacher_ops.go:252-289` — `runCourseTeacherUpdateModuleStatus`. Already accepts `--slt-hash` after `621a48a`. Will be invoked internally from the new `register-module` path.
- `cmd/andamio/course_teacher_ops.go:159-213` — `runCourseTeacherPublishModule`. Same pattern of inspecting the response for `source` and `warning` fields and printing a structured CLI message; mirror this approach for the new branches in `register-module`.
- `internal/client/client.go:115-125` — Non-2xx responses surface as an `error` whose message embeds the API body and status code. There is no typed error for "conflict / already exists"; the handler must string-match the error body. `404` already maps to `apierr.NotFoundError`.
- `cmd/andamio/course.go` — `getJSON` helper used to GET module metadata; the existing modules listing endpoint (`/api/v2/course/user/modules/{course_id}`, see CLI reference in `CLAUDE.md`) returns module records that include `course_module_code`, `slt_hash`, and `course_module_status`.
- `cmd/andamio/course_export.go` — Existing precedent for using the teacher list endpoint (`POST /v2/course/teacher/course-modules/list`) when draft + on-chain modules need to be inspected together. Prefer this over the user GET because draft modules may not appear in the user view.

### Institutional Learnings

- `docs/solutions/` — None of the indexed solutions cover this exact recovery; the closest related plan is `docs/plans/2026-03-23-fix-slt-hash-flag-for-chain-only-modules-plan.md`, which informed the original `--slt-hash` work in `621a48a`. The same pattern (CLI-side preflight + existing endpoint reuse) applies here.
- `docs/COURSE-LIFECYCLE.md:362-371` — Existing troubleshooting entry "course_module_code already exists on register-module" tells users to delete and re-import. After this fix, that entry should be replaced with the safer "advance to APPROVED" path; delete + re-import is destructive.

### External References

- None. This is CLI-side composition over the existing Andamio gateway. No external library or framework guidance needed.

## Key Technical Decisions

- **Decision 1: Detect conflict via error-body string match, not a typed error.** The `client` package surfaces conflicts as a generic `errors.New` with the API body inlined. A typed `ConflictError` would be cleaner long-term, but introducing it across all three Post sites is broader than this fix needs. **Rationale:** scope discipline. Add a TODO note for a future client refactor and pattern-match on `"already exists"` (and HTTP 409 if surfaced) in `register-module` only.
- **Decision 2: On conflict, fetch the existing module and compare `slt_hash` before any mutation.** Do not assume the user's `--slt-hash` is correct. **Rationale:** the destructive workaround in today's docs (delete + re-import) exists because hash mismatches are real; we must not silently approve a mismatched module.
- **Decision 3: Use the teacher list endpoint (`POST /api/v2/course/teacher/course-modules/list`) for the lookup, mirroring `course_export.go`.** **Rationale:** draft-only modules are not always visible via the user GET; teacher list returns both. Single endpoint covers all reachable states.
- **Decision 4: When DRAFT + matching hash, transparently call `update-module-status --status APPROVED --slt-hash <h>` rather than re-POSTing register.** **Rationale:** the gateway endpoint that *would* handle this is `update-status`, not `register`. Keep responsibilities aligned. Surface in the CLI message what actually happened (`Advanced module 101 from DRAFT to APPROVED`) so the user is not misled.
- **Decision 5: When already APPROVED/PENDING_TX/ON_CHAIN + matching hash, exit 0 with an "already registered" message.** **Rationale:** idempotency. A retry of `register-module` after a partially completed run should not error.
- **Decision 6: When hash mismatch on any existing status, exit non-zero with a remediation message that names both hashes and points to `delete-module` as the destructive recovery.** **Rationale:** preserve existing safety guarantees; this is the only path that loses data, so it must remain explicit.

## Open Questions

### Resolved During Planning

- **Should `publish-module` also handle the DRAFT case?** No — see Scope Boundaries Decision in this plan. `publish-module` is for finalizing the link, not for status transitions.
- **Should the conflict detection use HTTP 409 instead of string match?** Not now — `internal/client/client.go` does not preserve the status code distinct from the message, and confirming the gateway always returns 409 (vs. 400) is an extra discovery step. String match on the embedded body is sufficient and consistent with how other CLI commands recover.

### Deferred to Implementation

- Exact phrasing of the conflict-detection match. The current gateway error string from #57 is `"course_module_code already exists in this course"`; the implementer should confirm the live string and use a tolerant substring match (`strings.Contains(err.Error(), "already exists")`) rather than equality.
- Whether the teacher-list response field is `slt_hash` or `course_module_slt_hash` — confirm against the live API response when wiring the lookup. Both names appear elsewhere in the codebase.

## Implementation Units

- [x] **Unit 1: Refactor `register-module` to detect conflict and inspect existing module**

**Goal:** When `POST /api/v2/course/teacher/course-module/register` returns a "already exists" error, fetch the existing module via the teacher list endpoint and route to the correct branch (advance, no-op, or mismatch error) based on its current status and `slt_hash`.

**Requirements:** R1, R2, R3, R4

**Dependencies:** None.

**Files:**
- Modify: `cmd/andamio/course_teacher_ops.go`
- Test: `cmd/andamio/course_teacher_ops_test.go` (create — first test file for this command group)

**Approach:**
- **Input validation (fail-fast):** At the top of `runCourseTeacherRegisterModule`, reject empty `--slt-hash` values with `slt-hash must be non-empty`. `cobra.MarkFlagRequired` only enforces the flag was set, not that its value is non-empty, and an empty hash could silently match an on-chain module whose `slt_hash` is null/empty.
- In `runCourseTeacherRegisterModule`, wrap the existing `c.Post` call. On error, check whether the message **both** contains `"already exists"` **and** mentions a module context (`"course_module_code"` or `"module"`). This narrows the match so unrelated "already exists" errors from future fields (duplicate teacher, duplicate credential, etc.) don't route into the module-lookup recovery branch. If the match fails, return the original error unchanged.
- On conflict, GET the module list via `POST /api/v2/course/teacher/course-modules/list` with `{"course_id": <id>}`, find the entry with matching `course_module_code`, and read `slt_hash` and `course_module_status`. Use a defensive field lookup that tries both `slt_hash` and `course_module_slt_hash`.
- Branch:
  - **Hash matches, status DRAFT** → call the shared helper (see below). Print `Module <code>: advanced from DRAFT to APPROVED.` to stderr; return the stable envelope with `action: "advanced"`, `advanced_from: "DRAFT"`, `status: "APPROVED"`.
  - **Hash matches, status APPROVED/PENDING_TX/ON_CHAIN** → no-op. Print `Module <code>: already registered (status: <status>).` to stderr; return the envelope with `action: "already_registered"`, `advanced_from: null`, `status: "<current>"`.
  - **Hash mismatch** → return an error wrapping the original gateway error via `%w`, whose message names both hashes: `module <code> already exists with slt_hash <existing> (you supplied <given>). To replace, run: andamio course teacher delete-module --course-id <id> --module-code <code>: %w`. Do not perform any destructive action automatically.
- **Shared helper boundary:** Extract `postUpdateModuleStatus(c *client.Client, courseID, moduleCode, status, sltHash string) (map[string]interface{}, error)`. The helper performs only the HTTP POST and returns the raw response. It does **not** print progress or results — the caller is responsible for all stdout/stderr output. Rewrite `runCourseTeacherUpdateModuleStatus` to call the helper and keep its existing output shape; `runCourseTeacherRegisterModule` composes its own progress/JSON output from the envelope.
- Suppress stderr progress when `--output json`, mirroring the convention in `course_export.go` and `course_teacher_ops.go:177-179`.

**Patterns to follow:**
- `cmd/andamio/course_teacher_ops.go:159-213` — response inspection + structured CLI/JSON output for `publish-module`.
- `cmd/andamio/course_export.go` — calling `POST /v2/course/teacher/course-modules/list` and iterating returned modules.
- `cmd/andamio/course.go` — `getJSON` helper for read-only calls.

**Test scenarios:**
- `register-module` succeeds normally (no-conflict path): unchanged behavior, returns the gateway response wrapped in the envelope with `action: "registered"`.
- Empty `--slt-hash` (e.g., `--slt-hash ""`): returns `slt-hash must be non-empty` before any HTTP call.
- Conflict + DRAFT + matching hash: invokes helper, returns advanced message and envelope `{action: "advanced", advanced_from: "DRAFT", status: "APPROVED", slt_hash: "<h>"}`.
- Conflict + APPROVED + matching hash: no-op, envelope `{action: "already_registered", advanced_from: null, status: "APPROVED", slt_hash: "<h>"}`.
- Conflict + ON_CHAIN + matching hash: no-op, same shape as APPROVED branch with `status: "ON_CHAIN"`.
- Conflict + hash mismatch (any status): returns non-zero error mentioning both hashes and `delete-module` remediation; original gateway error accessible via `errors.Unwrap`.
- Conflict but module not present in teacher-list response: returns a clear "could not locate existing module" error rather than panicking on nil access.
- "Already exists" error without module context (e.g., a future "teacher already exists" error): returned verbatim, does not route into the module-lookup branch.
- Non-conflict POST error (e.g., 401): returned verbatim, unchanged.

**Verification:**
- Manual test against preprod against the course in #57 (`5f74e419…dec9e5`): pick one module stuck at DRAFT after merged on-chain mint, run `register-module --slt-hash <h>`, observe it advances to APPROVED.
- `go test ./cmd/andamio/...` covers branching with stub HTTP responses (mirror the test harness used in `course_export_test.go`).

- [x] **Unit 2: Update troubleshooting and quick-reference docs**

**Goal:** Replace the existing "delete and re-import" advice with the new safer recovery, and add a dedicated entry for the DRAFT → APPROVED case.

**Requirements:** R5

**Dependencies:** Unit 1 (so the documented behavior matches the code).

**Files:**
- Modify: `docs/COURSE-LIFECYCLE.md`
- Modify: `CLAUDE.md` (CLI command reference table for `course teacher register-module` — add a one-line note that it now also advances DRAFT→APPROVED on hash match)

**Approach:**
- Rewrite `### "course_module_code already exists" on register-module` (around `docs/COURSE-LIFECYCLE.md:362-371`) to:
  1. Describe the new behavior (`register-module` is now idempotent on hash match).
  2. Point to `delete-module` only as the hash-mismatch escape hatch.
- Add a new troubleshooting entry: `### Module on-chain (source: merged) but stuck at DRAFT` describing the issue from #57 and pointing to `register-module` as the canonical fix.
- Update the Quick Reference table at `docs/COURSE-LIFECYCLE.md:389-402` if needed: keep Step 6 framing but note that `register-module` now also handles the "module already exists in DRAFT" case.
- In `CLAUDE.md`, append a short note to the `course teacher register-module` row: "If the module already exists in DRAFT with a matching `slt_hash`, advances it to APPROVED."

**Patterns to follow:**
- Existing troubleshooting entries in `docs/COURSE-LIFECYCLE.md` (Cause / Fix structure).

**Test scenarios:** N/A (docs only).

**Verification:**
- Render the updated `COURSE-LIFECYCLE.md` and confirm the recovery flow reads end-to-end.
- Confirm `andamio course teacher register-module --help` still matches the documented behavior (no flag changes in this unit).

## System-Wide Impact

- **Interaction graph:** `register-module` now consumes `POST /api/v2/course/teacher/course-modules/list` and `POST /api/v2/course/teacher/course-module/update-status` in addition to its current endpoint. All three are existing gateway routes already exercised elsewhere in the CLI and are teacher-scoped JWT endpoints; no new permission is expected. Confirm during manual preprod testing.
- **Error propagation:** Conflict path now hides the original gateway error string from the user when the recovery succeeds. On hash mismatch, the original gateway error is replaced with a CLI-composed message — keep the original error wrapped via `%w` so `--output json` consumers can still see the underlying message if they introspect.
- **State lifecycle risks:** The advance branch performs a write (status update) after a read (module list). A racing user-initiated state change between read and write is theoretically possible; in practice the only relevant transitions are gateway-driven (PENDING_TX → ON_CHAIN), and the update-status endpoint will reject an invalid transition. Surface that gateway error verbatim if it occurs.
- **API surface parity:** `update-module-status --status APPROVED --slt-hash <h>` continues to work directly. We are not removing or changing any flag.
- **Integration coverage:** Manual end-to-end against preprod is required because the conflict detection depends on the live gateway error string. Unit tests with stub responses cover branching logic but not the string match itself.

## Risks & Dependencies

- **Risk:** Gateway changes the exact "already exists" error wording in a future release, breaking the substring match. **Mitigation:** Tolerant `strings.Contains` match on a short stem (`"already exists"`); add an inline TODO referencing a future typed-error refactor in `internal/client/`.
- **Risk:** Teacher-list response shape diverges between environments (preprod vs. mainnet) on the `slt_hash` field name. **Mitigation:** Resolve at implementation time by inspecting one live response per environment; use a defensive lookup that tries both candidate field names.
- **Risk:** Users could lose work if they misread the mismatch error and run `delete-module` on a module they meant to keep. **Mitigation:** Mismatch error includes both hashes verbatim so the user can verify before destructive action; we deliberately do not auto-delete.

## Documentation / Operational Notes

- No rollout / migration concerns — CLI-side change, no schema or persistent state involved.
- No new monitoring or alerts required.
- Release note (for next `scripts/release.sh`):
  > **`register-module` is now idempotent on hash match.** When a module already exists in DRAFT with a matching `slt_hash`, it advances to APPROVED instead of erroring; when already APPROVED/PENDING_TX/ON_CHAIN, it exits as a no-op. **Breaking change for `--output json` consumers:** the response is now wrapped in an envelope `{action, status, slt_hash, advanced_from, response}`. Scripts that previously parsed gateway fields at the top level must now read them under `.response.*`. The `action` field (`"registered"` / `"advanced"` / `"already_registered"`) is the new branching key.
- **Behavior delta for callers:** Any CI/automation that today treats `register-module`'s "already exists" failure as a success signal (e.g., "module is already registered, keep going") must be audited. Matching-hash conflicts now exit 0 with a state side-effect (advance-from-draft) or 0 with a no-op, not non-zero as before. Mismatching-hash conflicts still exit non-zero.

## Sources & References

- Issue: https://github.com/Andamio-Platform/andamio-cli/issues/57
- Prior fix that delivered `--slt-hash` to `update-module-status`: commit `621a48a` ("feat: draft-before-mint workflow and auto task_hash metadata")
- Related plan: `docs/plans/2026-03-23-fix-slt-hash-flag-for-chain-only-modules-plan.md`
- Existing troubleshooting entry to be replaced: `docs/COURSE-LIFECYCLE.md:362-371`
- Affected handler: `cmd/andamio/course_teacher_ops.go:122-157`
