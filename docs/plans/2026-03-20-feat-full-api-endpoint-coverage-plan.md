---
title: "feat: full API endpoint coverage for all role-based operations"
type: feat
status: active
date: 2026-03-20
origin: docs/brainstorms/2026-03-20-full-api-coverage-brainstorm.md
---

# feat: full API endpoint coverage for all role-based operations

## Overview

Add CLI commands for all 38 missing Andamio API role-based endpoints, bringing coverage from 46% to 100% of public endpoints used by the template app. Delivered as 7 incremental PRs in lifecycle order, one per role group.

(see brainstorm: docs/brainstorms/2026-03-20-full-api-coverage-brainstorm.md)

## Problem Statement

The CLI covers all public read endpoints and transaction operations, but lacks commands for role-based CRUD: course owners can't create courses, students can't submit assignments, project contributors can't commit to tasks. These operations require users to fall back to the template app or raw API calls.

## Proposed Solution

Add role-based subcommand groups nested under domain parents:

```
andamio course owner list|create|update|register|teachers
andamio course teacher register-module|publish-module|delete-module|update-module-status|review
andamio course student courses|credentials|commitments|commitment|submit|claim|leave
andamio project owner list|create|update|register
andamio project manager commitments
andamio project contributor list|commitments|commitment|commit|update|delete
andamio project tasks
```

Existing `andamio teacher` and `andamio manager` top-level commands stay as-is. No deprecation.

## Technical Approach

### Command structure: `resource role action`

Each role group gets a parent command attached to the domain parent (`courseCmd` or `projectCmd`) with `PersistentPreRunE` for JWT auth. Subcommands inherit auth automatically.

```go
// cmd/andamio/course_owner.go
var courseOwnerCmd = &cobra.Command{
    Use:   "owner",
    Short: "Course owner operations (requires user login)",
    PersistentPreRunE: jwtAuthPreRunE,
}

func init() {
    courseCmd.AddCommand(courseOwnerCmd)
    courseOwnerCmd.AddCommand(courseOwnerListCmd)
    // ...
}
```

### Shared auth helper

Extract the repeated `PersistentPreRunE` pattern into a reusable function (used by all 7 role parents + existing `teacher.go` and `manager.go`):

```go
// cmd/andamio/helpers.go (or course.go where other helpers live)
func jwtAuthPreRunE(cmd *cobra.Command, args []string) error {
    if err := rootCmd.PersistentPreRunE(cmd, args); err != nil {
        return err
    }
    cfg, err := config.Load()
    if err != nil {
        return err
    }
    if !cfg.HasUserAuth() {
        return &apierr.AuthError{Message: "not authenticated. Run 'andamio user login' first"}
    }
    return nil
}
```

### Input patterns

**List endpoints:** Use existing `printList(path, emptyMsg, titleKey, idKey, usePost)` helper. Most role-based lists are POST with empty body.

**Get endpoints:** Use `postJSON(path)` or build payload with ID flags.

**Create/Update endpoints:** Named flags for common fields, `--body`/`--body-file` escape hatch. Update commands use `cmd.Flags().Changed()` for partial updates.

### File organization

One file per role group, following existing naming:

| File | Role Group | Commands |
|------|-----------|----------|
| `course_owner.go` | Course Owner | 5 commands |
| `course_teacher_ops.go` | Course Teacher (new) | 5 commands |
| `course_student.go` | Course Student | 9 commands |
| `project_owner.go` | Project Owner | 4 commands |
| `project_manager_ops.go` | Project Manager (new) | 1 command |
| `project_contributor.go` | Project Contributor | 6 commands |
| `project_tasks_public.go` | Project User | 1 command |

Note: `course_teacher_ops.go` (not `course_teacher.go`) to avoid confusion with the existing top-level `teacher.go`. Same for `project_manager_ops.go`.

## Implementation Steps

### Phase 0: Shared auth helper extraction

