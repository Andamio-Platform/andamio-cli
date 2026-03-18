---
title: "refactor: Remove Interactive Pickers and Maximize CLI Composability"
type: refactor
status: completed
date: 2026-03-18
---

# refactor: Remove Interactive Pickers and Maximize CLI Composability

## Overview

The andamio-cli currently has an interactive stdin picker in `resolveProject()` that blocks 4 of 7 task commands from working in non-interactive environments (CI, bash scripts, agent pipelines). The team has identified composability as a key deliverable — the CLI must work seamlessly in bash scripts that combine `andamio`, `gh`, and `jq` without any human interaction.

This refactor removes all interactive patterns, enforces explicit IDs, routes progress messages to stderr, and establishes composability-first conventions for all future commands.

**The North Star:** Every `andamio` command must work correctly when run as part of a pipeline:

```bash
andamio project task list $PROJECT_ID --output json | jq '.data[].content.title'
```

---

## Problem Statement

### The Interactive Picker

`resolveProject()` in `cmd/andamio/project_task.go:178` invokes a `bufio.NewScanner(os.Stdin)` picker when project-id is omitted. This blocks four commands:

- `project task list [project-id]`
- `project task create [project-id]`
- `project task export [project-id]`
- `project task import [project-id]`

Any caller without a TTY — CI job, shell script, bash pipe, agent subprocess — hits the scanner, reads nothing, and gets `"no input received"`. **There is no workaround.**

### The Inconsistency

The four affected commands accept project-id as an *optional* positional arg. Three other task commands (`get`, `update`, `delete`) already require `--project-id` as a mandatory flag and work fine non-interactively. This creates two calling conventions for the same parameter.

### Stdout Pollution

Progress messages ("Creating task:", "Exporting 5 tasks to...") currently go to `os.Stdout` in text mode and only get suppressed with `--output json`. This means:

```bash
# This captures progress messages along with the task list — broken
andamio project task list $ID | grep "my-task"

# This works but forces JSON parsing — unnecessary friction
andamio project task list $ID --output json | jq '.data[].content.title'
```

Unix convention: **stdout is for data, stderr is for human-readable status**. Progress messages should go to stderr unconditionally.

---

## Proposed Solution

Three targeted changes:

### 1. Remove the interactive picker from `resolveProject()`

When project-id is omitted, return a clear error instead of invoking stdin:

```go
// Before
if len(args) == 0 {
    // interactive picker ... bufio.Scanner ... fmt.Print("Select project number: ")
}

// After
if len(args) == 0 || args[0] == "" {
    return nil, nil, fmt.Errorf(
        "project-id required\n\nList your projects with:\n  andamio project list --output json",
    )
}
```

The error message teaches the user the correct two-step workflow, matching how `gh` and `gcloud` work.

### 2. Make project-id a required positional arg on the four affected commands

Change `cobra.MaximumNArgs(1)` → `cobra.ExactArgs(1)` for `list`, `create`, `export`, `import`. Update `Use:` strings to show `<project-id>` as required:

```
Use: "list <project-id>"
Use: "create <project-id>"
Use: "export <project-id>"
Use: "import <project-id>"
```

This makes the requirement explicit in `--help` output and prevents Cobra from even calling the command without the arg.

### 3. Route progress messages to stderr

Replace all `fmt.Printf(...)` / `fmt.Println(...)` progress messages with `fmt.Fprintf(os.Stderr, ...)`. Keep the `!isJSON` gate (stderr output is still suppressed in JSON mode for clean pipelines).

```go
// Before
if !isJSON {
    fmt.Printf("Creating task: %s\n", title)
}

// After
if !isJSON {
    fmt.Fprintf(os.Stderr, "Creating task: %s\n", title)
}
```

Affected files: `project_task.go`, `project_task_export.go`, `project_task_import.go`.

---

## The Composable Workflow

After this change, the intended bash script pattern works:

