---
title: "feat: add teacher assignment-commitments commands"
type: feat
status: completed
date: 2026-03-18
---

# Add teacher assignment-commitments commands

## Overview

Add `andamio teacher assignments list` and `andamio teacher assignments get` commands so the `/assess-assignment` agent skill can fetch pending student submissions. Optionally add `andamio teacher assignments assess` as a convenience wrapper. Time-sensitive: needed for CF Office Hours demo on Mar 21.

## Problem Statement

The assess-assignment agent skill needs to fetch pending student submissions to evaluate them against SLTs. The API endpoints exist but the CLI has no commands to call them. Step 3 of the agent flow (fetch pending submissions) is the gap blocking the demo from using live data.

## Proposed Solution

Add an `assignments` subcommand group under `teacher`:

```
andamio teacher
├── courses        (exists)
└── assignments    (new)
    ├── list       (POST /v2/course/teacher/assignment-commitments/list)
    ├── get        (POST /v2/course/teacher/assignment-commitment/review)
    └── assess     (optional — convenience wrapper for tx build)
```

### New file: `cmd/andamio/teacher_assignments.go`

Follow the `project_task.go` pattern — one file for the command group with all handlers.

## Implementation

### 1. `teacher assignments list`

```go
// cmd/andamio/teacher_assignments.go

var teacherAssignmentsCmd = &cobra.Command{
    Use:   "assignments",
    Short: "Manage assignment reviews (teacher role)",
}

var teacherAssignmentsListCmd = &cobra.Command{
    Use:   "list",
    Short: "List pending assignment commitments for review",
    RunE:  runTeacherAssignmentsList,
}
```

**Flags:**
- `--course <course-id>` — optional, filter by course
- `--module <module-code>` — optional, filter by module (requires `--course`)

**Handler:** POST to `/api/v2/course/teacher/assignment-commitments/list` with optional filters in the body. Use the same config→client→post→output pattern as existing teacher commands.

**Text output:** Formatted table with columns: STUDENT, COURSE, MODULE, STATUS, SUBMITTED

**JSON output:** Raw API response pass-through via `output.PrintJSON(resp)`

### 2. `teacher assignments get`

```go
var teacherAssignmentsGetCmd = &cobra.Command{
    Use:   "get <course-id> <module-code> <student-alias>",
    Short: "Get a specific assignment commitment for review",
    Args:  cobra.ExactArgs(3),
    RunE:  runTeacherAssignmentsGet,
}
```

**Handler:** POST to `/api/v2/course/teacher/assignment-commitment/review` with `course_id`, `module_code`, and `student_alias` in the body. Return full commitment details including submission content.

### 3. `teacher assignments assess` (optional)

Convenience wrapper that builds a body and calls `POST /v2/tx/course/teacher/assignments/assess` via the same pattern as `tx build`. Skip if time is tight — `tx build` already works for this.

### Registration

```go
func init() {
    teacherCmd.AddCommand(teacherAssignmentsCmd)
    teacherAssignmentsCmd.AddCommand(teacherAssignmentsListCmd)
    teacherAssignmentsCmd.AddCommand(teacherAssignmentsGetCmd)

    // List flags (all optional)
    teacherAssignmentsListCmd.Flags().String("course", "", "Filter by course ID")
    teacherAssignmentsListCmd.Flags().String("module", "", "Filter by module code (requires --course)")
}
```

Auth is inherited from `teacherCmd.PersistentPreRunE` — no per-command auth checks needed.

## Acceptance Criteria

- [x] `andamio teacher assignments list` returns pending commitments (text table + JSON)
- [x] `andamio teacher assignments list --course <id>` filters by course
- [ ] `andamio teacher assignments list --course <id> --module <code>` filters by module — deferred: API only accepts course_id filter
- [x] `andamio teacher assignments get <course> <module> <alias>` returns full commitment with submission
- [x] `--output json` produces stable JSON for agent consumption
- [x] Exit code 3 on auth failure, exit code 2 on not-found
- [x] Empty results: `{"data": []}` on stdout (JSON) or message on stderr (text)
- [x] All progress messages to stderr, data to stdout only

## Dependencies & Risks

**Verify before implementing:**
1. **API request body shape** for `/v2/course/teacher/assignment-commitments/list` — run `andamio spec paths --filter assignment` and check `openapi.json` for the expected payload
2. **API response shape** — what fields are in the commitment objects? Especially the submission content field name
3. **Filter parameters** — does the API accept `course_id` and `module_code` as filter fields in the POST body?

**Low risk**: This follows exact existing patterns (`teacher.go` + `project_task.go`). One new file, no changes to existing code. Auth already handled by `teacherCmd`.

## Implementation Notes

### Files to create/modify

| File | Change |
|------|--------|
| `cmd/andamio/teacher_assignments.go` | New file: command group + list/get handlers |
| `cmd/andamio/teacher.go` | No changes needed — assignments register via init() |

### Patterns to follow

- **List command**: Same as `teacherCoursesCmd` but with optional flag-based filters in POST body (like `project_task.go:runTasksList`)
- **Get command**: Same as `project_task.go:runTaskGet` — POST with args in body, return single item
- **Auth**: Inherited from `teacherCmd.PersistentPreRunE`
- **Output**: `output.PrintJSON(resp)` for JSON, custom table for text
- **Errors**: `getJSONWithHint` not needed — these are POST commands, use direct error returns

### Pre-implementation checklist

```bash
# 1. Check available endpoints
andamio spec paths --filter assignment

# 2. Check API response shape (try the endpoint directly)
andamio teacher assignments list --output json  # after implementing

# 3. Verify agent skill can consume the output
andamio teacher assignments list --output json | jq '.data[0]'
```

## Sources

- GitHub Issue: #20 — teacher assignment-commitments commands
- Pattern: `cmd/andamio/teacher.go` — teacher command group structure
- Pattern: `cmd/andamio/project_task.go` — complex POST-based command group
- Learning: `docs/solutions/architecture/command-structure-refactoring.md` — printList helper, PersistentPreRunE auth
- Learning: `docs/solutions/architecture/cli-composability-audit-and-fix.md` — stderr/stdout contract