- [ ] Extract `jwtAuthPreRunE` function from the repeated pattern in `teacher.go` and `manager.go`
- [ ] Refactor `teacher.go` and `manager.go` to use `jwtAuthPreRunE`
- [ ] Verify existing commands still work

### Phase 1: Course Owner (PR #1) — 5 endpoints

**File:** `cmd/andamio/course_owner.go`

| Command | Endpoint | Flags |
|---------|----------|-------|
| `course owner list` | `POST /v2/course/owner/courses/list` | (none — empty body POST) |
| `course owner create` | `POST /v2/course/owner/course/create` | `--title`, `--description`, `--image-url` |
| `course owner update` | `POST /v2/course/owner/course/update` | `--course-id` (required), `--title`, `--description`, `--image-url` |
| `course owner register` | `POST /v2/course/owner/course/register` | `--course-id` (required) |
| `course owner teachers` | `POST /v2/course/owner/teachers/update` | `--course-id` (required), `--teachers` (repeatable alias list) |

Implementation steps:
- [ ] Create `courseOwnerCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [ ] `course owner list` — use `printList` with `usePost: true`
- [ ] `course owner create` — named flags, POST, print result
- [ ] `course owner update` — named flags with `Changed()` guards, POST
- [ ] `course owner register` — `--course-id` required flag, POST
- [ ] `course owner teachers` — `--course-id` + `--teachers` StringArray, POST
- [ ] Update CLAUDE.md command reference
- [ ] Build + test

### Phase 2: Project Owner (PR #2) — 4 endpoints

**File:** `cmd/andamio/project_owner.go`

| Command | Endpoint | Flags |
|---------|----------|-------|
| `project owner list` | `POST /v2/project/owner/projects/list` | (none) |
| `project owner create` | `POST /v2/project/owner/project/create` | `--title`, `--description`, `--image-url` |
| `project owner update` | `POST /v2/project/owner/project/update` | `--project-id` (required), `--title`, `--description`, `--image-url` |
| `project owner register` | `POST /v2/project/owner/project/register` | `--project-id` (required) |

Implementation steps:
- [ ] Create `projectOwnerCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [ ] `project owner list` — `printList` with `usePost: true`
- [ ] `project owner create` — named flags, POST
- [ ] `project owner update` — named flags with `Changed()`, POST
- [ ] `project owner register` — `--project-id` required, POST
- [ ] Update CLAUDE.md command reference
- [ ] Build + test

### Phase 3: Course Teacher extras (PR #3) — 5 endpoints

**File:** `cmd/andamio/course_teacher_ops.go`

These complete the teacher workflow beyond what `teacher.go` already covers.

| Command | Endpoint | Flags |
|---------|----------|-------|
| `course teacher register-module` | `POST /v2/course/teacher/course-module/register` | `--course-id`, `--module-code` |
| `course teacher publish-module` | `POST /v2/course/teacher/course-module/publish` | `--course-id`, `--module-code` |
| `course teacher delete-module` | `POST /v2/course/teacher/course-module/delete` | `--course-id`, `--module-code` |
| `course teacher update-module-status` | `POST /v2/course/teacher/course-module/update-status` | `--course-id`, `--module-code`, `--status` |
| `course teacher review` | `POST /v2/course/teacher/assignment-commitment/review` | `--course-id`, `--commitment-id`, `--decision` (approve/reject), `--feedback` |

Implementation steps:
- [ ] Create `courseTeacherCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [ ] Attach to `courseCmd`
- [ ] Module lifecycle commands (register, publish, delete, update-status) — all take `--course-id` + `--module-code`
- [ ] `course teacher review` — takes commitment ID + decision + optional feedback
- [ ] Update CLAUDE.md command reference
- [ ] Build + test

### Phase 4: Project Manager extras (PR #4) — 1 endpoint

**File:** `cmd/andamio/project_manager_ops.go`

| Command | Endpoint | Flags |
|---------|----------|-------|
| `project manager commitments` | `POST /v2/project/manager/commitments/list` | `--project-id` (required) |

Implementation steps:
- [ ] Create `projectManagerCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [ ] Attach to `projectCmd`
- [ ] `project manager commitments` — `printList` with project-id in body, `usePost: true`
- [ ] Update CLAUDE.md command reference
- [ ] Build + test

