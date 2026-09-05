---
status: pending
priority: p3
issue_id: "040"
tags: [quiz, course-import, import-assignment, verify, maintainability]
dependencies: ["039-pending-p2-teacher-module-list-206-blind-spot"]
---

# Residuals from the quiz-assignment work (#165, PR #166)

Real but not fixed in #166. Each is small; none blocks the feature.

## Read-back cannot see a clobbered concurrent edit

`course import-assignment` fetches, POSTs, then re-fetches. A teacher editing
description / image / video in the app between the fetch and the POST is
silently reverted (db-api overwrites every assignment field it is given) and
`verified: true` then certifies the revert. A compare-and-set would need
gateway support (an `updated_at` or version on the assignment); until then
the window is one round trip.

## Read-back has no retry

The post-write re-fetch uses `Post`, not `PostWithRetry`
(`course_teacher_ops.go` retries the same endpoint). A transient blip after
an accepted write is reported as `kind: verify` with the cause wrapped —
correct, but a retry would turn most of those into a confirmed publish.

## No committed fixture for the teacher module-list wire shape

`verifyStoredAssignment` and `updateModuleContent` assume the list nests the
assignment's raw JSON under `content.assignment.content_json`. Unlike the #90
qualified-contributors fixture (`internal/client/testdata/`), nothing pins
that shape; field drift would surface only as a spurious `kind: verify` in
production. Capture one from preprod.

## Validator drift is undetectable by CI

`internal/quiz` mirrors two apps' validators by hand (`testdata/quiz/SOURCE.md`
records the commits). A rule added in either app produces zero CLI failures
until someone re-mirrors. A weekly job that fetches both `quiz-envelope.ts`
files and diffs their issue-code lists against `quiz.AllCodes` would catch it.

## Duplicated metadata merge and hand-maintained verify field table

The preserve-existing-metadata rule lives in three shapes (two map loops in
`course_import.go`, a typed construction in `course_import_assignment.go`);
`existingString` duplicates `getStr` (`user.go`); the `fields` table in
`verifyStoredAssignment` will silently miss any new `assignmentInput` field.
Reflect over `assignmentInput`'s json tags, or generate the table from it.

## Export reports the quiz only by filename

`ExportResult.Files` lists `assignment.quiz.json`; there is no structured
`assignment_quiz` field to mirror `course import`'s. Additive when wanted.

## Live preprod check on an ON_CHAIN module

The "assignments are editable in any module status" statement rests on
gateway and db-api source. Run `course import-assignment` against an
`ON_CHAIN` module on preprod once and record the result in the CHANGELOG.
