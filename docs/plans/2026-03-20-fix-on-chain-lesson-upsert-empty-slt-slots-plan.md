---
title: "fix: ON_CHAIN upsert skips new lessons for empty SLT slots"
type: fix
status: active
date: 2026-03-20
origin: https://github.com/Andamio-Platform/andamio-cli/issues/8
supersedes: docs/plans/2026-03-16-fix-on-chain-lesson-creation-plan.md
---

# fix: ON_CHAIN upsert skips new lessons for empty SLT slots

## Overview

When importing a module with ON_CHAIN status, the CLI sends all lesson files in the payload, but the API only updates lessons that already exist -- new lessons for previously-empty SLT slots are silently ignored. This plan supersedes the 2026-03-16 plan, which diagnosed the root cause and implemented partial fixes (warnings) but left the core creation gap unresolved.

GitHub Issue: [#8](https://github.com/Andamio-Platform/andamio-cli/issues/8)

## Root Cause Analysis

The bug is **not in the CLI code**. The prior plan (2026-03-16) confirmed this through code analysis:

1. `readCompiledModule` reads all `lesson-*.md` files via glob (line 456)
2. `updateModuleContent` builds a payload entry for every lesson (lines 1112-1138)
3. `sltsLocked` only gates SLT text, NOT lessons (lines 1152-1166)
4. The lessons array is sent unconditionally when files exist (line 1147-1149)

The problem is the **API endpoint** `/v2/course/teacher/course-module/update`. It uses the `lessons` array to update existing lessons (matched by `slt_index`), but does not create new lesson records for SLT slots that have no existing lesson in the database. The API response confirms: `lessons_updated: 2` with no `lessons_created` count.

### Why this happens in the ON_CHAIN flow specifically

- **DRAFT modules**: When the module is created fresh and imported for the first time, the API creates all lessons because none exist yet. Every entry is "new."
- **ON_CHAIN modules**: A second import finds existing lessons for SLTs 1 and 2 (created in the first import). The update endpoint matches by `slt_index` and updates them. SLT 3 has no existing lesson record, so the API has nothing to match against -- it silently drops the entry.

### Key constraint from solutions doc

From `docs/solutions/integration-issues/cli-course-import-app-parity-and-payload-alignment.md`, Bug 5:

> "Lessons, introduction, assignment can always be updated"

This was an assumption. Lessons can be **updated** at any time, but **creation** of a new lesson for an existing SLT may require a different API behavior or endpoint.

## Diagnosis Steps (Phase 1)

Before implementing the fix, we need to confirm the exact API behavior. These steps should be done manually:

### Step 1: Reproduce with `--dry-run`

```bash
# Module with 3 SLTs, lesson-3.md is new (no existing lesson for SLT 3)
andamio course import ./compiled/my-course/101 --course-id <id> --dry-run
```

Verify the dry-run payload includes all 3 lessons with correct `slt_index` values.

### Step 2: Test API behavior with curl

Extract the dry-run payload and send it directly:

```bash
curl -X POST https://preprod.api.andamio.io/api/v2/course/teacher/course-module/update \
  -H "Authorization: Bearer $JWT" \
  -H "Content-Type: application/json" \
  -d @payload.json
```

Check the response `changes` object. If `lessons_created: 0` and `lessons_updated: 2`, the API is confirmed to not create new lessons via the update endpoint.

### Step 3: Check for a separate lesson creation endpoint

```bash
andamio spec paths --filter lesson
```

Look for endpoints like:
- `POST /v2/course/teacher/lesson/create`
- `POST /v2/course/teacher/lesson/upsert`

### Step 4: Check the web app behavior

In Studio (the web app), click "+ Add Lesson" on SLT 3 of an ON_CHAIN module. Use browser dev tools to capture the API call. This tells us exactly what endpoint/payload the app uses to create a lesson for an existing SLT slot.

## Implementation Plan (Phase 2)

Based on the diagnosis, there are two possible paths. Implement whichever matches the API reality.

---

### Path A: API supports creation via the update endpoint (payload issue)

If the API *does* support creating lessons via `/course-module/update` but requires a specific field (e.g., a `create: true` flag or a different payload shape for new lessons), adjust the payload construction.

**Changes to `updateModuleContent` in `cmd/andamio/course_import.go`:**

1. Detect which lessons are new vs existing:
   ```go
   var newLessonIndices []int
   for _, lesson := range data.Lessons {
       if _, exists := existing.Lessons[lesson.Index]; !exists {
           newLessonIndices = append(newLessonIndices, lesson.Index)
       }
   }
   ```

2. Add any required fields to new lesson entries (whatever the diagnosis reveals).

3. Report new vs updated counts in progress output.

**Estimated scope:** ~15 lines changed in `updateModuleContent`.

---

### Path B: API requires a separate creation call (most likely)

If the update endpoint genuinely cannot create new lessons and a separate endpoint exists (e.g., Studio calls a different route), the CLI needs a two-phase approach.

**Changes to `cmd/andamio/course_import.go`:**

#### B1. Partition lessons into "update" and "create" sets

Add a function after line 1138 (end of lessons array construction in `updateModuleContent`):

```go
func partitionLessons(data *ImportData, existing *ExistingModuleData) (updates, creates []LessonImport) {
    for _, lesson := range data.Lessons {
        if _, exists := existing.Lessons[lesson.Index]; exists {
            updates = append(updates, lesson)
        } else {
            creates = append(creates, lesson)
        }
    }
    return
}
```

#### B2. Send existing lessons via update endpoint (current flow)

In `updateModuleContent`, only include existing-matched lessons in the `lessons` payload:

```go
// Line ~1112: filter to only lessons that already exist
var updateLessons []map[string]interface{}
for i, lesson := range data.Lessons {
    if _, exists := existing.Lessons[lesson.Index]; exists {
        updateLessons = append(updateLessons, lessons[i])
    }
}
if len(updateLessons) > 0 {
    payload["lessons"] = updateLessons
}
```

#### B3. Create new lessons via separate endpoint

After the update call succeeds, call the creation endpoint for each new lesson:

```go
func createNewLessons(c *client.Client, courseID, moduleCode string, creates []LessonImport, quiet bool) (int, []string) {
    var created int
    var failed []string
    for _, lesson := range creates {
        payload := map[string]interface{}{
            "course_id":          courseID,
            "course_module_code": moduleCode,
            "slt_index":          lesson.Index,
            "title":              lesson.Title,
            "content_json":       lesson.TiptapJSON,
        }
        var resp map[string]interface{}
        if err := c.Post("/api/v2/course/teacher/lesson/create", payload, &resp); err != nil {
            failed = append(failed, fmt.Sprintf("lesson-%d: %v", lesson.Index, err))
            continue
        }
        created++
        if !quiet {
            fmt.Fprintf(os.Stderr, "  Created lesson for SLT %d\n", lesson.Index)
        }
    }
    return created, failed
}
```

#### B4. Update `importModule` orchestration

In the `importModule` function (around line 250), after `updateModuleContent`:

```go
// After the main update call
resp, err := updateModuleContent(c, p.CourseID, data, existing, sltsLocked, p.DryRun)
if err != nil {
    return nil, err
}

// Create new lessons for empty SLT slots (update endpoint only handles existing)
_, newLessons := partitionLessons(data, existing)
if len(newLessons) > 0 && !p.DryRun {
    created, failures := createNewLessons(p.Client, p.CourseID, data.ModuleCode, newLessons, p.Quiet)
    changes["lessons_created_by_cli"] = created
    if len(failures) > 0 {
        changes["lesson_creation_failures"] = failures
    }
}
```

#### B5. Update dry-run reporting

In dry-run mode, report the partition:

```go
if p.DryRun && len(newLessons) > 0 {
    if !p.Quiet {
        fmt.Fprintf(os.Stderr, "Dry-run: %d lesson(s) would be created (new SLT slots):\n", len(newLessons))
        for _, l := range newLessons {
            fmt.Fprintf(os.Stderr, "  lesson-%d.md -> SLT %d\n", l.Index, l.Index)
        }
    }
}
```

**Estimated scope:** ~60 lines added, ~10 lines modified.

---

### Path C: No creation endpoint exists (fallback)

If no separate creation endpoint exists and the update endpoint truly cannot create, the CLI should fail loudly instead of silently dropping lessons.

**Changes:**

1. Detect the condition (new lessons for existing module)
2. Surface actionable warnings to stderr
3. Add `skipped_lessons` to `ImportResult` for JSON consumers
4. Add `SkippedLessons` field to `ImportResult` struct

```go
type ImportResult struct {
    // ... existing fields ...
    SkippedLessons []SkippedLesson `json:"skipped_lessons,omitempty"`
}

type SkippedLesson struct {
    Index  int    `json:"slt_index"`
    File   string `json:"file"`
    Reason string `json:"reason"`
}
```

Warning output:
```
Warning: 1 lesson(s) target empty SLT slots on an ON_CHAIN module
  lesson-3.md -> SLT 3 (no existing lesson - create in Studio first, then re-import)
```

**Estimated scope:** ~30 lines added.

---

## Changes to `ImportResult` (all paths)

Regardless of which path is taken, update the `ImportResult` struct to distinguish created vs updated:

```go
type ImportResult struct {
    // ... existing fields ...
    LessonsUpdated int             `json:"lessons_updated"`
    LessonsCreated int             `json:"lessons_created"`
    SkippedLessons []SkippedLesson `json:"skipped_lessons,omitempty"`
}
```

This gives JSON consumers explicit counts instead of relying on the API's `changes` map.

## Changes to Text Output

In `runCourseImport` (line 326+), add reporting for the new/skipped lesson distinction:

```
  Changes:
    Lessons updated:    2
    Lessons created:    1  (new SLT slots)
```

Or, in Path C:

```
  Skipped:
    lesson-3.md -> SLT 3 (create in Studio first)
```

## Test Plan

### Unit tests (`cmd/andamio/course_import_test.go`)

1. **`TestPartitionLessons`** -- given ImportData with lessons 1,2,3 and existing lessons 1,2, verify lessons 1,2 are in "updates" and lesson 3 is in "creates"
2. **`TestPartitionLessonsAllExisting`** -- all lessons have existing records, creates is empty
3. **`TestPartitionLessonsAllNew`** -- no existing lessons, all are in creates (DRAFT first import case)

### Integration tests (manual)

1. Export a module with 3 SLTs and all 3 lessons
2. Delete lesson 3 via Studio
3. Re-import -- verify lesson 3 is created (Path A/B) or warned (Path C)
4. Verify lessons 1 and 2 are updated, not duplicated
5. Run with `--dry-run` and `--output json` to verify structured output
6. Test with DRAFT module (should work as before -- no regression)
7. Test with ON_CHAIN module where all lessons already exist (update-only, no regression)

## Files Changed

| File | Change |
|------|--------|
| `cmd/andamio/course_import.go` | Add lesson partitioning, new lesson creation (or warning), update ImportResult struct, update text output |
| `cmd/andamio/course_import_test.go` | Add partition tests |
| `cmd/andamio/course_import_all.go` | Update summary output to reflect created vs updated (minor) |

## Decision Log

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Supersede prior plan | Yes | 2026-03-16 plan diagnosed but didn't fully resolve; this plan adds implementation detail for all paths |
| Phase 1 before Phase 2 | Yes | Must confirm API behavior before writing code -- wrong assumption led to the original "lessons can always be updated" claim |
| Three implementation paths | A/B/C | Cannot commit to one path until API diagnosis is done; all three are straightforward |
| Warnings to stderr | Always | Composability rule: progress/warnings to stderr, data to stdout |
| SkippedLessons in JSON | Yes | JSON consumers need to know when lessons were silently dropped |

## Sequence

1. Run Phase 1 diagnosis (manual, ~15 min)
2. Based on findings, implement one of Path A/B/C (~30-60 min)
3. Write unit tests for partition logic (~15 min)
4. Manual integration test against preprod (~15 min)
5. Update the 2026-03-16 plan status to `superseded`

## Sources

- **GitHub Issue:** [#8](https://github.com/Andamio-Platform/andamio-cli/issues/8)
- **Prior plan:** [docs/plans/2026-03-16-fix-on-chain-lesson-creation-plan.md](2026-03-16-fix-on-chain-lesson-creation-plan.md)
- **Brainstorm:** [docs/brainstorms/2026-03-16-course-module-import-export-brainstorm.md](../brainstorms/2026-03-16-course-module-import-export-brainstorm.md)
- **Solutions doc:** [docs/solutions/integration-issues/cli-course-import-app-parity-and-payload-alignment.md](../solutions/integration-issues/cli-course-import-app-parity-and-payload-alignment.md) -- Bug 5 (SLT_LOCKED) and the "lessons can always be updated" assumption
- **Key code:** `cmd/andamio/course_import.go:1046-1108` (`fetchExistingModule`), `:1110-1235` (`updateModuleContent`), `:142-277` (`importModule`)
