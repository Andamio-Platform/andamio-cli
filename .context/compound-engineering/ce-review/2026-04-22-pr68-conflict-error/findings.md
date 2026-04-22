---
run_id: 2026-04-22-pr68-conflict-error
mode: autofix
pr: 68
base: c986f6c8fcc711f1972019badf6f8bd915aab783
plan: docs/plans/2026-04-22-001-refactor-typed-conflict-error-plan.md
reviewers: correctness, testing, maintainability, project-standards, agent-native, learnings-researcher, reliability
---

# ce:review autofix run — PR #68 (typed ConflictError)

## Intent

Add `apierr.ConflictError` for HTTP 409. Surface from `client.Get/Post/Put`. Migrate `isModuleAlreadyExistsError` from pure string matching to three-gate check (type + `already exists` stem + `course_module_code` field). Closes #64.

## Verdict

**Ready with follow-up.** No blocking findings. Three `safe_auto` fixes applied; one `gated_auto` residual captured as a todo.

## Applied safe_auto fixes

| # | File | Fix | Reviewer |
|---|------|-----|----------|
| 1 | `internal/apierr/errors.go` | Trimmed `ConflictError` doc comment — removed the "Callers use `errors.As`..." sentence that explained standard Go idiom. Now matches the terseness of sibling `NotFoundError`/`AuthError` docs. | maintainability |
| 2 | `cmd/andamio/course_teacher_ops.go` | Trimmed the `isModuleAlreadyExistsError` doc comment — removed the trailing two-sentence refactor-history narration ("The type gate replaces what the body match was silently doing..."). Kept the enumerated three-gate rationale (which carries the WHY). | maintainability |
| 3 | `internal/client/client_test.go` | Added `strings.Contains(err.Message, "403")` and `strings.Contains(err.Message, "404")` assertions to the 403 and 404 subtests to mirror the 401/409 pattern. Catches future regressions where the message string drops the status-code prefix. | testing |

All three are non-behavioral; `go test ./...` stays green, `go vet ./...` clean, `go build ./...` clean.

## Residual actionable (gated_auto → downstream-resolver)

### [P2] Status-only type gate can silently regress idempotency if the gateway ever returns non-409 for duplicate module_code

**File:** `cmd/andamio/course_teacher_ops.go:305-310`

**Cross-reviewer consensus:** reliability (P2, 0.82) + correctness residual (0.65) + project-standards residual (0.70). Merged confidence 0.92.

**Why it matters:** The new three-gate predicate ANDs `errors.As(*apierr.ConflictError)` with the two body substrings. Before this PR, the predicate matched body text alone regardless of status code. If preprod or mainnet ever returns 400/422/500 for a duplicate `course_module_code` POST (wording drift, proxy rewrite, validation path firing before the conflict check), the recovery branch is skipped and callers see `"failed to register module: ..."` on what used to be an idempotent no-op. The plan's Risks & Dependencies table acknowledges this but defers the mitigation.

**Fix options (concrete):**
- **Option A (verification):** Capture a real preprod 409 response body for a duplicate `register-module` POST. Commit the body (or a sanitized fragment) as a test fixture, or document it in `docs/COURSE-LIFECYCLE.md`. One-shot curl session. Non-behavioral.
- **Option B (belt-and-braces fallback):** In `isModuleAlreadyExistsError`, if `errors.As` fails but the raw `err.Error()` contains both body tokens, emit a warning to stderr and still return `true`. Preserves pre-PR-#68 idempotency across status-code drift at the cost of potential false positives on pathological 5xx bodies. Behavioral change — requires user sign-off.

**Why gated_auto, not safe_auto:** Option B changes predicate behavior (widens beyond what the plan chose). Option A requires a human action (preprod request). Neither is mechanical.

**Owner:** downstream-resolver (human follow-up on a future CLI session or a separate issue).

## Advisory (human)

### [P3] Document the three-gate pattern in `docs/solutions/architecture/`

**Source:** learnings-researcher

**Why:** The approach extends Prevention Strategy #2 from `docs/solutions/feature-implementations/cli-course-module-management-commands.md` ("sentinel errors, not string matching"), but the specific three-gate idiom (typed error + two body substrings when status alone is ambiguous) is new institutional pattern worth capturing. A short ~40-line doc under `docs/solutions/architecture/typed-error-three-gate-conflict-detection.md` cross-linked from the three existing solution docs would close the knowledge loop.

**Owner:** human (judgment call — optional documentation work, not required for merge).

## Not flagged (below threshold or out of scope)

- `TestClient_200OK_DecodesBody` doesn't cover the `result == nil` path in Post/Put — pre-existing contract, not this PR's regression target.
- `lookupTeacherModule` has no pagination handling — pre-existing from PR #63; confidence 0.55 below the 0.60 gate; would silently miss modules on very large courses if the gateway ever paginates.
- `isModuleAlreadyExistsError` test table has 5 type-gate negatives where 2 would suffice — mild over-testing; reviewer explicitly noted "not worth blocking the PR over."
- The 3× duplicated `switch resp.StatusCode` block in `client.go` — plan explicitly defers with a "5th-case trigger" threshold; all reviewers endorsed the deferral.

## Requirements completeness (plan: explicit)

| # | Requirement | Status |
|---|---|---|
| R1 | `apierr.ConflictError` exists with `Message`/`Error()` mirroring `NotFoundError`/`AuthError`, scoped HTTP-derived | met — `internal/apierr/errors.go:15-18` |
| R2 | `client.Get`/`Post`/`Put` return `*apierr.ConflictError` for 409 | met — `client_test.go:TestClient_StatusCodeToTypedError` covers all 3 methods |
| R3 | `isModuleAlreadyExistsError` uses `errors.As(err, &conflict)` with narrowing body check | met — `cmd/andamio/course_teacher_ops.go:301-311` |
| R4 | Existing `TestRegisterOrRecoverModule` continues to pass without modification | met — verified end-to-end (httptest 409 → real client → typed error → recovery flow) |
| R5 | `TestIsModuleAlreadyExistsError` updated to feed `*apierr.ConflictError` directly | met — 14 cases with isolated gate-negative coverage |

All 5 met. All 3 implementation units checked off in the plan.
