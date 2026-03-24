---
title: "fix: Add --task-hash flag for chain-only tasks"
type: fix
status: completed
date: 2026-03-24
origin: GitHub Issue #45
---

# Add --task-hash Flag for Chain-Only Tasks

## Problem

Chain-only tasks have no `task_index`, so `resolveTaskHash()` cannot find them. Same pattern as #42 (chain-only modules lacking `course_module_code`, fixed with `--slt-hash`).

## Proposed Solution

Add `--task-hash` as alternative to `--task-index` on contributor commands that use `resolveTaskHash`. Mirror the `resolveSltHashFromFlags` pattern from #42.

### Commands to update

| Command | Uses `resolveTaskHash` | Needs `--task-hash` |
|---------|----------------------|-------------------|
| `project contributor commit` | Yes (via `loadClientAndResolveTask`) | Yes |
| `project contributor update` | Yes (via `loadClientAndResolveTask`) | Yes |
| `project contributor delete` | Yes (via `loadClientAndResolveTask`) | Yes |
| `project contributor commitment` | Yes (via `loadClientAndResolveTask`) | Yes |

All four use `loadClientAndResolveTask` which calls `resolveTaskHash`.

### Implementation

**File: `cmd/andamio/helpers.go`**

Add `resolveTaskHashFromFlags` (parallel to `resolveSltHashFromFlags`):

```go
func resolveTaskHashFromFlags(cmd *cobra.Command, c *client.Client, projectID string) (string, int, error) {
    taskHash, _ := cmd.Flags().GetString("task-hash")
    taskIndexStr, _ := cmd.Flags().GetString("task-index")

    if taskHash != "" && taskIndexStr != "" {
        return "", 0, fmt.Errorf("--task-hash and --task-index are mutually exclusive")
    }
    if taskHash == "" && taskIndexStr == "" {
        return "", 0, fmt.Errorf("either --task-index or --task-hash is required\n\n...")
    }

    if taskHash != "" {
        return taskHash, -1, nil  // hash provided directly, no index
    }

    taskIndex, err := strconv.Atoi(taskIndexStr)
    ...
    hash, err := resolveTaskHash(c, projectID, taskIndex)
    return hash, taskIndex, err
}
```

**File: `cmd/andamio/project_contributor.go`**

- Add `--task-hash` flag to all four commands
- Remove `MarkFlagRequired("task-index")`
- Update `loadClientAndResolveTask` to use `resolveTaskHashFromFlags`

## Acceptance Criteria

- [x] `--task-hash` flag on commit, update, delete, commitment commands
- [x] `--task-hash` and `--task-index` are mutually exclusive
- [x] At least one required (clear error with discovery hint)
- [x] Chain-only tasks work with `--task-hash`
- [x] Existing `--task-index` flow unchanged
- [x] Progress messages show hash when index not available

## Sources

- GitHub Issue #45
- `cmd/andamio/helpers.go:252` — `resolveTaskHash()`
- `cmd/andamio/helpers.go:284` — `resolveSltHashFromFlags()` (pattern to mirror)
- `cmd/andamio/project_contributor.go:115` — `loadClientAndResolveTask()`
