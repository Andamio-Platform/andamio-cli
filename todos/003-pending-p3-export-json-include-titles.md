---
status: pending
priority: p3
issue_id: "003"
tags: [code-review, agent-native, export, json-output]
dependencies: []
---

# Export JSON Output Should Include Per-File Titles

## Problem Statement

The `ExportResult` struct returned in `--output json` mode includes `Files []string` (just filenames like `"lesson-1.md"`) but does not report the titles now embedded as H1 headings. An automation agent that exports and needs lesson titles must parse the markdown files rather than using the structured JSON output.

## Findings

- **Source**: Agent-Native Reviewer
- **Location**: `cmd/andamio/course_export.go` lines 59-70 (`ExportResult` struct)
- **Evidence**: `Files` field is `[]string` with no title metadata

## Proposed Solutions

### Option A: Add Titles map to ExportResult
```go
type ExportResult struct {
    // ... existing fields
    Titles map[string]string `json:"titles,omitempty"` // filename -> title
}
```

- **Pros**: Simple, backwards-compatible (omitempty)
- **Cons**: Flat map loses association between files
- **Effort**: Small
- **Risk**: Low

### Option B: Restructure Files as objects
```go
Files []struct {
    Name  string `json:"name"`
    Title string `json:"title,omitempty"`
    Type  string `json:"type"` // "lesson", "introduction", "assignment", "outline"
} `json:"files"`
```

- **Pros**: Richer metadata, extensible
- **Cons**: Breaking change to JSON output shape
- **Effort**: Small-Medium
- **Risk**: Medium (breaks existing JSON consumers)

## Recommended Action

## Technical Details

**Affected files:**
- `cmd/andamio/course_export.go` (ExportResult struct + writeCompiledModule)

## Acceptance Criteria

- [ ] `--output json` export includes title for each lesson/intro/assignment file
- [ ] Existing non-JSON output unchanged

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-03-16 | Identified via agent-native review | Structured output should surface all data that would otherwise require file parsing |

## Resources

- Agent-Native Reviewer report