```bash
#!/bin/bash
PROJECT_ID=$(andamio project list --output json | jq -r '.data[] | select(.content.title == "My Project") | .project_id')

# Create tasks from GitHub issues
gh issue list --repo org/repo --json number,title --jq '.[]' | while IFS= read -r issue; do
  NUMBER=$(echo "$issue" | jq -r '.number')
  TITLE=$(echo "$issue" | jq -r '.title')
  andamio project task create "$PROJECT_ID" \
    --title "$TITLE" \
    --github-issue "org/repo#$NUMBER" \
    --lovelace 5000000 \
    --expiration 2026-06-01
done

# Export to Markdown, edit, reimport
andamio project task export "$PROJECT_ID"
# ... edit tasks/ files ...
andamio project task import "$PROJECT_ID" --dry-run
andamio project task import "$PROJECT_ID"
```

Progress messages (now on stderr) don't interfere with captured output. `--output json` unlocks structured parsing with `jq`.

---

## Technical Considerations

### Backwards Compatibility

This is a **breaking change** for users who relied on the interactive picker (omitting project-id triggers a prompt). However:
- The feature is brand new (just merged on this branch)
- No public release has shipped with the picker
- The error message guides users to the correct workflow
- The fix lands before "end of day" use

### `resolveProject()` simplification

After removing the picker, `resolveProject()` becomes much simpler — it always needs args[0]:

```go
func resolveProject(c *client.Client, args []string) (*managerProject, []managerProject, error) {
    if len(args) == 0 || args[0] == "" {
        return nil, nil, fmt.Errorf("project-id required\n\nList your projects:\n  andamio project list --output json")
    }
    projects, err := fetchManagerProjects(c)
    if err != nil {
        return nil, nil, err
    }
    for i := range projects {
        if projects[i].ProjectID == args[0] {
            return &projects[i], projects, nil
        }
    }
    return nil, nil, fmt.Errorf("project %s not found in your managed projects", args[0])
}
```

The `bufio` and `os` imports in `project_task.go` (currently only there for the scanner) can be removed.

### stderr vs stdout: Clean pipe behavior

After moving progress to stderr:

```bash
# Data only on stdout — works cleanly
andamio project task list $ID | grep something
andamio project task export $ID > export.log  # captures only file list

# Progress visible on terminal but doesn't pollute stdout
andamio project task import $ID
# stderr: "Importing 3 task files from tasks/my-project/"
# stderr: "  001-build-api.md: CREATED 'Build API'"
# stdout: (nothing in text mode)
```

### Scope boundary

This plan covers `project_task.go`, `project_task_export.go`, `project_task_import.go`. The `course_export.go` and `course_import.go` files already use `!isJSON` gates consistently — they can be migrated to stderr in a separate pass after the pattern is established.

---

## Acceptance Criteria

### Functional

- [x] `andamio project task list` (no args) returns an error with a helpful message pointing to `andamio project list`
- [x] `andamio project task list <project-id>` works without a TTY
- [x] `andamio project task create <project-id> --title ... --lovelace ... --expiration ...` works without a TTY
- [x] `andamio project task export <project-id>` works without a TTY
- [x] `andamio project task import <project-id>` works without a TTY
- [x] `--help` for each command shows project-id as required positional arg

### Composability / Scripting

- [ ] `andamio project task list $ID --output json | jq '.data[].content.title'` captures task titles cleanly
- [ ] `andamio project task create $ID --title "..." ... 2>/dev/null` (suppress stderr) exits 0 with no stdout output in text mode
- [ ] `andamio project task export $ID 2>/dev/null` writes files silently
- [ ] The reference bash script (GitHub issues → tasks) runs end-to-end without a TTY

### Progress to stderr

- [x] Running `andamio project task create $ID ...` in text mode shows progress on stderr, nothing on stdout
- [x] Running `andamio project task create $ID ... --output json` shows no progress on stderr, JSON on stdout
- [x] `andamio project task import $ID 2>/dev/null` produces no output (text mode, stderr suppressed)

### Code quality

- [x] `bufio` import removed from `project_task.go`
- [x] `resolveProject()` no longer references `os.Stdin`
- [x] All `fmt.Printf` / `fmt.Println` progress messages in task files write to `os.Stderr`
- [x] `cobra.ExactArgs(1)` on `list`, `create`, `export`, `import`

---

## Implementation Plan

