---
status: pending
priority: p2
issue_id: "006"
tags: [code-review, bug, task-commands, ux]
dependencies: []
---

# `project task update` Sends No-Op API Call When No Update Flags Provided

## Problem Statement

`runTaskUpdate` sends a POST to `/api/v2/project/manager/task/update` even when no content flags (`--title`, `--lovelace`, `--expiration`, `--content`) are provided. The payload only contains `project_state_policy_id` and `index`. This results in a silent no-op (or confusing API error), with no user feedback that nothing was actually changed.

**Why it matters:** A user running `andamio project task update 3 --project-id abc123` with no other flags expects an error telling them what to change, not a silent "Task 3 updated successfully."

## Findings

- `project_task.go:490–515` — payload built with only required fields when no `Changed()` flags are set
- `project_task.go:516–519` — success message printed regardless
- All content fields guarded by `cmd.Flags().Changed(...)` — correct pattern, but no guard for the case where none are changed

## Proposed Solutions

### Option A: Check if any content flags were changed before calling API (Recommended)
After the payload is built, check if any optional field was added. If not, return an informative error:

```go
contentFields := []string{"title", "lovelace", "expiration", "content"}
hasUpdates := false
for _, f := range contentFields {
    if cmd.Flags().Changed(f) {
        hasUpdates = true
        break
    }
}
if !hasUpdates {
    return fmt.Errorf("no fields to update: specify at least one of --title, --lovelace, --expiration, --content")
}
```

**Pros:** Clear user feedback, prevents unnecessary API calls
**Effort:** Small | **Risk:** None

### Option B: Cobra `MarkFlagsMutuallyExclusiveWith` or `AtLeastOneRequired`
Cobra supports `MarkFlagsOneRequired` (v1.8+) to require at least one of a group of flags at the framework level.

**Pros:** Framework-enforced, shows in `--help`
**Cons:** Requires checking Cobra version compatibility
**Effort:** Small | **Risk:** Low

## Recommended Action

Option A — simple runtime check. Consistent with the existing `validateLovelace`-style inline validation pattern.

## Technical Details

- **File:** `cmd/andamio/project_task.go` lines 490–515
- **Insert guard** before the API call at line 516

## Acceptance Criteria

- [ ] `project task update 3 --project-id abc123` (no content flags) returns a clear error
- [ ] `project task update 3 --project-id abc123 --title "New title"` works correctly
- [ ] Error message lists the available flags

## Work Log

- 2026-03-18: Identified via simplicity review of feat/project-task-commands branch
