---
title: "feat: complete API coverage — phases 3-7 (teacher, manager, student, contributor)"
type: feat
status: completed
date: 2026-03-21
origin: docs/brainstorms/2026-03-20-full-api-coverage-brainstorm.md
---

# Complete API Coverage — Phases 3-7

## Overview

Finish the remaining 22 endpoints to reach full CLI-to-API parity with the template app. Phases 0-2 are complete (PRs #31-34). This plan covers phases 3-7: course teacher ops, project manager extras, course student, project contributor, and public task listing.

**Current state:** 55% coverage (53/109 endpoints, or ~60/61 relevant endpoints after excluding admin/billing/developer-auth)
**Target:** 100% of role-based endpoints used by the template app

(see brainstorm: docs/brainstorms/2026-03-20-full-api-coverage-brainstorm.md)

## What's Already Done

| Phase | Role | PR | Status |
|-------|------|-----|--------|
| 0 | Auth helper extraction | #33 | Merged — `jwtAuthPreRunE` in helpers.go |
| 1 | Course Owner (5 endpoints) | #32 | Merged — course_owner.go |
| 2 | Project Owner (4 endpoints) | #33 | Merged — project_owner.go |
| — | Headless login, hex encoding, lesson merge | #34 | Merged — fixes #28, #8, #29 |

## Remaining Phases

### Phase 3: Course Teacher Ops (PR #3) — 5 endpoints

**File:** `cmd/andamio/course_teacher_ops.go`

These complete the teacher workflow beyond what `teacher.go` already covers (list courses, list assignments).

| Command | Endpoint | Flags |
|---------|----------|-------|
| `course teacher register-module` | `POST /v2/course/teacher/course-module/register` | `--course-id`, `--module-code` |
| `course teacher publish-module` | `POST /v2/course/teacher/course-module/publish` | `--course-id`, `--module-code` |
| `course teacher delete-module` | `POST /v2/course/teacher/course-module/delete` | `--course-id`, `--module-code` |
| `course teacher update-module-status` | `POST /v2/course/teacher/course-module/update-status` | `--course-id`, `--module-code`, `--status` |
| `course teacher review` | `POST /v2/course/teacher/assignment-commitment/review` | `--course-id`, `--commitment-id`, `--decision` (approve/reject), `--feedback` |

**Implementation:**
- [x] Create `courseTeacherCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [x] Attach to `courseCmd` (not top-level — `teacher` top-level stays as-is)
- [x] Module lifecycle: register, publish, delete, update-status — all take `--course-id` + `--module-code`
- [x] `course teacher review` — commitment ID + decision + optional feedback
- [x] Update CLAUDE.md command reference
- [x] Build + vet + test

**Pattern reference:** `cmd/andamio/course_owner.go` — same structure, same auth, same flag patterns.

---

### Phase 4: Project Manager Extras (PR #4) — 1 endpoint

**File:** `cmd/andamio/project_manager_ops.go`

| Command | Endpoint | Flags |
|---------|----------|-------|
| `project manager commitments` | `POST /v2/project/manager/commitments/list` | `--project-id` (required) |

**Implementation:**
- [x] Create `projectManagerCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [x] Attach to `projectCmd`
- [x] `project manager commitments` — POST with project-id in body, use `printList` pattern
- [x] Update CLAUDE.md command reference
- [x] Build + vet + test

**Note:** This is the smallest phase — just one endpoint for viewing pending assessments. Could be bundled with Phase 3 if convenient.

---

### Phase 5: Course Student (PR #5) — 9 endpoints

**File:** `cmd/andamio/course_student.go`

This is the largest phase — the full student journey from enrollment through credential claim.

| Command | Endpoint | Flags |
|---------|----------|-------|
| `course student courses` | `POST /v2/course/student/courses/list` | (none) |
| `course student credentials` | `POST /v2/course/student/credentials/list` | (none) |
| `course student commitments` | `POST /v2/course/student/assignment-commitments/list` | (none) |
| `course student commitment` | `POST /v2/course/student/assignment-commitment/get` | `--course-id`, `--module-code` |
| `course student create` | `POST /v2/course/student/commitment/create` | `--course-id`, `--module-code` |
| `course student submit` | `POST /v2/course/student/commitment/submit` | `--course-id`, `--module-code`, `--evidence` |
| `course student update` | `POST /v2/course/student/commitment/update` | `--course-id`, `--module-code`, `--evidence` |
| `course student leave` | `POST /v2/course/student/commitment/leave` | `--course-id`, `--module-code` |
| `course student claim` | `POST /v2/course/student/commitment/claim` | `--course-id`, `--module-code` |

**Implementation:**
- [x] Create `courseStudentCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [x] List commands (courses, credentials, commitments) — `printList` with `usePost: true`
- [x] `course student commitment` (get single) — POST with course-id + module-code
- [x] Commitment lifecycle (create, submit, update, leave, claim) — named flags, POST
- [x] `--evidence` flag for submit/update — string or `--evidence-file` for file content
- [x] Update CLAUDE.md command reference
- [x] Build + vet + test

**Composability example:**
```bash
# Enroll, complete, and claim — fully scriptable
COURSE_ID=$(andamio course list -o json | jq -r '.data[0].course_id')
andamio course student create --course-id "$COURSE_ID" --module-code 101
andamio course student submit --course-id "$COURSE_ID" --module-code 101 --evidence "https://github.com/..."
# Wait for teacher review, then:
andamio course student claim --course-id "$COURSE_ID" --module-code 101
```

---

### Phase 6: Project Contributor (PR #6) — 6 endpoints

**File:** `cmd/andamio/project_contributor.go`

| Command | Endpoint | Flags |
|---------|----------|-------|
| `project contributor list` | `POST /v2/project/contributor/projects/list` | (none) |
| `project contributor commitments` | `POST /v2/project/contributor/commitments/list` | (none) |
| `project contributor commitment` | `POST /v2/project/contributor/commitment/get` | `--project-id`, `--task-index` |
| `project contributor commit` | `POST /v2/project/contributor/commitment/create` | `--project-id`, `--task-index` |
| `project contributor update` | `POST /v2/project/contributor/commitment/update` | `--project-id`, `--task-index`, `--evidence` |
| `project contributor delete` | `POST /v2/project/contributor/commitment/delete` | `--project-id`, `--task-index` |

**Implementation:**
- [x] Create `projectContributorCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [x] List commands — `printList` with `usePost: true`
- [x] `project contributor commitment` (get single) — POST with project-id + task-index
- [x] Commitment lifecycle (commit, update, delete) — named flags
- [x] Update CLAUDE.md command reference
- [x] Build + vet + test

---

### Phase 7: Public Task Listing (PR #7) — 1 endpoint

**File:** Add to existing `cmd/andamio/project.go` or new `project_tasks_public.go`

| Command | Endpoint | Flags |
|---------|----------|-------|
| `project tasks` | `POST /v2/project/user/tasks/list` | `--project-id` (required) |

**Implementation:**
- [x] Add `projectTasksPublicCmd` to `projectCmd`
- [x] May not require JWT (verify — uses `user` namespace)
- [x] `printList` with project-id in body
- [x] Update CLAUDE.md
- [x] Build + vet + test

**Note:** This is distinct from `project task list` (manager endpoint). This is the public user-facing task view.

---

## Suggested Delivery Order

Phases 3+4 can be a single PR (6 endpoints, both small). Phase 5 is the largest and most impactful. Phase 7 is trivial and can be bundled anywhere.

| PR | Phases | Endpoints | Priority |
|----|--------|-----------|----------|
| Next PR | 3 + 4 | 6 | High — completes teacher/manager workflows |
| After | 5 | 9 | High — enables student journey from CLI |
| After | 6 | 6 | Medium — contributor workflow |
| After | 7 | 1 | Low — public task listing |

## Acceptance Criteria (Per PR)

- [x] All endpoints for the role group have CLI commands
- [x] Commands follow `andamio <resource> <role> <action>` structure
- [x] JWT auth via parent `PersistentPreRunE: jwtAuthPreRunE`
- [x] `--output json` returns stable JSON on stdout
- [x] Progress/errors go to stderr
- [x] No stdin reads, no interactive prompts
- [x] Required flags use `MarkFlagRequired`
- [x] Update commands use `Changed()` for partial updates
- [x] CLAUDE.md command reference updated
- [x] `go build`, `go vet`, `go test` pass

## Overall Acceptance

- [x] All 22 remaining endpoints covered (phases 3-7)
- [x] CLAUDE.md command reference complete for all roles
- [x] Existing commands unchanged (backwards compatible)
- [x] andamio-docs CLI guides updated

## Dependencies & Risks

- **API payload shapes:** Verify exact request/response fields against OpenAPI spec during implementation. Flag names above are based on the spec but may need adjustment.
- **Evidence submission format:** Course student submit and project contributor update may need file upload or URL — verify what the API expects.
- **No new Go dependencies** — all commands use existing packages.

## Sources

- **Origin brainstorm:** [docs/brainstorms/2026-03-20-full-api-coverage-brainstorm.md](../brainstorms/2026-03-20-full-api-coverage-brainstorm.md) — key decisions: role-as-subcommand, named flags + escape hatch, lifecycle order
- **Prior plan:** [docs/plans/2026-03-20-feat-full-api-endpoint-coverage-plan.md](2026-03-20-feat-full-api-endpoint-coverage-plan.md) — phases 0-2 complete
- **Pattern reference:** `cmd/andamio/course_owner.go`, `cmd/andamio/project_owner.go` — implemented in phases 1-2
- **Helpers:** `cmd/andamio/helpers.go` — `jwtAuthPreRunE`, `printList`, `postJSON`
- **Coverage analysis:** CLI vs API audit from 2026-03-21 showing 55% coverage (53/109)
- PRs: #31, #32, #33, #34
