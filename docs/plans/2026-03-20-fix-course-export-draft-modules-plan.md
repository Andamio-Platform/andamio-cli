---
title: "Fix course export for unpublished/draft modules"
type: fix
status: completed
date: 2026-03-20
issue: 7
---

# Fix: course export fails for unpublished/draft modules

## Overview

Issue #7 reports that `course export` returns a 404 when the target module is not yet on-chain. The original bug (using `GET /api/v2/course/user/slts/` which only works for on-chain modules) was already fixed in commit `0324c26` by switching to `POST /api/v2/course/teacher/course-modules/list`. However, the issue remains open because edge cases were not addressed and verification was not documented.

## Current State

The main fix is already in `course_export.go`. `fetchModuleData()` uses the teacher endpoint exclusively -- no user endpoints remain. This means draft, approved, pending_tx, and on-chain modules all return data from the same call.

**Remaining gaps:**

1. **Empty draft modules produce silent empty output.** A brand-new DRAFT module with zero SLTs exports an `outline.md` with an empty SLTs section and no lesson files, with no warning to the user. This is confusing.
2. **No status indicator in text output.** The export prints the module status but doesn't flag when a module is in a pre-publication state where content may be incomplete.
3. **Progress messages go to stdout, not stderr.** Lines 87-88 use `fmt.Printf` (stdout) instead of `fmt.Fprintf(os.Stderr, ...)`, violating the composability rule in CLAUDE.md.

## Tasks

### 1. Warn on empty draft modules

In `runCourseExport`, after `fetchModuleData` returns, check if `len(moduleData.SLTs) == 0` and emit a warning to stderr:

```
Warning: module 101 (DRAFT) has no SLTs defined. Exported outline will be empty.
```

**File:** `cmd/andamio/course_export.go`, after line 92.

### 2. Fix progress output to stderr

Replace `fmt.Printf` calls on lines 87-88, 107, and 864-865 (in `downloadImages`) with `fmt.Fprintf(os.Stderr, ...)` to match the composability contract.

**File:** `cmd/andamio/course_export.go`, lines 87, 107, 863, 878, 882, 927.

### 3. Verify and close

Run manual verification against preprod with a DRAFT module and an ON_CHAIN module. Confirm both export correctly. Close issue #7.

## Test Plan

- `go test ./cmd/andamio/ -run TestExport` -- existing unit tests pass
- Manual: `andamio course export <course-id> <draft-module-code>` succeeds
- Manual: `andamio course export <course-id> <on-chain-module-code>` still works (no regression)
- Verify `--output json` produces no stderr pollution
