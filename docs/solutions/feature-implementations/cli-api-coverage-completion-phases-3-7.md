---
title: "Complete API coverage: role-based endpoint commands (phases 3-7)"
date: 2026-03-21
problem_type: feature-implementation
module: course teacher, course student, project manager, project contributor, public tasks
symptoms:
  - CLI only covered ~55% of Andamio API role-based endpoints
  - Students could not enroll or submit assignments via CLI
  - Contributors could not commit to project tasks via CLI
  - Teachers could not manage module lifecycle or review submissions via CLI
  - Managers could not view pending task assessments via CLI
root_cause: "Missing command implementations for all role-based POST endpoints across student, teacher, contributor, and manager roles"
severity: high
tags:
  - api-coverage
  - role-based-commands
  - factory-pattern
  - composability
  - cobra
  - post-endpoint
  - refactoring
---

# Complete API Coverage: Role-Based Endpoint Commands (Phases 3-7)

## Problem

The CLI covered roughly 55% of the Andamio API's role-based endpoints. Four entire user roles had no CLI commands:

- **Course students** could not enroll, submit assignments, update evidence, leave modules, or claim credentials.
- **Course teachers** could not register/publish/delete modules, update module status, or review student submissions.
- **Project contributors** could not commit to tasks, update evidence, or withdraw commitments.
- **Project managers** could not view pending task assessments.
- **Public users** had no way to list project tasks without the manager role.

This blocked agent workflows (e.g., `/assess-assignment`), scripted pipelines, and terminal-first developer workflows for every role except read-only consumers.

## Solution

