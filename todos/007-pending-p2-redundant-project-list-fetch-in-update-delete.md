---
status: pending
priority: p2
issue_id: "007"
tags: [code-review, architecture, performance, task-commands]
dependencies: []
---

# Redundant Project List Fetch in `update` and `delete`

## Problem Statement

`runTaskUpdate` and `runTaskDelete` both require `--project-id` as a mandatory flag, yet they still call `fetchManagerProjects(c)` — a full POST to `/api/v2/project/manager/projects/list` — solely to look up `project_state_policy_id`. This means every `update` and `delete` invocation makes one extra network round-trip before the actual mutation.

Additionally, `runTaskCreate` calls `resolveProject` (which fetches the project list) and then calls `findProjectPolicyID(projects, proj.ProjectID)` — a second linear scan over the same slice to get a field already on `proj.ContributorStateID`.

**Why it matters:** Extra latency on every mutation, extra failure surface (update/delete fail if the projects-list endpoint is temporarily unavailable even though we have the project ID), and confusing code that appears to be a mistake.

## Findings

- `project_task.go:474–482` (`runTaskUpdate`): `fetchManagerProjects` + `findProjectPolicyID` when `--project-id` is already required
- `project_task.go:543–551` (`runTaskDelete`): same pattern
- `project_task.go:373–383` (`runTaskCreate`): `resolveProject` returns `proj` with `ContributorStateID`, then `findProjectPolicyID(projects, proj.ProjectID)` re-scans the same slice
- `project_task_export.go:72` (`runTaskExport`): correctly reads `proj.ContributorStateID` directly — proves the field is available and the indirect path is unnecessary

## Proposed Solutions

### Option A: Use `resolveProject` pattern for update/delete, read policyID directly from proj (Recommended)

For `runTaskUpdate` and `runTaskDelete`: since `--project-id` is required, call `resolveProject` the same way `runTaskCreate` does (or just `fetchManagerProjects` + find). BUT then read `policyID` directly from the returned `*managerProject.ContributorStateID` instead of calling `findProjectPolicyID` separately.

For `runTaskCreate`: replace `findProjectPolicyID(projects, proj.ProjectID)` with:
```go
policyID := proj.ContributorStateID
if policyID == "" {
    return fmt.Errorf("project %s has no contributor_state_id (may not be on-chain yet)", proj.ProjectID)
}
```

**Pros:** Removes redundant scan in create; documents the constraint clearly
**Effort:** Small | **Risk:** Low

### Option B: Add a single-project lookup endpoint
If the API provides a way to look up `project_state_policy_id` by `project_id` without fetching all managed projects, use that instead.

**Pros:** Single targeted request
**Cons:** Requires API endpoint that may not exist
**Effort:** Medium | **Risk:** Medium

### Option C: Cache the project list in config during the session
Store the projects list in memory for the duration of a command invocation to avoid repeated fetches.

**Pros:** Transparent to callers
**Cons:** Over-engineering for a CLI tool
**Effort:** Medium | **Risk:** Low

## Recommended Action

Option A — consolidate the existing patterns. Add a comment in `findProjectPolicyID` or the update/delete path explaining that the projects-list fetch is required to obtain `project_state_policy_id` (the API needs this, not `project_id` directly).

## Technical Details

- **File:** `cmd/andamio/project_task.go`
- `runTaskCreate` lines 373–383: replace `findProjectPolicyID` call with direct field access
- `runTaskUpdate` lines 474–482: add comment explaining the fetch necessity, or refactor to read from project struct
- `runTaskDelete` lines 543–551: same

## Acceptance Criteria

- [ ] `runTaskCreate` does not call `findProjectPolicyID` — reads `proj.ContributorStateID` directly
- [ ] `runTaskUpdate` and `runTaskDelete` have a comment explaining why the project list fetch is necessary
- [ ] No behavioral change to any command

## Work Log

- 2026-03-18: Identified via architecture review of feat/project-task-commands branch
