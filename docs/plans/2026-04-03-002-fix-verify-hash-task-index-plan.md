---
title: "fix: verify-hash displays index 0 for all tasks"
type: fix
status: completed
date: 2026-04-03
---

# fix: verify-hash displays index 0 for all tasks

## Overview

Fix `project task verify-hash` to correctly read the task index from the API response. Currently displays `Task 0` for every task because `chain_only` tasks lack a top-level `task_index` field.

## Problem Frame

The user endpoint `/api/v2/project/user/tasks/list` returns two task structures depending on `source`:

- **`chain_only` tasks** (on-chain only, no DB record): No top-level `task_index`. The index is only available nested at `content.task_index` (if a DB record exists in `content`).
- **`merged` tasks** (both chain and DB): Top-level `task_index` is present.

The `verify-hash` command does `m["task_index"].(float64)` which silently returns 0 for `chain_only` tasks. Since the command specifically targets on-chain tasks (skips those without `task_hash`), most tasks it processes are `chain_only`.

## Requirements Trace

- R1. Display correct task index for all tasks in `verify-hash` output
- R2. Handle `chain_only` tasks that lack top-level `task_index`
- R3. Maintain correct JSON output schema for `--output json`

## Scope Boundaries

- Not in scope: Changing which endpoint `verify-hash` uses (staying on user endpoint for broader auth compatibility)
- Not in scope: Fixing other commands that may have similar issues

## Key Technical Decisions

- **Fallback chain for task index**: Try top-level `task_index` first, then `content.task_index` nested field, then use 1-based position in the array. The position fallback is a last resort since on-chain ordering may differ from array ordering.

## Implementation Units

- [ ] **Unit 1: Fix task index extraction in verify-hash**

  **Goal:** Read task_index from the correct location in the API response

  **Requirements:** R1, R2, R3

  **Files:**
  - Modify: `cmd/andamio/project_task.go`

  **Approach:**
  - At line 731, replace the single type assertion with a fallback chain
  - Try `m["task_index"].(float64)` first (works for merged tasks)
  - If zero, try `m["content"].(map[string]interface{})["task_index"].(float64)` (works for chain_only tasks with content)
  - If still zero, use `i + 1` (1-based position in the data array) as last resort

  **Verification:**
  - `andamio project task verify-hash <project-id>` shows correct indices, not all zeros