Added 22 commands across 5 new files in a single PR (#35), organized by role. All commands follow the existing resource-role-action nesting convention (`andamio course student submit`, `andamio project contributor commit`).

### Phase 3 — Course Teacher Ops (6 commands)

**File:** `cmd/andamio/course_teacher_ops.go`

Parent command `course teacher` uses `PersistentPreRunE: jwtAuthPreRunE` to enforce JWT auth for all subcommands.

**Module lifecycle** (register-module, publish-module, delete-module) share a factory function:

```go
func runCourseTeacherModuleAction(endpoint, verb string) func(cmd *cobra.Command, args []string) error {
    return func(cmd *cobra.Command, args []string) error {
        courseID, _ := cmd.Flags().GetString("course-id")
        moduleCode, _ := cmd.Flags().GetString("module-code")
        // ... POST {course_id, course_module_code} to endpoint
    }
}
```

Three commands reuse this with different endpoints and verbs. Flag registration uses a loop over the three command pointers to avoid repetition.

**update-module-status** adds a `--status` flag to the same course-id + module-code pattern.

**review** validates `--decision` input before sending:

```go
if decision != "approve" && decision != "reject" {
    return fmt.Errorf("--decision must be 'approve' or 'reject', got %q", decision)
}
```

**commitments** uses the `printListPost` helper to list pending reviews.

### Phase 4 — Project Manager (1 command)

**File:** `cmd/andamio/project_manager_ops.go`

Single command `project manager commitments` lists pending task assessments. Delegates entirely to `printListPost`:

```go
func runProjectManagerCommitments(cmd *cobra.Command, args []string) error {
    projectID, _ := cmd.Flags().GetString("project-id")
    return printListPost(
        "/api/v2/project/manager/commitments/list",
        map[string]string{"project_id": projectID},
        "No pending assessments found.",
        "content.title", "commitment_id",
    )
}
```

### Phase 5 — Course Student (9 commands)

**File:** `cmd/andamio/course_student.go`

Parent command `course student` uses `PersistentPreRunE: jwtAuthPreRunE`.

**List commands** (courses, credentials, commitments) are inline `RunE` closures calling `printList`.

**commitment** (singular) does a POST to a `/get` endpoint with course-id + module-code payload.

**Lifecycle commands** use two factory functions:

- `runCourseStudentAction(endpoint, verb)` — for create, leave, claim (course-id + module-code payload).
- `runCourseStudentSubmitOrUpdate(endpoint, verb)` — for submit, update (adds `--evidence` to payload).

Flag registration uses loops: one for the three simple lifecycle commands, one for the two evidence commands.

### Phase 6 — Project Contributor (6 commands)

**File:** `cmd/andamio/project_contributor.go`

Parent command `project contributor` uses `PersistentPreRunE: jwtAuthPreRunE`.

**List commands** (list, commitments) are inline closures calling `printList`.

**commitment** does a POST to `/get` with project-id + task-index.

**Lifecycle commands** use `runProjectContributorAction(endpoint, verb)` factory for commit and delete. **update** is a standalone function because it adds `--evidence`.

### Phase 7 — Public Tasks (1 command)

**File:** `cmd/andamio/project.go` (added to existing file)

`project tasks <project-id>` uses the public user endpoint `/api/v2/project/user/tasks/list` instead of the manager endpoint. Takes project-id as a positional argument. Uses `printListPost`.

### Shared Infrastructure — printListPost helper

**File:** `cmd/andamio/helpers.go`

Extracted `printListPost` to eliminate three duplicated ~30-line functions across the new files. Handles:

1. Config load and client creation
2. POST with payload
3. JSON passthrough when `--output json`
4. Empty-data message to stderr
5. `output.PrintList` for text/csv/markdown formats

```go
func printListPost(path string, payload interface{}, emptyMsg, titleKey, idKey string) error
```

This complements the existing `printList` helper (which supports both GET and POST without payload).

## Command Reference

### course teacher

| Command | Endpoint | Flags |
|---------|----------|-------|
| `register-module` | `POST /v2/course/teacher/course-module/register` | `--course-id`, `--module-code` |
| `publish-module` | `POST /v2/course/teacher/course-module/publish` | `--course-id`, `--module-code` |
| `delete-module` | `POST /v2/course/teacher/course-module/delete` | `--course-id`, `--module-code` |
| `update-module-status` | `POST /v2/course/teacher/course-module/update-status` | `--course-id`, `--module-code`, `--status` |
| `review` | `POST /v2/course/teacher/assignment-commitment/review` | `--course-id`, `--commitment-id`, `--decision`, `--feedback` |
| `commitments` | `POST /v2/course/teacher/assignment-commitments/list` | `--course-id` |

### project manager

| Command | Endpoint | Flags |
|---------|----------|-------|
| `commitments` | `POST /v2/project/manager/commitments/list` | `--project-id` |

### course student

| Command | Endpoint | Flags |
|---------|----------|-------|
| `courses` | `POST /v2/course/student/courses/list` | none |
| `credentials` | `POST /v2/course/student/credentials/list` | none |
| `commitments` | `POST /v2/course/student/assignment-commitments/list` | none |
| `commitment` | `POST /v2/course/student/assignment-commitment/get` | `--course-id`, `--module-code` |
| `create` | `POST /v2/course/student/commitment/create` | `--course-id`, `--module-code` |
| `submit` | `POST /v2/course/student/commitment/submit` | `--course-id`, `--module-code`, `--evidence` |
| `update` | `POST /v2/course/student/commitment/update` | `--course-id`, `--module-code`, `--evidence` |
| `leave` | `POST /v2/course/student/commitment/leave` | `--course-id`, `--module-code` |
| `claim` | `POST /v2/course/student/commitment/claim` | `--course-id`, `--module-code` |

### project contributor

| Command | Endpoint | Flags |
|---------|----------|-------|
| `list` | `POST /v2/project/contributor/projects/list` | none |
| `commitments` | `POST /v2/project/contributor/commitments/list` | none |
| `commitment` | `POST /v2/project/contributor/commitment/get` | `--project-id`, `--task-index` |
| `commit` | `POST /v2/project/contributor/commitment/create` | `--project-id`, `--task-index` |
| `update` | `POST /v2/project/contributor/commitment/update` | `--project-id`, `--task-index`, `--evidence` |
| `delete` | `POST /v2/project/contributor/commitment/delete` | `--project-id`, `--task-index` |

### project (public)

| Command | Endpoint | Args |
|---------|----------|------|
| `tasks <project-id>` | `POST /v2/project/user/tasks/list` | positional project-id |

## Design Patterns

### Factory functions eliminate boilerplate

Each role group has commands that differ only by endpoint URL and a verb string. Factory functions return `func(cmd, args) error` closures:

- `runCourseTeacherModuleAction` — 3 callers
- `runCourseStudentAction` — 3 callers
- `runCourseStudentSubmitOrUpdate` — 2 callers
- `runProjectContributorAction` — 2 callers

### PersistentPreRunE chains auth checks

Each role parent command sets `PersistentPreRunE: jwtAuthPreRunE`. This function chains with the root command's `PersistentPreRunE` (which sets `--output` format) and then checks for JWT auth. All subcommands inherit this without per-command auth code.

### Composability preserved

All 22 commands follow the composability rules:

- Progress to stderr, data to stdout
- `--output json` suppresses progress messages
- Required flags enforced via `MarkFlagRequired`
- Discoverable: error messages reference the list command to find valid IDs

Example scripted workflow now possible:

```bash
# Student enrollment pipeline
COURSE_ID=$(andamio course student courses --output json | jq -r '.data[0].course_id')
andamio course student create --course-id "$COURSE_ID" --module-code 101
andamio course student submit --course-id "$COURSE_ID" --module-code 101 --evidence "https://github.com/..."
```

## Files Changed

| File | Change |
|------|--------|
| `cmd/andamio/course_teacher_ops.go` | New file: 6 course teacher commands + factory function |
| `cmd/andamio/project_manager_ops.go` | New file: 1 project manager command |
| `cmd/andamio/course_student.go` | New file: 9 course student commands + 2 factory functions |
| `cmd/andamio/project_contributor.go` | New file: 6 project contributor commands + factory function |
| `cmd/andamio/project.go` | Added `project tasks` public command |
| `cmd/andamio/helpers.go` | Added `printListPost` helper, `jwtAuthPreRunE` shared auth |
