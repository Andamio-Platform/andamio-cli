---
title: "fix: course lesson/intro/assignment return 404 for draft modules"
type: fix
status: completed
date: 2026-04-07
origin: https://github.com/Andamio-Platform/andamio-cli/issues/56
---

# Fix: course lesson/intro/assignment return 404 for draft modules

## Overview

`course lesson`, `course intro`, and `course assignment` commands always use user endpoints that require modules to exist on-chain. When a module is created via `course import --create` (draft status), these commands return "No lesson found" even though `course slts` (which uses the teacher endpoint when JWT is available) shows lessons exist.

## Problem Frame

After importing a module with lessons, a teacher sees `has_lesson: "Yes"` via `course slts` but gets a 404 when trying to read the lesson via `course lesson`. The root cause is an endpoint mismatch: `course slts` already prefers the teacher endpoint for authenticated users (line 345 of `course.go`), but the three content commands do not.

The user endpoint `GET /api/v2/course/user/lesson/{id}/{module}/{slt}` requires on-chain module presence (per OpenAPI spec: "Returns lesson content for an SLT if the module exists on-chain"). Draft modules created by `course import --create` are DB-only, so the user endpoint returns 404.

Related: GitHub issue #56.

## Requirements Trace

- R1. `course lesson` must return lesson content for draft modules when the user has JWT auth
- R2. `course intro` must return introduction content for draft modules when the user has JWT auth
- R3. `course assignment` must return assignment content for draft modules when the user has JWT auth
- R4. All three commands must continue working via user endpoints when only API key auth is available
- R5. JSON output schema must remain stable — the response shape should match what the user endpoint returns

## Scope Boundaries

- No changes to API endpoints — this is a CLI-side fix only
- No new teacher-specific endpoints — extract content from the existing teacher modules list response
- No changes to `course slts` — it already works correctly
- No changes to import/export commands

## Context & Research

### Relevant Code and Patterns

- `cmd/andamio/course.go:345-347` — `runCourseSlts` already implements the teacher-endpoint preference pattern: check `cfg.HasUserAuth()`, call teacher endpoint, fall back to user endpoint
- `cmd/andamio/course.go:353-438` — `runCourseSltsTeacher` shows how to fetch from `POST /v2/course/teacher/course-modules/list`, find a module by code, and extract SLT data
- `cmd/andamio/course_import.go:1122-1184` — `fetchExistingModule` does the same fetch and extracts lessons (keyed by slt_index), introduction, and assignment from the teacher response
- `cmd/andamio/course.go:66-110` — Current lesson/intro/assignment handlers are simple one-liner delegates to `getJSONWithHint`

### Institutional Learnings

- The teacher modules list endpoint returns full content inline (SLTs with nested lessons, introduction, assignment) for both draft and on-chain modules
- Lessons in the teacher response are nested inside SLT objects as `slt.lesson` (a map), not in a separate array
- Introduction and assignment are top-level fields in the module's `content` object

## Key Technical Decisions

- **Extract from teacher modules list rather than adding new endpoints**: No dedicated teacher endpoint exists for fetching individual lessons/intros/assignments. The teacher modules list already returns all content inline. This is the same approach `fetchExistingModule` and `runCourseSltsTeacher` use.

- **Try teacher endpoint first, fall back to user endpoint**: When JWT is available, try the teacher endpoint. If it fails or the content isn't found, fall back to the user endpoint. This ensures the command works for all module states (draft, on-chain) and all auth levels (JWT, API key only).

- **Shared helper for teacher content fetch**: The module-finding logic (POST to teacher modules list, iterate to find matching module code, extract content) is duplicated between `runCourseSltsTeacher` and `fetchExistingModule`. A shared helper avoids tripling this duplication. The helper should return the raw module content map, letting each caller extract what it needs.

- **Match user endpoint response shape for JSON output**: To keep R5, wrap the extracted content in the same `{"data": {...}}` envelope the user endpoint returns. Include `course_id`, `course_module_code`, `slt_index` (for lessons), and the content fields.

## Open Questions

### Resolved During Planning

- **Are there teacher endpoints for individual content?** No — checked OpenAPI spec, only teacher modules list and update exist. Must extract from list response.
- **Does `course slts` without JWT also show lesson presence?** Yes, the user SLTs endpoint returns `has_lesson` boolean per SLT. But the user lesson endpoint still 404s for drafts. So the inconsistency only manifests when the module is draft.

### Deferred to Implementation

- **Exact response field mapping**: The teacher endpoint's lesson/intro/assignment objects may have slightly different field names than the user endpoint response. Implementation should verify and normalize if needed.

## Implementation Units

- [ ] **Unit 1: Add shared teacher module content helper**

  **Goal:** Extract the module-finding logic into a reusable helper that `course slts`, `course lesson`, `course intro`, and `course assignment` can all use.

  **Requirements:** Supports R1-R3 by providing the data source.

  **Dependencies:** None.

  **Files:**
  - Modify: `cmd/andamio/course.go`

  **Approach:**
  - Add a function like `fetchTeacherModuleContent(cfg, courseID, moduleCode)` that returns the raw module content map (the `content` object from the matching module in the teacher response)
  - Reuse the existing POST-to-teacher-modules-list + iterate pattern from `runCourseSltsTeacher` (lines 356-387)
  - Return `nil, errModuleNotFound` if the module isn't found
  - `runCourseSltsTeacher` can be refactored to use this helper, reducing its own module-finding code

  **Patterns to follow:**
  - `fetchExistingModule` in `course_import.go` for the fetch + iterate pattern
  - `runCourseSltsTeacher` for the same pattern in the read path

  **Test scenarios:**
  - Helper finds a module by code and returns its content map
  - Helper returns error when module code doesn't match any module

  **Verification:**
  - `runCourseSltsTeacher` continues to work identically after refactoring to use the helper