### Phase 1: Remove picker, require project-id (`project_task.go`) — ~20 min

**File:** `cmd/andamio/project_task.go`

1. Change `Use:` strings on `projectTaskListCmd`, `projectTaskCreateCmd` to show `<project-id>` as required
2. Change `Args: cobra.MaximumNArgs(1)` → `Args: cobra.ExactArgs(1)` on both commands
3. Rewrite `resolveProject()`: remove interactive block (lines 197–220), replace with error on empty args
4. Remove `bufio` import (no longer needed)
5. Move progress `fmt.Printf` → `fmt.Fprintf(os.Stderr, ...)` in `runTaskCreate`, `runTaskUpdate`, `runTaskDelete`

### Phase 2: Update export and import (`project_task_export.go`, `project_task_import.go`) — ~15 min

**File:** `cmd/andamio/project_task_export.go`

1. Change `Use: "export [project-id]"` → `Use: "export <project-id>"`
2. Change `Args: cobra.MaximumNArgs(1)` → `Args: cobra.ExactArgs(1)`
3. Move progress `fmt.Printf` / `fmt.Println` → `fmt.Fprintf(os.Stderr, ...)`

**File:** `cmd/andamio/project_task_import.go`

1. Change `Use: "import [project-id]"` → `Use: "import <project-id>"`
2. Change `Args: cobra.MaximumNArgs(1)` → `Args: cobra.ExactArgs(1)`
3. Move progress `fmt.Printf` / `fmt.Println` → `fmt.Fprintf(os.Stderr, ...)`

### Phase 3: Update CLAUDE.md conventions — ~10 min

Add a **Composability Rules** section to `CLAUDE.md`:

```markdown
## Composability Rules

All commands must work without a TTY. Never read from stdin in command handlers.

1. **No interactive pickers.** If a required argument is omitted, return an error that tells
   the user how to discover valid values (e.g., "Run 'andamio project list --output json'").
2. **Progress to stderr.** Use `fmt.Fprintf(os.Stderr, ...)` for all progress/status messages.
   Gate with `if !isJSON` to suppress in JSON mode.
3. **Data to stdout.** Only structured output (tables, JSON, CSV) goes to stdout.
4. **Required args are required.** Use `cobra.ExactArgs(N)` and `MarkFlagRequired`. Never
   use `MaximumNArgs` for args that the command cannot function without.
5. **`--output json` is the scripting surface.** All list/get commands must support it and
   return stable, documented JSON schemas.
```

### Phase 4: Update todo #004

Mark `todos/004-pending-p2-interactive-picker-blocks-non-interactive.md` as complete.

---

## Files Changed

| File | Change |
|------|--------|
| `cmd/andamio/project_task.go` | Remove picker, `ExactArgs(1)`, progress → stderr |
| `cmd/andamio/project_task_export.go` | `ExactArgs(1)`, progress → stderr |
| `cmd/andamio/project_task_import.go` | `ExactArgs(1)`, progress → stderr |
| `CLAUDE.md` | Add Composability Rules section |
| `todos/004-pending-p2-...md` | Mark complete |

**Not in scope:** `course_export.go`, `course_import.go` (already use `!isJSON` gates; no interactive elements; migrate to stderr in a future pass).

---

## Dependencies & Risks

- **Risk:** `os` import in `project_task.go` — currently only imported for `os.Stdin`. After this change, `os.Stderr` is used instead, so the import stays (different purpose).
- **Risk:** None of this breaks any existing tests — the test files in the project cover export/import conversion functions, not command handlers.
- **No new dependencies.** `fmt.Fprintf(os.Stderr, ...)` is stdlib.

---

## Sources & References

- Interactive picker location: `cmd/andamio/project_task.go:178–221`
- Affected commands: `list` (line 42), `create` (line 54), `export` (`project_task_export.go:8`), `import` (`project_task_import.go:11`)
- Existing composable pattern: `project task get/update/delete` — already correct, no changes needed
- Code review finding: `todos/004-pending-p2-interactive-picker-blocks-non-interactive.md`
- Prior architecture doc: `docs/solutions/architecture/command-structure-refactoring.md`
