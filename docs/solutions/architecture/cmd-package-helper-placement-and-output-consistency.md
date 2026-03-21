---
title: "Fix: Extract shared helpers to helpers.go and enforce stdout consistency"
date: 2026-03-21
category: architecture
tags:
  - go
  - cli
  - cobra
  - refactor
  - composability
  - architecture
  - auth
  - code-review
severity:
  - P2
  - P3
component:
  - cmd/andamio/helpers.go
  - cmd/andamio/course.go
  - cmd/andamio/project_task.go
  - cmd/andamio/project_owner.go
related_prs:
  - 33
  - 15
  - 26
---

# Extract Shared Helpers to `helpers.go` and Enforce stdout Consistency

## Problem

During code review of PR #33 ("feat: add project owner commands"), three quality issues were identified:

1. **Cross-cutting helpers in domain file (P2)**: Six shared functions (`jwtAuthPreRunE`, `getJSON`, `postJSON`, `getJSONWithHint`, `truncateUTF8`, `printList`) lived in `course.go` — a domain-specific file for course commands. These helpers were imported by `manager.go`, `teacher.go`, `project_owner.go`, and `project_task.go`, creating misleading coupling.

2. **Incomplete refactor (P2)**: PR #33 extracted `jwtAuthPreRunE` from inline closures in `manager.go` and `teacher.go` but missed `project_task.go:21-35`, which still had the identical 12-line inline closure.

3. **Inconsistent stdout output (P3)**: `project owner create` printed `project_id: <id>` to stdout on success, but `update` and `register` emitted nothing to stdout in text mode — only stderr messages.

## Root Cause

1. **Helper accumulation by proximity**: `course.go` was the first command file written and contained the first helper (`getJSON`). Subsequent helpers were added there because the pattern already existed, not because they belonged to the course domain. By the time 6 helpers lived there, the file was half infrastructure.

2. **Search was not exhaustive**: The refactor searched `manager.go` and `teacher.go` (the files being actively changed) but did not grep for all copies of the inline pattern across the entire `cmd/andamio/` package.

3. **First-mover pattern not enforced**: `project owner create` set the stdout convention but there was no documented rule requiring mutating commands to match it. `update` and `register` were written later without copying the pattern.

## Solution

### 1. Created `cmd/andamio/helpers.go`

Moved all 6 cross-cutting helpers from `course.go` into a dedicated file with no domain affiliation:

```go
// helpers.go — shared helpers for all command files
package main

func jwtAuthPreRunE(cmd *cobra.Command, args []string) error { ... }
func getJSON(path string) error { ... }
func postJSON(path string) error { ... }
func getJSONWithHint(path, notFoundHint string) error { ... }
func truncateUTF8(s string, maxRunes int) string { ... }
func printList(path, emptyMsg, titleKey, idKey string, usePost bool) error { ... }
```

`course.go` now contains only course-specific commands and helpers (`fetchTeacherCourses`, `resolveCourseID`, etc.).

### 2. Replaced inline closure in `project_task.go`

Before (12-line inline closure):
```go
var projectTaskCmd = &cobra.Command{
    Use:   "task",
    Short: "Manage project tasks (manager role)",
    PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
        if err := rootCmd.PersistentPreRunE(cmd, args); err != nil {
            return err
        }
        cfg, err := config.Load()
        // ... 8 more lines ...
    },
}
```

After:
```go
var projectTaskCmd = &cobra.Command{
    Use:               "task",
    Short:             "Manage project tasks (manager role)",
    PersistentPreRunE: jwtAuthPreRunE,
}
```

All 4 role-based parent commands (`manager`, `teacher`, `owner`, `task`) now reference the single shared function.

### 3. Added stdout output to `update` and `register`

Both commands now emit `project_id` to stdout in text mode, matching `create`:

```go
fmt.Fprintf(os.Stderr, "Project updated.\n")
if id, ok := resp["project_id"].(string); ok {
    fmt.Printf("project_id: %s\n", id)
}
```

This enables consistent scripting:
```bash
andamio project owner update --project-id "$ID" --title "New" 2>/dev/null
# stdout: project_id: abc123
```

### 4. Cleaned up imports

- `course.go`: removed `errors`, `unicode/utf8` (no longer needed)
- `project_task.go`: removed `apierr` import (no longer needed for inline auth)

## Verification

- `go build ./cmd/andamio/` — passes
- `go vet ./cmd/andamio/` — passes
- `go test ./...` — all tests pass
- `grep -rn "jwtAuthPreRunE" cmd/andamio/` — returns only definition in `helpers.go` and call sites, zero inline copies

## Prevention

### Rules for CLAUDE.md

**Helper placement**: Cross-cutting helpers belong in `cmd/andamio/helpers.go`, never in domain command files. If a function is used by more than one command file, it must not be defined in a domain file.

**Refactoring completeness**: After extracting a shared function, grep for the old inline pattern across the entire package. Zero matches required before the refactor is done.

**Mutating command stdout contract**: Commands that create, update, delete, or register must print exactly one machine-readable identifier to stdout on success. All human-readable messages go to stderr.

### Grep Patterns for Review

```bash
# Find helpers in domain files called from other files
grep -rn "getJSON\|postJSON\|printList\|jwtAuthPreRunE" cmd/andamio/ | grep -v helpers.go | grep -v "_test.go"

# Find duplicate inline auth patterns
grep -rn "HasUserAuth\|cfg\.UserJWT" cmd/andamio/*.go

# Find mutating commands missing stdout output
grep -A5 "Project updated\|Project registered\|Project created\|Task created\|Task updated\|Task deleted" cmd/andamio/*.go
```

## Related References

### Solution Documents
- [command-structure-refactoring.md](command-structure-refactoring.md) — Earlier extraction of `printList`/`getJSON`/`postJSON` and `PersistentPreRunE` centralization
- [cli-composability-audit-and-fix.md](cli-composability-audit-and-fix.md) — Full audit of stdout/stderr violations with prevention checklist
- [non-interactive-cli-stdin-picker-removal.md](non-interactive-cli-stdin-picker-removal.md) — Companion doc on stdin picker removal

### Related Todos
- `018-complete-p2-jwt-auth-helper-wrong-file.md` — Tracked the `jwtAuthPreRunE` misplacement
- `019-complete-p2-project-task-inline-jwt-duplicate.md` — Tracked the missed inline copy
- `020-complete-p3-owner-commands-stdout-consistency.md` — Tracked the stdout inconsistency
- `009-pending-p3-project-slug-from-list-in-wrong-file.md` — Similar placement issue in `project_task_export.go`

### GitHub PRs
- [PR #33](https://github.com/Andamio-Platform/andamio-cli/pull/33) — Where issues were found and fixed
- [PR #15](https://github.com/Andamio-Platform/andamio-cli/pull/15) — Earlier composability gap fixes
- [PR #26](https://github.com/Andamio-Platform/andamio-cli/pull/26) — Earlier helper extraction example
