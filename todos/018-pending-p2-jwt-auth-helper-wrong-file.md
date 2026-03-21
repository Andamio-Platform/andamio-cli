---
status: complete
priority: p2
issue_id: "018"
tags: [code-review, architecture, refactor]
dependencies: []
---

# Move `jwtAuthPreRunE` Out of `course.go`

## Problem Statement

The shared `jwtAuthPreRunE` function was extracted from inline closures in `manager.go` and `teacher.go` (PR #33) but placed in `course.go`. This function is a cross-cutting auth concern used by `manager`, `teacher`, `project owner`, and should also be used by `project task`. Placing it in a domain-specific file creates misleading coupling — anyone reading `manager.go` or `project_owner.go` has to look in `course.go` to find it.

As the 7-PR API coverage push continues, more command groups will reference this function, deepening the misplacement.

## Findings

- `jwtAuthPreRunE` defined at `cmd/andamio/course.go:122-136`
- Referenced by: `manager.go`, `teacher.go`, `project_owner.go`
- `project_task.go:21-35` still has an inline duplicate (see todo #019)
- Other general-purpose helpers (`getJSON`, `postJSON`, `printList`, `truncateUTF8`) also live in `course.go` — existing debt, but this PR should not deepen it

## Proposed Solutions

### Option A: Create `cmd/andamio/helpers.go` (Recommended)
Move `jwtAuthPreRunE` to a new `helpers.go` file. Optionally move other general-purpose helpers there too.
- **Pros**: Clean separation, obvious discovery location
- **Cons**: One more file
- **Effort**: Small (move function, no logic change)
- **Risk**: None

### Option B: Create `cmd/andamio/auth.go`
Dedicated file just for auth helpers.
- **Pros**: Very clear naming
- **Cons**: Might be overkill for one function
- **Effort**: Small
- **Risk**: None

## Acceptance Criteria

- [ ] `jwtAuthPreRunE` is not in `course.go`
- [ ] All references (`manager.go`, `teacher.go`, `project_owner.go`, `project_task.go`) use the shared function
- [ ] `go build` and `go vet` pass

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-03-21 | Created from PR #33 architecture review | Cross-cutting concerns should not live in domain files |

## Resources

- PR #33: https://github.com/Andamio-Platform/andamio-cli/pull/33
