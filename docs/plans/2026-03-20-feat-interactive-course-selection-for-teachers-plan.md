---
title: "feat: interactive course selection for teachers"
type: feat
status: completed
date: 2026-03-20
issue: 6
---

# feat: Interactive Course Selection for Teachers

## Overview

GitHub issue #6 requests an interactive course picker when course-id is omitted from commands like `course export` and `course import`. This directly conflicts with the CLI's composability rules (codified in CLAUDE.md and enforced since the 2026-03-18 picker-removal refactor). This plan proposes a composable alternative that solves the real UX problem -- teachers shouldn't need to memorize opaque IDs -- without breaking pipes, CI, or agent workflows.

## Problem Statement / Motivation

Teachers using `course export` and `course import` must know their course ID upfront. The current discovery workflow requires two separate commands:

```bash
# Step 1: find the course ID
andamio teacher courses --output json | jq -r '.data[] | "\(.course_id)  \(.content.title)"'

# Step 2: use it
andamio course export abc123 module-1
```

The issue asks for an interactive picker when course-id is omitted. However, the CLI has an explicit architectural rule against interactive stdin pickers:

> "No interactive pickers. If a required argument is omitted, return an error that tells the user how to discover valid values." -- CLAUDE.md, Composability Rules

This rule exists because interactive pickers were already implemented and removed (see `docs/solutions/architecture/non-interactive-cli-stdin-picker-removal.md`). They blocked CI pipelines, bash scripts, agent tool calls, and `$(...)` subshells.

## Why Not an Interactive Picker

The issue's proposed solution (`bufio.Scanner` + numbered list on stdin) was already built for `project task` commands and reverted on 2026-03-18. The problems:

1. **Hangs in pipes:** `andamio course export | jq .` blocks forever waiting for stdin
2. **Breaks CI/CD:** no TTY available, exits with "no input received"
3. **Breaks agent workflows:** Claude Code and other agents run commands non-interactively
4. **TTY detection is fragile:** `isatty()` checks add complexity and create two code paths that diverge over time

The team decision is firm: composability over convenience. The right answer is to make ID discovery effortless, not to add interactive fallbacks.

## Proposed Solution

Three changes that solve the UX problem composably:

### 1. Add `--course` flag as a name-based alternative to course-id

Allow teachers to specify a course by title substring instead of opaque ID. The CLI resolves it to a course-id via the teacher courses endpoint.

```bash
# Instead of memorizing abc123:
andamio course export --course "Cardano" module-1

# Exact match or unique substring match:
andamio course export --course "Introduction to Cardano" module-1

# Ambiguous match returns error with candidates:
# Error: "Cardano" matches multiple courses:
#   abc123  Introduction to Cardano
#   def456  Advanced Cardano Development
# Use a more specific name or pass the course-id directly.
```

This works in scripts, pipes, CI, and agents -- no stdin required.

**Implementation:**

Add a shared `resolveCourseID()` function (similar to `resolveProject()` pattern):

```go
func resolveCourseID(c *client.Client, args []string, cmd *cobra.Command) (string, error) {
    // If positional arg provided, use it directly
    if len(args) >= 1 && args[0] != "" {
        return args[0], nil
    }

    // Check --course flag
    courseName, _ := cmd.Flags().GetString("course")
    if courseName == "" {
        return "", fmt.Errorf(
            "course-id required\n\nList your courses with:\n  andamio teacher courses\n  andamio teacher courses --output json",
        )
    }

    // Fetch teacher courses and match by title substring
    courses, err := fetchTeacherCourses(c)
    if err != nil {
        return "", fmt.Errorf("failed to fetch courses: %w", err)
    }

    var matches []teacherCourse
    needle := strings.ToLower(courseName)
    for _, course := range courses {
        if strings.Contains(strings.ToLower(course.Title), needle) {
            matches = append(matches, course)
        }
    }

    switch len(matches) {
    case 0:
        return "", fmt.Errorf("no course matching %q found\n\nList your courses:\n  andamio teacher courses", courseName)
    case 1:
        return matches[0].CourseID, nil
    default:
        var lines []string
        for _, m := range matches {
            lines = append(lines, fmt.Sprintf("  %s  %s", m.CourseID, m.Title))
        }
        return "", fmt.Errorf(
            "%q matches multiple courses:\n%s\n\nUse a more specific name or pass the course-id directly.",
            courseName, strings.Join(lines, "\n"),
        )
    }
}
```

### 2. Make course-id optional (positional) when `--course` is provided

Change affected commands from `cobra.ExactArgs(2)` to accept either:
- `andamio course export <course-id> <module-code>` (current behavior, unchanged)
- `andamio course export <module-code> --course "Name"` (new)

For `course import` and `course import-all`, the `--course-id` flag already exists. Rename to `--course` for consistency, keep `--course-id` as a hidden alias.

### 3. Improve the error message when course-id is missing

For commands that still require a course-id positional arg without `--course`, the error message should guide discovery:

```
Error: course-id required

List your courses with:
  andamio teacher courses
  andamio teacher courses --output json | jq -r '.data[] | "\(.course_id)  \(.content.title)"'
```

## Affected Commands

