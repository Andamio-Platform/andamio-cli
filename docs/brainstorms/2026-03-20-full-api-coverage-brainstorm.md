# Full API Coverage — All Role-Based Endpoints

**Date:** 2026-03-20
**Status:** Brainstorm
**Author:** James + Claude

## What We're Building

Complete CLI coverage of all Andamio API public endpoints, matching what the template app consumes. Today the CLI covers 28 of 61 relevant endpoints (46%). The goal is to close the gap on all role-based CRUD endpoints for courses and projects.

### The Gap

The CLI already covers:
- All **public read** endpoints (course list/get/modules/slts/lessons, project list/get)
- All **transaction** endpoints (build, sign, submit, register, status, run)
- **Teacher** course list + assignment list
- **Manager** project/task CRUD
- **API key** and **user** basics

What's missing — 38 endpoints across 7 role groups:

| Role Group | Missing | Endpoints |
|------------|---------|-----------|
| Course Owner | 5 | courses/list, course/create, course/update, course/register, teachers/update |
| Course Teacher | 4 | module register/publish/delete/update-status, assignment-commitment/review |
| Course Student | 9 | courses/list, credentials/list, commitment CRUD (create/submit/update/leave/claim), assignment-commitment/get, assignment-commitments/list |
| Project Owner | 4 | projects/list, project/create, project/update, project/register |
| Project Manager | 1 | commitments/list (pending assessments) |
| Project Contributor | 6 | projects/list, commitments/list, commitment/get, commitment/create, commitment/update, commitment/delete |
| Project User | 1 | user/tasks/list (public task listing via POST) |

### Not In Scope

- User utility endpoints (delete account, init-roles, access-token-alias) — low priority, rarely used from CLI
- Admin endpoints — internal only
- Billing endpoints — browser-only flow
- Developer auth (register/login/verify) — browser-only flow
- SSE streaming (`tx/stream/{hash}`) — polling via `tx status` is sufficient

## Why This Approach

1. **Parity with template** — anything a developer can do in the template app, they should be able to do from the CLI. This is a core principle of the developer toolchain.
2. **Role-based nesting** — `andamio course owner create`, `andamio project contributor commit`. Groups by resource first, role second. Consistent with existing `project task` pattern.
3. **Named flags for input** — `--title`, `--description`, etc. for common fields, with `--body`/`--body-file` as escape hatch for complex payloads. Discoverable via `--help`.
4. **One PR per role group** — 7 incremental PRs, each reviewable and releasable independently. Follows lifecycle order: owners create, teachers/managers configure, students/contributors use.

## Key Decisions

### 1. Command structure: `resource role action`

```
andamio course owner list
andamio course owner create --title "..." --description "..."
andamio course student commitments
andamio course student submit --course-id <id> --module <code>
andamio project owner list
andamio project contributor commit --project-id <id> --task-index <n>
```

Existing `teacher` and `manager` top-level commands stay as-is — they're shortcuts. New nested commands under `course teacher` and `project manager` are the canonical location. Both work, no deprecation.

### 2. Input pattern: named flags + escape hatch

For create/update operations:
- Named flags for the common fields (`--title`, `--description`, `--course-id`, etc.)
- `--body` / `--body-file` as escape hatch when the payload is complex or the flags don't cover all fields
- Flags and `--body` are mutually exclusive

For list/get operations:
- Positional args or required flags for IDs
- No request body needed (or empty POST body for JWT-authenticated lists)

### 3. Implementation order: lifecycle sequence

1. **Course Owner** (5 endpoints) — create/manage courses
2. **Project Owner** (4 endpoints) — create/manage projects
3. **Course Teacher** (4 endpoints) — module lifecycle + reviews
4. **Project Manager** (1 endpoint) — pending assessments
5. **Course Student** (9 endpoints) — enrollment + submissions
6. **Project Contributor** (6 endpoints) — task commitments
7. **Project User** (1 endpoint) — public task listing

### 4. Existing commands: keep both

Top-level `teacher` and `manager` stay. New nested `course teacher` and `project manager` are canonical but don't deprecate the old ones. Users can use either.

### 5. Auth: all new endpoints require JWT

Every new endpoint is a POST that requires JWT auth. Use the same `PreRunE` pattern from `tx_build.go` / `tx_register.go`: load config, check `HasUserAuth()`, return `AuthError` if not.

### 6. File organization: one file per role group

Following the existing pattern of one file per command group:
- `cmd/andamio/course_owner.go`
- `cmd/andamio/course_student.go`
- `cmd/andamio/project_owner.go`
- `cmd/andamio/project_contributor.go`
- etc.

### 7. POST list helpers

Many role-based list endpoints are `POST` with an empty body (just needs JWT). Extract a `postJSON` helper analogous to the existing `getJSON` helper to reduce boilerplate.

## Resolved Questions

- **What about the user utility endpoints?** — Skip for now. Delete account, init-roles, and access-token-alias are rarely needed from CLI.
- **What about inconsistency with existing top-level commands?** — Keep both. No deprecation. Accept that `andamio teacher courses` and `andamio course teacher courses` both work.
- **How to handle complex payloads?** — Named flags for common fields, `--body`/`--body-file` escape hatch. Mutually exclusive.
- **Implementation approach?** — One PR per role group, lifecycle order.

## Open Questions

(None — all resolved.)
