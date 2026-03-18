---
status: pending
priority: p2
issue_id: "008"
tags: [code-review, bug, import, task-commands, data-safety]
dependencies: []
---

# Import Silently Skips DRAFT Guard When `fetchTasks` Fails

## Problem Statement

In `runTaskImport`, the initial `fetchTasks` call (used to check task status before updating) silently swallows errors. If the call fails, `existingTasks` is nil and the import proceeds — sending update API calls against tasks without verifying they are in DRAFT status. A non-DRAFT (e.g. on-chain) task could receive an update call, potentially returning an API error or silently no-op'ing against a live task.

## Findings

- `project_task_import.go:121–124`:
  ```go
  resp, err := fetchTasks(c, proj.ProjectID)
  if err == nil {
      existingTasks = extractTaskList(resp)
  }
  // err is silently ignored
  ```
- `project_task_import.go:220–229`: DRAFT guard is skipped when `existingTasks` is empty
- Security review flagged this as a logic correctness issue; learnings researcher confirms the pattern of swallowing fetch errors in import was noted in prior course import work

## Proposed Solutions

### Option A: Fail loudly when `fetchTasks` fails (Recommended)
Return an error when `fetchTasks` fails rather than continuing without the guard:

```go
resp, err := fetchTasks(c, proj.ProjectID)
if err != nil {
    return fmt.Errorf("failed to fetch existing tasks (needed for DRAFT status check): %w", err)
}
existingTasks = extractTaskList(resp)
```

**Pros:** Safe by default, clear error message
**Cons:** User must retry if the list call is flaky
**Effort:** Minimal | **Risk:** None

### Option B: Warn and require explicit `--force` to proceed without DRAFT check
Print a warning when `fetchTasks` fails, and only continue if `--force` flag is set.

**Pros:** Gives advanced users an escape hatch
**Cons:** Adds flag complexity
**Effort:** Small | **Risk:** Low

### Option C: Require `--skip-draft-check` to proceed without the guard
More explicit than `--force`, makes the intent obvious.

**Pros:** Self-documenting flag
**Cons:** Verbose
**Effort:** Small | **Risk:** Low

## Recommended Action

Option A — fail loudly. The DRAFT check exists to protect against accidentally modifying on-chain tasks; silently bypassing it on a fetch failure is the wrong default.

## Technical Details

- **File:** `cmd/andamio/project_task_import.go` lines 121–124
- One-line fix: change `if err == nil {` block to `if err != nil { return ... }; existingTasks = extractTaskList(resp)`

## Acceptance Criteria

- [ ] If `fetchTasks` fails during import, the import returns an error with a clear message
- [ ] Import does not proceed without the DRAFT guard when the task list cannot be fetched
- [ ] `--dry-run` path has the same protection

## Work Log

- 2026-03-18: Identified via security review of feat/project-task-commands branch