| Command | Current Args | After |
|---------|-------------|-------|
| `course export` | `<course-id> <module-code>` | `<course-id> <module-code>` or `<module-code> --course "Name"` |
| `course import` | `<path> --course-id <id>` | `<path> --course-id <id>` or `<path> --course "Name"` |
| `course import-all` | `<dir> --course-id <id>` | `<dir> --course-id <id>` or `<dir> --course "Name"` |
| `course modules` | `<course-id>` | `<course-id>` or `--course "Name"` |
| `course slts` | `<course-id> <module-code>` | `<course-id> <module-code>` or `<module-code> --course "Name"` |

## Composable Workflow (After)

```bash
#!/usr/bin/env bash
set -euo pipefail

# Option A: name-based (human-friendly)
andamio course export --course "Intro to Cardano" module-1
andamio course import ./compiled/intro-to-cardano/module-1 --course "Intro to Cardano"

# Option B: ID-based (script-friendly, unchanged)
COURSE_ID=$(andamio teacher courses --output json | jq -r '.data[0].course_id')
andamio course export "$COURSE_ID" module-1 --output json | jq .

# Option C: mixed (agent-friendly)
andamio course modules --course "Cardano" --output json | jq -r '.data[].content.course_module_code'
```

All three work without a TTY. No stdin reads. No interactive prompts.

## Implementation Plan

### Step 1: Add `fetchTeacherCourses()` helper and `resolveCourseID()` (~30 min)

**File:** `cmd/andamio/course.go`

- Add `teacherCourse` struct with `CourseID` and `Title` fields
- Add `fetchTeacherCourses(c *client.Client)` that calls `POST /api/v2/course/teacher/courses/list`
- Add `resolveCourseID(c *client.Client, args []string, cmd *cobra.Command) (string, error)` with substring matching

### Step 2: Wire `--course` flag into export command (~20 min)

**File:** `cmd/andamio/course_export.go`

- Add `--course` string flag
- Change `Args` from `cobra.ExactArgs(2)` to custom validator that accepts 1 or 2 args
- When 1 arg + `--course`: arg is module-code, resolve course-id from flag
- When 2 args: current behavior (course-id, module-code)
- When 1 arg without `--course`: error with discovery guidance

### Step 3: Wire `--course` flag into import commands (~20 min)

**Files:** `cmd/andamio/course_import.go`, `cmd/andamio/course_import_all.go`

- Add `--course` string flag alongside existing `--course-id`
- In `RunE`, check `--course` first, then `--course-id` (both resolve to the same course_id variable)
- Keep `--course-id` working (backwards compatible), add note in `--help` that `--course` is preferred

### Step 4: Wire into read-only course commands (~15 min)

**File:** `cmd/andamio/course.go`

- `course modules`: accept `--course` flag as alternative to positional arg
- `course slts`: accept `--course` flag as alternative to first positional arg

### Step 5: Update issue #6 (~5 min)

- Comment on issue explaining the composable approach
- Reference the non-interactive architecture decision
- Close with "resolved differently than proposed"

## Acceptance Criteria

### From Issue #6 (reinterpreted composably)

- [ ] Teachers can use a course name instead of looking up and typing course IDs
- [ ] Display course title and ID when there are ambiguous matches
- [ ] Works for `course export`, `course import`, and other course commands
- [ ] Falls back to **error message** (not interactive picker) when no course specified

### Composability (non-negotiable)

- [ ] No `bufio.Scanner`, `os.Stdin`, or `fmt.Scan` in any command handler
- [ ] All commands work when piped: `andamio course export --course "X" mod-1 | wc -l` does not hang
- [ ] `--output json` produces clean JSON on stdout with no mixed text
- [ ] Progress messages go to stderr via `fmt.Fprintf(os.Stderr, ...)`

### Backwards Compatibility

- [ ] `andamio course export <course-id> <module-code>` still works (positional args unchanged)
- [ ] `andamio course import <path> --course-id <id>` still works
- [ ] Existing scripts are not broken

## Files Changed

| File | Change |
|------|--------|
| `cmd/andamio/course.go` | Add `fetchTeacherCourses()`, `resolveCourseID()`, `--course` flag on modules/slts |
| `cmd/andamio/course_export.go` | Add `--course` flag, flexible arg count |
| `cmd/andamio/course_import.go` | Add `--course` flag alongside `--course-id` |
| `cmd/andamio/course_import_all.go` | Add `--course` flag alongside `--course-id` |

## Dependencies & Risks

- **Risk:** Substring matching could be surprising if a teacher has many similarly-named courses. Mitigation: require unique match, list all candidates on ambiguity.
- **Risk:** Extra API call to resolve `--course` adds latency (~200ms). Acceptable for a convenience flag; scripts that care about speed will use course-id directly.
- **No new dependencies.** Uses existing `client.Post()` and string matching.

## Related

- `docs/solutions/architecture/non-interactive-cli-stdin-picker-removal.md` -- the architectural decision this plan preserves
- `docs/plans/2026-03-18-refactor-cli-composability-remove-interactive-pickers-plan.md` -- the refactor that removed the last interactive picker
- `docs/brainstorms/2026-03-18-composability-fixes-brainstorm.md` -- composability audit context
