---
status: pending
priority: p2
issue_id: "039"
tags: [degraded-read, 206, course, teacher-endpoints, reliability]
dependencies: []
---

# Teacher module-list scans outside import share the 206 blind spot

## Problem

The merged `POST /v2/course/teacher/course-modules/list` answers 206 with
`meta.warning` when one backend is down, and it carries `content` only from
db-api. On a db-api outage every module comes back chain-only with no
`content`, so a client-side scan for one module code finds nothing and reports
"not found" — for a module that exists.

PR #166 (#165) fixed this for the import paths: `fetchExistingModule`
(`cmd/andamio/course_import.go`) returns `errDegradedRead` when the module is
missing and the warning names the db side (`warningHidesDBContent`), and
`course import` / `course import-assignment` refuse to write on it. The other
scans of the same list still treat "not in the list" as "not found":

- `cmd/andamio/course.go` (`fetchTeacherModuleContent`)
- `cmd/andamio/course_export.go` (`fetchModuleData`)
- `cmd/andamio/course_teacher_ops.go`

Export is the worst case: a db-api outage exports nothing where a module has
content. Raised in the review of #166 as a follow-up candidate.

## Proposed fix

One shared `findTeacherModule(ctx, c, courseID, moduleCode)` helper in
`cmd/andamio/helpers.go` that owns the list call, the module match, and the
`errDegradedRead` / `errModuleNotFound` distinction, used by all four call
sites. Map `errModuleNotFound` to `apierr.NotFoundError` at the call sites
that expose it (`course.go` already does) so the exit code stays 2.

Related: `todos/031-pending-p1-no-typed-api-contract-coupling.md` (a typed
`MergedCourseModuleItem` with `Source` would make the chain-only case
explicit instead of inferred from a missing key).
