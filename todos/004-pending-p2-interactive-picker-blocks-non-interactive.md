---
status: complete
priority: p2
issue_id: "004"
tags: [code-review, agent-native, ux, task-commands]
dependencies: []
---

# Interactive Picker Blocks Non-Interactive Use

## Problem Statement

`project task list`, `create`, `export`, and `import` all call `resolveProject()` which falls into a `bufio.NewScanner(os.Stdin)` interactive picker when project-id is omitted. Any caller without a TTY — CI jobs, shell scripts piping output, agents invoking the CLI as a subprocess — hits the scanner, reads nothing, and returns `"no input received"`. This blocks 4 of 7 task commands from working non-interactively.

**Why it matters:** The existing `get`, `update`, and `delete` commands already require `--project-id` as a required flag and work fine non-interactively. The inconsistency means agents must know two different conventions for the same parameter, and half the commands are blocked in automation contexts.

## Findings

- `project_task.go:184–221` — `resolveProject()` uses `bufio.NewScanner(os.Stdin)` with no TTY check
- Affected commands: `list`, `create`, `export`, `import` (all use optional positional project-id arg)
- `get`, `update`, `delete` already use `--project-id` as a required flag — correct pattern
- Learnings researcher confirms: the interactive picker pattern was noted as a design issue in prior CLI work

## Proposed Solutions

### Option A: Make project-id required positional arg on all four commands (Recommended)
Change `cobra.MaximumNArgs(1)` to `cobra.ExactArgs(1)` for `list`, `create`, `export`, `import`. Remove the interactive picker from `resolveProject`. The existing `project list` command is the agent-accessible way to discover IDs.

**Pros:** Simple, consistent with `get`/`update`/`delete` pattern, no TTY dependency
**Cons:** Breaking change for users who relied on the interactive picker
**Effort:** Small | **Risk:** Low

### Option B: Add TTY detection before picker
Use `golang.org/x/term` or check `os.Stdin` `stat.Mode()&os.ModeCharDevice` to detect non-TTY contexts. Return a clear error when project-id is omitted in non-interactive context: `"project-id is required in non-interactive mode. Run 'andamio project list' to find IDs."`.

**Pros:** Preserves interactive UX, explicit error for automation
**Cons:** Adds a dependency or OS-specific code, error is still not a clean failure
**Effort:** Small | **Risk:** Low

### Option C: Add `--project-id` flag as an alternative to positional arg
Keep positional arg + picker, but add `--project-id` flag that all four commands accept (bypasses picker when set). Agents use the flag; humans use the interactive picker.

**Pros:** Backwards compatible, unambiguous for agents
**Cons:** Two ways to supply the same thing, more surface area
**Effort:** Small | **Risk:** Low

## Recommended Action

Option A — make project-id required. The interactive picker can be documented as `andamio project list` (already exists) and noted in help text.

## Technical Details

- **Affected files:** `cmd/andamio/project_task.go`
- **Affected commands:** `project task list`, `project task create`, `project task export`, `project task import`
- **Related:** `resolveProject()` at lines 184–221

## Acceptance Criteria

- [ ] All task commands work when piped (`echo "" | andamio project task list abc123`)
- [ ] All task commands work in CI without a TTY
- [ ] Help text for each command shows how to discover project IDs (`andamio project list`)
- [ ] `project task list --help` shows project-id as required

## Work Log

- 2026-03-18: Identified via agent-native review of feat/project-task-commands branch
