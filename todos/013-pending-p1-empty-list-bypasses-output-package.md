---
status: complete
priority: p1
issue_id: "013"
tags: [code-review, composability, json, course, output]
dependencies: []
---

# Hardcoded `{"data":[]}` Bypasses Output Package — Breaks `--output csv/markdown`

## Problem Statement

`course.go` handles empty lists with a hardcoded JSON string:

```go
if output.GetFormat() == output.FormatJSON {
    fmt.Println(`{"data":[]}`)
} else {
    fmt.Fprintln(os.Stderr, emptyMsg)
}
```

This appears in two places: `printList` (line 144) and `runCourseModulesTeacher` (line 190).

The hardcoded string bypasses the `output` package entirely. This violates the invariant that `--output` controls all stdout content:

1. When `--output csv` or `--output markdown` is set, the code enters the `output.GetFormat() == output.FormatJSON` branch... but waits: actually it **doesn't** — it falls through to the `else` branch and prints to stderr. That's actually correct for csv/markdown. But the JSON branch is hand-rolled, meaning:
   - The output is single-line unindented JSON, while `output.PrintJSON` via `json.MarshalIndent` produces indented JSON
   - A script doing `andamio course list --output json` on a non-empty list gets indented JSON; the same script on an empty list gets unindented JSON — schema inconsistency detectable by `jq`
   - Any future change to the JSON envelope shape (e.g., adding metadata) will be missed by these hardcoded strings
   - It's a maintenance trap: three separate locations in the codebase have to be updated if the empty-list response shape ever changes

## Findings

- **Source**: Architecture agent (P1), simplicity agent (Medium)
- **Location**: `cmd/andamio/course.go:144`, `cmd/andamio/course.go:190`

## Proposed Solutions

### Option A: Use `output.PrintJSON` with empty slice (Recommended)

```go
if !ok || len(data) == 0 {
    if output.GetFormat() == output.FormatJSON {
        return output.PrintJSON(map[string]interface{}{"data": []interface{}{}})
    }
    fmt.Fprintln(os.Stderr, emptyMsg)
    return nil
}
```

**Pros:** Routes through the output package. Consistent indented JSON. Works correctly for csv/markdown modes (PrintJSON has format-aware handling). One-line change per occurrence.
**Cons:** None.
**Effort:** Trivial
**Risk:** None

### Option B: Use `output.PrintJSON` with typed empty slice

```go
return output.PrintJSON(map[string]interface{}{"data": []map[string]interface{}{}})
```

Matches the type of the data array exactly.
**Pros:** Type-correct.
**Cons:** Slightly more verbose.
**Effort:** Trivial
**Risk:** None

## Recommended Action

Option A. Fix all three occurrences in `course.go` (lines 144, 190, and check for any others using `grep`).

## Technical Details

- **Affected files**: `cmd/andamio/course.go:144`, `cmd/andamio/course.go:190`
- **PR**: #15 fix/composability-gaps

## Acceptance Criteria

- [ ] `andamio course list --output json` on empty result set returns indented JSON matching `{"data": []}` (with spaces, no trailing newline difference)
- [ ] No hardcoded JSON string literals in course.go
- [ ] `--output csv` and `--output markdown` on empty lists behave consistently with non-empty lists

## Work Log

- 2026-03-18: Flagged by architecture and simplicity agents during PR #15 review