- [ ] **Unit 2: Add teacher-aware lesson handler**

  **Goal:** `course lesson` returns lesson content for draft modules when JWT is available.

  **Requirements:** R1, R4, R5.

  **Dependencies:** Unit 1.

  **Files:**
  - Modify: `cmd/andamio/course.go`

  **Approach:**
  - Replace the inline `RunE` closure with a named function (e.g., `runCourseLesson`)
  - When `cfg.HasUserAuth()`: call the shared helper, extract the lesson from `content.slts[slt_index-1].lesson`, wrap in the expected output shape, print via `output.PrintJSON`
  - Fall back to the existing user endpoint path on any teacher-endpoint failure or when no JWT
  - Preserve the `getJSONWithHint` error message for the fallback path

  **Patterns to follow:**
  - `runCourseSlts` teacher-preference pattern (lines 345-350)

  **Test scenarios:**
  - Draft module with lessons → returns lesson content via teacher endpoint
  - On-chain module with lessons → returns lesson content (either path works)
  - SLT index out of range → meaningful error
  - SLT exists but has no lesson → "No lesson found" hint
  - No JWT → falls back to user endpoint
  - `--output json` → stable JSON schema with `data` envelope

  **Verification:**
  - `andamio course lesson <course-id> <module-code> <slt-index>` succeeds for draft modules when authenticated with JWT

- [ ] **Unit 3: Add teacher-aware intro handler**

  **Goal:** `course intro` returns introduction content for draft modules when JWT is available.

  **Requirements:** R2, R4, R5.

  **Dependencies:** Unit 1.

  **Files:**
  - Modify: `cmd/andamio/course.go`

  **Approach:**
  - Replace the inline closure with a named function (e.g., `runCourseIntro`)
  - When `cfg.HasUserAuth()`: call the shared helper, extract `content.introduction`, wrap and print
  - Fall back to user endpoint when no JWT or on teacher-endpoint failure

  **Test scenarios:**
  - Draft module with introduction → returns intro content
  - Module without introduction → meaningful error
  - No JWT → falls back to user endpoint

  **Verification:**
  - `andamio course intro <course-id> <module-code>` succeeds for draft modules when authenticated with JWT

- [ ] **Unit 4: Add teacher-aware assignment handler**

  **Goal:** `course assignment` returns assignment content for draft modules when JWT is available.

  **Requirements:** R3, R4, R5.

  **Dependencies:** Unit 1.

  **Files:**
  - Modify: `cmd/andamio/course.go`

  **Approach:**
  - Replace the inline closure with a named function (e.g., `runCourseAssignment`)
  - When `cfg.HasUserAuth()`: call the shared helper, extract `content.assignment`, wrap and print
  - Fall back to user endpoint when no JWT or on teacher-endpoint failure

  **Test scenarios:**
  - Draft module with assignment → returns assignment content
  - Module without assignment → meaningful error
  - No JWT → falls back to user endpoint

  **Verification:**
  - `andamio course assignment <course-id> <module-code>` succeeds for draft modules when authenticated with JWT

- [ ] **Unit 5: Manual end-to-end verification**

  **Goal:** Verify all three commands work against a local devkit with a draft module.

  **Requirements:** R1-R5.

  **Dependencies:** Units 2-4.

  **Files:**
  - No file changes — verification only

  **Approach:**
  - Import a module with `--create` flag
  - Verify `course slts` shows lessons
  - Verify `course lesson`, `course intro`, `course assignment` all return content
  - Verify `--output json` returns stable schemas
  - Test with API key only (no JWT) to confirm fallback works

  **Verification:**
  - All reproduction steps from issue #56 pass
  - JSON output from teacher path matches user path structure

## System-Wide Impact

- **Interaction graph:** Only `course.go` command handlers change. No middleware, callbacks, or observers affected.
- **Error propagation:** Teacher endpoint failures fall back to user endpoint transparently. No new error types introduced.
- **API surface parity:** The `course modules` command also uses only the user endpoint and may have similar visibility limitations for draft modules, but that's out of scope for this fix.
- **State lifecycle risks:** None — read-only operations, no data mutation.

## Risks & Dependencies

- **Teacher endpoint returns all modules**: The teacher modules list returns every module for a course. For courses with many modules this is heavier than the targeted user endpoint. This is acceptable since `course slts` already does the same thing, and the teacher endpoint is only used when JWT is present.
- **Response shape differences**: The teacher endpoint's content objects may have different field names or nesting than the user endpoint response. Implementation must verify and normalize. This is deferred to implementation (see Open Questions).

## Sources & References

- **Origin:** [GitHub issue #56](https://github.com/Andamio-Platform/andamio-cli/issues/56)
- Related code: `cmd/andamio/course.go` (lesson/intro/assignment handlers, `runCourseSltsTeacher`)
- Related code: `cmd/andamio/course_import.go:1122-1184` (`fetchExistingModule` — same fetch pattern)
- OpenAPI spec: lesson endpoint requires on-chain module presence
