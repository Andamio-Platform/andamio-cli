---
status: pending
priority: p3
issue_id: "002"
tags: [code-review, architecture, refactoring, export]
dependencies: []
---

# Unify Export Converter Wrapper Structure

## Problem Statement

The export pipeline has two structurally different ways to carry title + content through the conversion:

**Lessons** (flat map):
```go
Lesson: map[string]interface{}{"content_json": lessonContent, "title": lessonTitle}
```

**Intro/Assignment** (3-level deep synthetic wrapper):
```go
data.Introduction = map[string]interface{}{
    "data": map[string]interface{}{
        "content": map[string]interface{}{
            "content_json": contentJSON,
            "title":        introTitle,
        },
    },
}
```

The deep nesting exists solely because `convertContentToMarkdown` was written to navigate an API response shape (`resp["data"]["content"]["content_json"]`). The export code now fabricates this structure artificially. Meanwhile, the import side uses clean typed structs (`LessonImport`, `ContentSection`).

This was flagged by architecture, patterns, and simplicity reviewers as a pre-existing design smell that the title threading makes slightly more visible.

## Findings

- **Source**: Architecture Strategist, Pattern Recognition, Code Simplicity agents
- **Location**: `cmd/andamio/course_export.go` lines 163-172 (structs), 290-318 (wrapper construction), 490-540 (converters)
- **Evidence**: Two completely different conventions for the same operation; export uses untyped maps while import uses typed structs
- **Pre-existing**: This is not introduced by the current diff

## Proposed Solutions

### Option A: Unify converters to accept (contentJSON, title) directly
Replace both `convertLessonToMarkdown` and `convertContentToMarkdown` with a single function:

```go
func contentToMarkdown(contentJSON map[string]interface{}, title string) (string, []string) {
    md, urls := tiptapToMarkdown(contentJSON)
    if title != "" {
        md = "# " + sanitizeTitle(title) + "\n\n" + md
    }
    return md, urls
}
```

Eliminate the synthetic wrapper structure entirely.

- **Pros**: Removes ~20 lines, eliminates artificial nesting, single code path
- **Cons**: Changes function signatures (minor refactor)
- **Effort**: Small
- **Risk**: Low

### Option B: Add typed structs to export side
Introduce `ExportLesson` and `ExportContent` structs mirroring the import side.

- **Pros**: Full type safety, compiler catches key typos
- **Cons**: Larger refactor, touches more code, may need to update test fixtures
- **Effort**: Medium
- **Risk**: Low-Medium

## Recommended Action

## Technical Details

**Affected files:**
- `cmd/andamio/course_export.go`

## Acceptance Criteria

- [ ] Single conversion function for title + content -> markdown
- [ ] No synthetic wrapper structure for intro/assignment
- [ ] All existing tests pass
- [ ] Export output is identical before and after refactor

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-03-16 | Identified via architecture/pattern review | Wrapper structures that simulate API shapes are a code smell when the data originates locally |

## Resources

- Architecture Strategist review
- Pattern Recognition review
- Code Simplicity review
