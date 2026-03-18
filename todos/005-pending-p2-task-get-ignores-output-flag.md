---
status: pending
priority: p2
issue_id: "005"
tags: [code-review, bug, output-format, task-commands]
dependencies: []
---

# `project task get` Ignores `--output` Flag

## Problem Statement

`runTaskGet` unconditionally calls `output.PrintJSON(item)` regardless of the `--output` flag. When the format is `text` (the default), the user always gets raw JSON. This is inconsistent with every other command in the file: `list` renders a formatted table in text mode, and all other commands respect the output format.

**Why it matters:** An agent piping output to a text parser gets unexpected JSON. A user running `project task get 3 --project-id abc` expects human-readable output in text mode.

## Findings

- `project_task.go:444`: `return output.PrintJSON(item)` — no format check
- `runTasksList` correctly checks `output.GetFormat() == output.FormatJSON` before choosing between table and JSON rendering
- All other commands (`create`, `update`, `delete`) use the `isJSON` guard pattern

## Proposed Solutions

### Option A: Add text-mode table/detail rendering (Recommended)
Mirror the pattern from `runTasksList`: check `output.GetFormat()`, render a human-readable detail view for text mode, and fall through to JSON for json/csv/markdown modes.

Example text output:
```
Index:      3
Title:      Build API endpoint
Status:     DRAFT
Lovelace:   5000000
Expiration: 2026-04-01T00:00:00Z
```

**Pros:** Consistent UX with `list`, correct behavior for all output formats
**Effort:** Small | **Risk:** Low

### Option B: Document that `get` always returns JSON
Add a comment and update help text to say `get` always outputs JSON regardless of `--output`. This is technically valid for a detail command.

**Pros:** No code change needed
**Cons:** Inconsistent with rest of CLI, breaks `--output csv/markdown` expectations
**Effort:** Minimal | **Risk:** None

## Recommended Action

Option A — implement text-mode detail rendering. A simple key-value print is sufficient.

## Technical Details

- **File:** `cmd/andamio/project_task.go` line 444
- **Fix:** Add `if output.GetFormat() == output.FormatJSON { return output.PrintJSON(item) }` guard, then render text view

## Acceptance Criteria

- [ ] `project task get <index> --project-id <id>` renders human-readable text by default
- [ ] `project task get <index> --project-id <id> --output json` renders JSON
- [ ] `project task get <index> --project-id <id> --output csv` renders CSV (or falls back gracefully)

## Work Log

- 2026-03-18: Identified via agent-native and simplicity review of feat/project-task-commands branch
