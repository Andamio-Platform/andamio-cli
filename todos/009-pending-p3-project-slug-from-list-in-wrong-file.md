---
status: pending
priority: p3
issue_id: "009"
tags: [code-review, quality, task-commands]
dependencies: []
---

# `projectSlugFromList` Defined in Export File but Only Used by Import

## Problem Statement

`projectSlugFromList` is defined at the end of `project_task_export.go` but its only caller is `project_task_import.go`. Meanwhile `runTaskExport` derives the slug inline with duplicate logic. This creates confusion and duplication.

## Findings

- `project_task_export.go:249–257`: `projectSlugFromList` defined here
- `project_task_import.go:91`: only caller
- `project_task_export.go:68–71`: inline slug derivation in `runTaskExport` that duplicates the same logic

## Proposed Solution

Move `projectSlugFromList` to `project_task.go` (alongside other shared helpers like `resolveProject`, `fetchManagerProjects`) and have `runTaskExport` call it instead of inlining. Removes ~4 lines of duplication.

## Acceptance Criteria

- [ ] `projectSlugFromList` lives in `project_task.go`
- [ ] `runTaskExport` calls it instead of inlining the slug logic
- [ ] No behavioral change

## Work Log

- 2026-03-18: Identified via simplicity + architecture review of feat/project-task-commands branch
