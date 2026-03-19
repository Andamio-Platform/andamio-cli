---
title: "Add teacher assignment-commitments commands"
date: 2026-03-18
problem_type: feature-implementation
module: teacher commands, assignment review
symptoms:
  - Agent skill /assess-assignment cannot fetch pending student submissions
  - No CLI commands for teacher assignment review workflow
  - Step 3 of agent flow blocked (fetch submissions)
root_cause: "Missing CLI commands for POST /v2/course/teacher/assignment-commitments/list and review endpoints"
severity: medium
tags:
  - teacher
  - assignments
  - agent-workflow
  - composability
  - post-endpoint
---

# Add teacher assignment-commitments commands

## Problem

The `/assess-assignment` agent skill needs to fetch pending student submissions to evaluate them against SLTs. The API endpoints exist (`POST /v2/course/teacher/assignment-commitments/list` and `POST /v2/course/teacher/assignment-commitment/review`), but the CLI had no commands to call them.

The agent flow was blocked at step 3:
1. `andamio course slts` — fetch SLTs (works)
2. `andamio course assignment` — fetch assignment prompt (works)
3. **Fetch pending submissions — missing**
4. Agent evaluates submissions — agent logic
5. `andamio tx build` — build assess TX (works)

## Solution

### New file: `cmd/andamio/teacher_assignments.go`

Added two commands under the existing `teacher` command group:

**`teacher assignments list`** — POST to `/v2/course/teacher/assignment-commitments/list`

```go
var teacherAssignmentsListCmd = &cobra.Command{
    Use:   "list",
    Short: "List pending assignment commitments for review",
    RunE:  runTeacherAssignmentsList,
}
```

- Without `--course`: lightweight on-chain-only summary across all courses
- With `--course <id>`: full merged history (on-chain + DB) with submission content
- Non-text formats pass through raw API response (fixes csv/markdown from the start)

**`teacher assignments get`** — filters from list endpoint client-side

```go
var teacherAssignmentsGetCmd = &cobra.Command{
    Use:   "get <course-id> <module-code> <student-alias>",
    Args:  cobra.ExactArgs(3),
    RunE:  runTeacherAssignmentsGet,
}
```

The API's `review` endpoint requires a `decision` field (accept/refuse) — it's an action, not a GET. So `get` fetches commitments for the course and filters by module + student client-side.

### Key API discovery

The OpenAPI spec revealed:
- `ListTeacherAssignmentCommitmentsRequest` only accepts `course_id` (no module filter)
- `ReviewAssignmentCommitmentV2Request` requires `decision` — it's a review action, not a read
- `TeacherAssignmentCommitmentItem` has `content.evidence` for the student's submission (Tiptap JSON)
- The assess TX endpoint (`/v2/tx/course/teacher/assignments/assess`) is already accessible via `andamio tx build`

### Review findings applied

| Finding | Fix |
|---------|-----|
| Empty list returns no JSON for agents | Non-text formats pass through raw API response before empty check |
| `get` not-found returns exit code 1 | Changed to `apierr.NotFoundError` (exit code 2) |
| csv/markdown fall through to text | Route all non-text formats through `output.PrintJSON` |

## Patterns Used

### Auth inheritance via PersistentPreRunE

No auth code needed in the new file. `teacherCmd` already validates JWT in its `PersistentPreRunE`, and all subcommands inherit it automatically.

### POST with optional body

```go
var body interface{}
if courseID != "" {
    body = map[string]string{"course_id": courseID}
}
c.Post(endpoint, body, &resp)  // nil body = POST with no body
```

Note: when `body` is nil, `client.Post` sends no `Content-Type` header. The API must handle bodyless POSTs.

### Client-side filtering when API lacks targeted endpoint

```go
// Fetch all, filter locally
for _, item := range data {
    m, _ := item.(map[string]interface{})
    if m["course_module_code"] == moduleCode && m["student_alias"] == studentAlias {
        return output.PrintJSON(m)
    }
}
return &apierr.NotFoundError{Message: "...hint to run list command..."}
```

### Non-text format dispatch (learned from prior review)

```go
// Always route non-text formats through PrintJSON BEFORE the empty check
if output.GetFormat() != output.FormatText {
    return output.PrintJSON(resp)
}
// Text-only: check empty, print table
```

## Prevention Checklist

When adding new teacher/manager commands:

- [ ] Auth is inherited from parent `PersistentPreRunE` — don't add per-command checks
- [ ] Check OpenAPI spec for request/response shapes before coding (`andamio spec paths --filter <keyword>`)
- [ ] Non-text format dispatch goes BEFORE empty-data checks
- [ ] Not-found conditions use `apierr.NotFoundError` (exit code 2), not `fmt.Errorf` (exit code 1)
- [ ] Error messages include hint to a discovery command
- [ ] Verify POST endpoints accept nil body if no filters are required

## Related Documentation

- `docs/solutions/architecture/command-structure-refactoring.md` — printList helper, PersistentPreRunE auth pattern
- `docs/solutions/architecture/cli-composability-audit-and-fix.md` — exit code contract, stderr/stdout separation
- `docs/solutions/integration-issues/cli-apikey-auth-isolation-and-content-404-ux.md` — output format dispatch pattern
- GitHub Issue: [#20](https://github.com/Andamio-Platform/andamio-cli/issues/20)
- GitHub PR: [#21](https://github.com/Andamio-Platform/andamio-cli/pull/21)

## Files

| File | Purpose |
|------|---------|
| `cmd/andamio/teacher_assignments.go` | New: list + get commands, 143 lines |
| `cmd/andamio/teacher.go` | Unchanged: parent command with auth guard |
