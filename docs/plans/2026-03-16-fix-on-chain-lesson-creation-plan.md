---
title: "fix: ON_CHAIN upsert skips new lessons for empty SLT slots"
type: fix
status: active
date: 2026-03-16
origin: docs/brainstorms/2026-03-16-course-module-import-export-brainstorm.md
---

# fix: ON_CHAIN upsert skips new lessons for empty SLT slots

## Overview

When importing a module with ON_CHAIN status, the CLI finds all lesson files and sends them to the API, but new lessons for previously-empty SLT slots are not created. The API responds with `lessons_updated: 2` (existing ones) but silently ignores the third lesson that has no existing entry.

GitHub Issue: [#8](https://github.com/Andamio-Platform/andamio-cli/issues/8)

## Problem Statement

1. A module has 3 SLTs but only 2 lessons were initially created (SLT 3 had no lesson)
2. User creates `lesson-3.md` and runs `andamio course import`
3. CLI correctly reads all 3 lesson files and sends them in the API payload
4. API updates the 2 existing lessons but does not create a new lesson for SLT 3
5. Studio still shows "+ Add Lesson" for SLT 3

**The CLI code is NOT the bug.** Research confirms:
- `readCompiledModule` reads all `lesson-*.md` files by glob (line 346)
- `updateModuleContent` builds a payload entry for every lesson (lines 991-1017)
- `sltsLocked` only gates SLT text, NOT lessons (lines 1031-1040)
- The lessons array is sent unconditionally when files exist (line 1027)

The issue is that the API endpoint `/v2/course/teacher/course-module/update` appears to only **update** existing lessons but not **create** new ones for ON_CHAIN modules.

## Proposed Solution

Two-pronged approach: diagnose the API behavior, then implement the appropriate CLI fix.

### Phase 1: Diagnose API behavior

- [x] Add `--debug` or `--dry-run` flag to import that dumps the JSON payload sent to the API (without actually sending)
- [ ] Manually send the payload via `curl` to confirm the API silently ignores new lessons for ON_CHAIN modules
- [x] Check if there's a separate lesson creation endpoint (e.g., `POST /v2/course/teacher/lesson/create`) that works for ON_CHAIN modules
- [x] Check the OpenAPI spec: `./andamio spec paths --filter lesson`

### Phase 2: Implement fix (based on diagnosis)

**If the API supports creation via a different endpoint:**

- [ ] After the module update call, check which lessons from `data.Lessons` have no matching entry in `existing.Lessons`
- [ ] For each new lesson, call the separate creation endpoint
- [ ] Report created vs updated counts separately in the output

**If the API does NOT support lesson creation for ON_CHAIN modules:**

- [x] Detect the condition: lesson file exists for an SLT index that has no existing lesson, AND module is ON_CHAIN
- [x] Surface a clear warning:
  ```
  Warning: lesson-3.md targets SLT 3 which has no existing lesson.
  ON_CHAIN modules require lesson creation via Studio first.
  Skipping lesson-3.md (create the lesson in Studio, then re-import to update it)
  ```
- [x] In JSON output, include `skipped_lessons` array in `ImportResult`
- [ ] Do NOT send the new lesson in the payload (to avoid silent failure)

## Technical Considerations

### API contract (from CLAUDE.md)
- "array items (lessons, slts) replace the full entity"
- "Omitted top-level fields = unchanged"
- The lessons array is a full replacement — the API should upsert by `slt_index`

### Existing metadata preservation
`updateModuleContent` already handles the case where `existing.Lessons[lesson.Index]` doesn't exist — the `ok` check at line 1008 is `false`, so metadata merging is skipped for new lessons. This is correct.

### Related learnings
- `docs/solutions/integration-issues/cli-course-import-app-parity-and-payload-alignment.md` — documents SLT_LOCKED behavior and states "Lessons can always be updated" (but says nothing about creation)
- `docs/solutions/logic-errors/export-import-round-trip-title-preservation.md` — documents omit vs empty array semantics

## Acceptance Criteria

- [ ] New lessons for empty SLT slots are either created successfully OR a clear warning is shown
- [ ] JSON output includes information about skipped/created lessons
- [ ] Existing lesson update behavior is unchanged
- [ ] DRAFT module import continues to work as before (no regression)
- [ ] ON_CHAIN module with all-existing lessons works as before

## MVP

### `cmd/andamio/course_import.go` — detect and report new vs existing lessons

```go
// In updateModuleContent, after building the lessons array:
var newLessons []int
var existingLessons []int
for _, lesson := range data.Lessons {
    if _, exists := existing.Lessons[lesson.Index]; exists {
        existingLessons = append(existingLessons, lesson.Index)
    } else {
        newLessons = append(newLessons, lesson.Index)
    }
}

if len(newLessons) > 0 && sltsLocked {
    if !isJSON {
        fmt.Printf("Warning: %d lesson(s) target empty SLT slots on an %s module\n",
            len(newLessons), existing.Status)
        for _, idx := range newLessons {
            fmt.Printf("  lesson-%d.md → SLT %d (no existing lesson)\n", idx, idx)
        }
    }
}
```

## Sources

- **GitHub Issue:** [#8](https://github.com/Andamio-Platform/andamio-cli/issues/8)
- **Origin brainstorm:** [docs/brainstorms/2026-03-16-course-module-import-export-brainstorm.md](../brainstorms/2026-03-16-course-module-import-export-brainstorm.md)
- **Related learning:** [CLI Course Import: App Parity](../solutions/integration-issues/cli-course-import-app-parity-and-payload-alignment.md) — SLT_LOCKED behavior
- **Related learning:** [Round-Trip Title Preservation](../solutions/logic-errors/export-import-round-trip-title-preservation.md) — API omit vs empty array semantics
- **Key code:** `cmd/andamio/course_import.go:989-1092` (updateModuleContent)
- **Key code:** `cmd/andamio/course_import.go:926-987` (fetchExistingModule)
