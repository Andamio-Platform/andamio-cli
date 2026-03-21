---
status: complete
priority: p2
issue_id: "019"
tags: [code-review, refactor, consistency]
dependencies: ["018"]
---

# Replace Inline JWT Auth in `project_task.go` with Shared `jwtAuthPreRunE`

## Problem Statement

PR #33 extracted `jwtAuthPreRunE` from `manager.go` and `teacher.go` but missed `project_task.go`, which still has the identical 12-line inline closure at lines 21-35. This is an incomplete refactor — three copies were consolidated but one remains.

## Findings

- `cmd/andamio/project_task.go:21-35` — inline closure identical to the extracted `jwtAuthPreRunE`
- `manager.go` and `teacher.go` already use the shared function
- The inline closure can be replaced with `PersistentPreRunE: jwtAuthPreRunE`
- This will also allow dropping `apierr` and `config` imports from the `PersistentPreRunE` (though `project_task.go` uses them elsewhere)

## Proposed Solutions

### Option A: Replace inline closure (Recommended)
Change `projectTaskCmd` to use `PersistentPreRunE: jwtAuthPreRunE`, matching `manager.go`, `teacher.go`, and `project_owner.go`.
- **Pros**: Eliminates last duplicate, ~12 LOC removed
- **Cons**: None
- **Effort**: Small (1 minute)
- **Risk**: None

## Acceptance Criteria

- [ ] `project_task.go` uses `jwtAuthPreRunE` instead of inline closure
- [ ] `go build` and `go vet` pass

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-03-21 | Created from PR #33 code simplicity + architecture review | When extracting shared functions, grep for all copies |

## Resources

- PR #33: https://github.com/Andamio-Platform/andamio-cli/pull/33