### Phase 5: Course Student (PR #5) — 9 endpoints

**File:** `cmd/andamio/course_student.go`

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

Implementation steps:
- [ ] Create `courseStudentCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [ ] List commands (courses, credentials, commitments) — `printList` with `usePost: true`
- [ ] `course student commitment` (get single) — POST with course-id + module-code
- [ ] Commitment lifecycle commands (create, submit, update, leave, claim) — named flags
- [ ] Update CLAUDE.md command reference
- [ ] Build + test

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

Implementation steps:
- [ ] Create `projectContributorCmd` parent with `PersistentPreRunE: jwtAuthPreRunE`
- [ ] List commands — `printList` with `usePost: true`
- [ ] `project contributor commitment` (get single) — POST with project-id + task-index
- [ ] Commitment lifecycle (commit, update, delete) — named flags
- [ ] Update CLAUDE.md command reference
- [ ] Build + test

### Phase 7: Project User public tasks (PR #7) — 1 endpoint

**File:** `cmd/andamio/project_tasks_public.go`

| Command | Endpoint | Flags |
|---------|----------|-------|
| `project tasks` | `POST /v2/project/user/tasks/list` | `--project-id` (required) |

Implementation steps:
- [ ] Add `projectTasksPublicCmd` to `projectCmd`
- [ ] Uses `PreRunE` (not PersistentPreRunE) — this endpoint may not require JWT
- [ ] `printList` with project-id body, `usePost: true`
- [ ] Update CLAUDE.md command reference
- [ ] Build + test

## Acceptance Criteria

### Per-PR checklist (apply to each of the 7 PRs)

- [ ] All endpoints for the role group have CLI commands
- [ ] Commands follow `andamio <resource> <role> <action>` structure
- [ ] JWT auth checked via parent `PersistentPreRunE`
- [ ] `--output json` returns stable JSON on stdout
- [ ] Progress/errors go to stderr
- [ ] No stdin reads, no interactive prompts
- [ ] Required flags use `MarkFlagRequired`
- [ ] Update commands use `Changed()` for partial updates
- [ ] User-supplied IDs escaped with `url.PathEscape`
- [ ] CLAUDE.md command reference updated
- [ ] `go build`, `go test`, `go vet` pass

### Overall acceptance

- [ ] All 38 endpoints covered
- [ ] CLAUDE.md command reference complete
- [ ] Existing commands unchanged (backwards compatible)
- [ ] `jwtAuthPreRunE` shared across all role parents

## Dependencies & Risks

- **API payload shapes** — The exact request/response fields for each endpoint need to be verified against the OpenAPI spec or template app code. The flag names listed above are best guesses; adjust during implementation.
- **Endpoint auth requirements** — All role endpoints should require JWT, but verify each one. Some may also need specific roles (owner vs teacher vs student).
- **No new dependencies** — All commands use existing packages (`client`, `config`, `output`, `apierr`).

## Sources

- **Origin brainstorm:** [docs/brainstorms/2026-03-20-full-api-coverage-brainstorm.md](../brainstorms/2026-03-20-full-api-coverage-brainstorm.md) — key decisions: role-as-subcommand structure, named flags + escape hatch, lifecycle implementation order, keep existing top-level commands
- **Command patterns:** `cmd/andamio/teacher.go`, `cmd/andamio/project_task.go` — auth and flag patterns
- **Helpers:** `cmd/andamio/course.go` — `getJSON`, `postJSON`, `printList`
- **Composability rules:** `docs/solutions/architecture/cli-composability-audit-and-fix.md`
- **Security:** `docs/solutions/security-issues/cli-security-hardening-input-validation.md` — validate/escape user inputs
- **Flag patterns:** `docs/solutions/feature-implementations/project-task-token-flag.md` — `Changed()` for updates
